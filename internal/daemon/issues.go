package daemon

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/moffa90/pr-sentinel/internal/config"
	"github.com/moffa90/pr-sentinel/internal/github"
	"github.com/moffa90/pr-sentinel/internal/publisher"
	"github.com/moffa90/pr-sentinel/internal/retry"
	"github.com/moffa90/pr-sentinel/internal/reviewer"
	"github.com/moffa90/pr-sentinel/internal/state"
)

// GitHubActions abstracts the GitHub writes made after a review so the
// post-review flow is testable without the gh CLI.
type GitHubActions interface {
	PostReview(repo string, number int64, body, verdict string) error
	EnableAutoMerge(repo string, number int64, strategy string, deleteBranch bool) error
	CreateIssue(repo, title, body string, labels []string) (int64, string, error)
	CommentOnIssue(repo, ref, body string) error
	GetIssueState(repo, ref string) (string, error)
}

// GitHubCLI is the production GitHubActions implementation backed by the gh CLI.
type GitHubCLI struct{}

func (GitHubCLI) PostReview(repo string, number int64, body, verdict string) error {
	return publisher.PostLiveReview(repo, number, body, verdict)
}

func (GitHubCLI) EnableAutoMerge(repo string, number int64, strategy string, deleteBranch bool) error {
	return github.EnableAutoMerge(repo, number, strategy, deleteBranch)
}

func (GitHubCLI) CreateIssue(repo, title, body string, labels []string) (int64, string, error) {
	return github.CreateIssue(repo, title, body, labels)
}

func (GitHubCLI) CommentOnIssue(repo, ref, body string) error {
	return github.CommentOnIssue(repo, ref, body)
}

func (GitHubCLI) GetIssueState(repo, ref string) (string, error) {
	return github.GetIssueState(repo, ref)
}

// issueRef returns the issue number when known, otherwise its URL. gh accepts either.
func issueRef(r state.IssueRecord) string {
	if r.IssueNumber > 0 {
		return strconv.FormatInt(r.IssueNumber, 10)
	}
	return r.IssueURL
}

// issueLabel formats an issue reference for status output.
func issueLabel(r state.IssueRecord) string {
	if r.IssueNumber > 0 {
		return fmt.Sprintf("#%d", r.IssueNumber)
	}
	return r.IssueURL
}

// qualifyingFindings returns the findings at or above the configured minimum severity.
func qualifyingFindings(ic config.IssuesConfig, findings []reviewer.Finding) []reviewer.Finding {
	var out []reviewer.Finding
	for _, f := range findings {
		if ic.Qualifies(f.Severity) {
			out = append(out, f)
		}
	}
	return out
}

func buildIssueTitle(pr github.PullRequest) string {
	return fmt.Sprintf("pr-sentinel: review findings for #%d %s", pr.Number, pr.Title)
}

// buildIssueBody renders findings as a checklist. Referencing #N cross-links the PR.
func buildIssueBody(pr github.PullRequest, findings []reviewer.Finding, followUp bool) string {
	var b strings.Builder
	if followUp {
		fmt.Fprintf(&b, "Follow-up review of #%d still reports %d finding(s):\n\n", pr.Number, len(findings))
	} else {
		fmt.Fprintf(&b, "Review of #%d (by @%s) reported %d finding(s):\n\n", pr.Number, pr.Author, len(findings))
	}
	for _, f := range findings {
		loc := f.File
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		fmt.Fprintf(&b, "- [ ] **%s** `%s` — %s\n", strings.ToUpper(f.Severity), loc, f.Message)
	}
	b.WriteString("\n_Opened by pr-sentinel._\n")
	return b.String()
}

// handleIssues opens an issue for qualifying findings, or comments on the PR's
// existing issue on follow-up reviews. A closed existing issue is replaced by a
// new one. Returns a short status for logging and command output.
func handleIssues(store *state.Store, gh GitHubActions, repo config.RepoConfig, pr github.PullRequest, review *reviewer.StructuredReview) string {
	if !repo.Issues.Enabled || review == nil {
		return ""
	}

	findings := qualifyingFindings(repo.Issues, review.Findings)
	if len(findings) == 0 {
		return ""
	}

	existing, found, err := store.GetIssue(repo.Name, pr.Number)
	if err != nil {
		slog.Error("failed to look up issue", "repo", repo.Name, "pr", pr.Number, "error", err)
		return "Failed: state lookup"
	}

	live := repo.Mode == config.ModeLive

	if found && live {
		issueState, err := gh.GetIssueState(repo.Name, issueRef(existing))
		if err != nil {
			// Can't tell; commenting is safer than opening a possible duplicate.
			slog.Warn("could not check issue state, commenting", "repo", repo.Name, "issue", issueLabel(existing), "error", err)
		} else if issueState == "CLOSED" {
			slog.Info("existing issue closed, opening a new one", "repo", repo.Name, "pr", pr.Number, "issue", issueLabel(existing))
			found = false
		}
	}

	if found {
		body := buildIssueBody(pr, findings, true)
		if !live {
			slog.Info("issue dry-run: would comment", "repo", repo.Name, "pr", pr.Number, "issue", issueLabel(existing), "findings", len(findings))
			return fmt.Sprintf("Would comment on %s", issueLabel(existing))
		}
		if err := gh.CommentOnIssue(repo.Name, issueRef(existing), body); err != nil {
			slog.Warn("failed to comment on issue", "repo", repo.Name, "pr", pr.Number, "issue", issueLabel(existing), "error", err)
			return fmt.Sprintf("Failed: %s", err)
		}
		slog.Info("commented on issue", "repo", repo.Name, "pr", pr.Number, "issue", issueLabel(existing))
		return fmt.Sprintf("Commented on %s", issueLabel(existing))
	}

	if !live {
		slog.Info("issue dry-run: would create", "repo", repo.Name, "pr", pr.Number, "findings", len(findings))
		return "Would create issue"
	}

	number, url, err := gh.CreateIssue(repo.Name, buildIssueTitle(pr), buildIssueBody(pr, findings, false), repo.Issues.Labels)
	if err != nil && url == "" {
		slog.Warn("failed to create issue", "repo", repo.Name, "pr", pr.Number, "error", err)
		return fmt.Sprintf("Failed: %s", err)
	}
	if err != nil {
		// Created, but the number couldn't be parsed. Record the URL anyway so
		// follow-ups comment on it instead of opening duplicates.
		slog.Warn("issue created but number unknown", "repo", repo.Name, "pr", pr.Number, "url", url, "error", err)
	}

	rec := state.IssueRecord{
		Repo:        repo.Name,
		PRNumber:    pr.Number,
		IssueNumber: number,
		IssueURL:    url,
		CreatedAt:   time.Now().UTC(),
	}
	if err := retry.Do(3, 500*time.Millisecond, "record issue", func() error {
		return store.RecordIssue(rec)
	}); err != nil {
		slog.Error("issue created but not recorded, a follow-up may open a duplicate", "repo", repo.Name, "pr", pr.Number, "url", url, "error", err)
		return fmt.Sprintf("Created %s (Failed: state record)", issueLabel(rec))
	}
	slog.Info("issue created", "repo", repo.Name, "pr", pr.Number, "issue", issueLabel(rec), "url", url)
	return fmt.Sprintf("Created %s", issueLabel(rec))
}
