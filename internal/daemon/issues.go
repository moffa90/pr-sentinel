package daemon

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/moffa90/pr-sentinel/internal/config"
	"github.com/moffa90/pr-sentinel/internal/github"
	"github.com/moffa90/pr-sentinel/internal/reviewer"
	"github.com/moffa90/pr-sentinel/internal/state"
)

// IssueClient abstracts GitHub issue operations so issue handling is testable.
type IssueClient interface {
	CreateIssue(repo, title, body string, labels []string) (int64, string, error)
	CommentOnIssue(repo string, number int64, body string) error
}

// GitHubIssueClient is the production implementation that calls the gh CLI.
type GitHubIssueClient struct{}

func (GitHubIssueClient) CreateIssue(repo, title, body string, labels []string) (int64, string, error) {
	return github.CreateIssue(repo, title, body, labels)
}

func (GitHubIssueClient) CommentOnIssue(repo string, number int64, body string) error {
	return github.CommentOnIssue(repo, number, body)
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
// existing issue on follow-up reviews. Returns a short status for logging.
func handleIssues(store *state.Store, client IssueClient, repo config.RepoConfig, pr github.PullRequest, review *reviewer.StructuredReview) string {
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

	body := buildIssueBody(pr, findings, found)
	live := repo.Mode == config.ModeLive

	if found {
		if !live {
			slog.Info("issue dry-run: would comment", "repo", repo.Name, "pr", pr.Number, "issue", existing.IssueNumber, "findings", len(findings))
			return fmt.Sprintf("Would comment on #%d", existing.IssueNumber)
		}
		if err := client.CommentOnIssue(repo.Name, existing.IssueNumber, body); err != nil {
			slog.Warn("failed to comment on issue", "repo", repo.Name, "pr", pr.Number, "issue", existing.IssueNumber, "error", err)
			return fmt.Sprintf("Failed: %s", err)
		}
		slog.Info("commented on issue", "repo", repo.Name, "pr", pr.Number, "issue", existing.IssueNumber)
		return fmt.Sprintf("Commented on #%d", existing.IssueNumber)
	}

	if !live {
		slog.Info("issue dry-run: would create", "repo", repo.Name, "pr", pr.Number, "findings", len(findings))
		return "Would create issue"
	}

	number, url, err := client.CreateIssue(repo.Name, buildIssueTitle(pr), body, repo.Issues.Labels)
	if err != nil {
		slog.Warn("failed to create issue", "repo", repo.Name, "pr", pr.Number, "error", err)
		return fmt.Sprintf("Failed: %s", err)
	}
	if err := store.RecordIssue(state.IssueRecord{
		Repo:        repo.Name,
		PRNumber:    pr.Number,
		IssueNumber: number,
		IssueURL:    url,
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		slog.Error("failed to record issue", "repo", repo.Name, "pr", pr.Number, "issue", number, "error", err)
	}
	slog.Info("issue created", "repo", repo.Name, "pr", pr.Number, "issue", number, "url", url)
	return fmt.Sprintf("Created #%d", number)
}
