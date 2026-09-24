package daemon

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/moffa90/pr-sentinel/internal/config"
	"github.com/moffa90/pr-sentinel/internal/github"
	"github.com/moffa90/pr-sentinel/internal/notifier"
	"github.com/moffa90/pr-sentinel/internal/publisher"
	"github.com/moffa90/pr-sentinel/internal/retry"
	"github.com/moffa90/pr-sentinel/internal/reviewer"
	"github.com/moffa90/pr-sentinel/internal/state"
)

// ProcessOutcome describes what ProcessReview did with a completed review.
type ProcessOutcome struct {
	Posted     bool
	ReviewPath string // dry-run file path, empty in live mode
	AutoMerge  string // auto-merge status, empty when not attempted
	Issue      string // issue status, empty when not attempted
}

// ReviewBody renders the comment body for a completed review: formatted
// markdown when structured output parsed, raw output otherwise.
func ReviewBody(opts PollOptions, pr github.PullRequest, rr reviewer.ReviewResult) string {
	content := rr.Output
	if rr.Review != nil {
		content = rr.Review.FormatMarkdown()
	}
	return publisher.BuildReviewBody(content, opts.AIDisclosure, opts.DisclosureText, pr.Author)
}

// isSelfAuthored reports whether the PR was opened by the configured GitHub user.
// GitHub rejects --approve/--request-changes and auto-merge approval on own PRs.
func isSelfAuthored(opts PollOptions, pr github.PullRequest) bool {
	return opts.GitHubUser != "" && strings.EqualFold(pr.Author, opts.GitHubUser)
}

// ProcessReview runs every post-review step shared by the daemon and the
// review command: publish (live or dry-run), auto-merge, issues, state
// recording, and notifications. It returns an error only when publishing
// fails, in which case nothing is recorded.
func ProcessReview(store *state.Store, notify *notifier.Dispatcher, opts PollOptions, repo config.RepoConfig, pr github.PullRequest, rr reviewer.ReviewResult) (ProcessOutcome, error) {
	return ProcessReviewWith(store, notify, opts, repo, pr, rr, GitHubCLI{})
}

// ProcessReviewWith is the testable version of ProcessReview that accepts
// the GitHub actions implementation.
func ProcessReviewWith(store *state.Store, notify *notifier.Dispatcher, opts PollOptions, repo config.RepoConfig, pr github.PullRequest, rr reviewer.ReviewResult, gh GitHubActions) (ProcessOutcome, error) {
	var out ProcessOutcome

	verdict := ""
	summary := ""
	if rr.Review != nil {
		verdict = string(rr.Review.Verdict)
		summary = rr.Review.Summary
	}

	body := ReviewBody(opts, pr, rr)
	selfAuthored := isSelfAuthored(opts, pr)
	mode := repo.Mode

	if mode == config.ModeLive {
		postVerdict := verdict
		if selfAuthored {
			postVerdict = "comment"
		}
		if err := retry.Do(3, 2*time.Second, "post review", func() error {
			return gh.PostReview(repo.Name, pr.Number, body, postVerdict)
		}); err != nil {
			return out, fmt.Errorf("posting review: %w", err)
		}
		out.Posted = true
	} else {
		savedPath, err := publisher.SaveDryRunReview(publisher.SaveParams{
			ReviewsDir: filepath.Join(config.ConfigDir(), "reviews"),
			Repo:       repo.Name,
			PRNumber:   pr.Number,
			PRTitle:    pr.Title,
			PRAuthor:   pr.Author,
			Body:       body,
		})
		if err != nil {
			return out, fmt.Errorf("saving dry-run review: %w", err)
		}
		out.ReviewPath = savedPath
	}

	findingsSummary := fmt.Sprintf("%d files, %d additions, %d deletions", pr.Files, pr.Additions, pr.Deletions)
	if rr.Review != nil {
		findingsSummary = rr.Review.FindingsSummary()
	}

	if repo.AutoMerge.Enabled && verdict == "approve" && !selfAuthored {
		out.AutoMerge = autoMerge(gh, repo, pr, rr.Review)
	}

	out.Issue = handleIssues(store, gh, repo, pr, rr.Review)

	if err := store.RecordReview(state.ReviewRecord{
		Repo:            repo.Name,
		PRNumber:        pr.Number,
		PRTitle:         pr.Title,
		PRAuthor:        pr.Author,
		ReviewOutput:    rr.Output,
		FindingsSummary: findingsSummary,
		Mode:            mode,
		Posted:          out.Posted,
		CostUSD:         rr.CostUSD,
		ReviewedAt:      time.Now().UTC(),
	}); err != nil {
		slog.Error("failed to record review", "repo", repo.Name, "pr", pr.Number, "error", err)
	}

	if err := store.IncrementDailyCount(time.Now().UTC().Format("2006-01-02")); err != nil {
		slog.Error("failed to increment daily count", "error", err)
	}

	evt := notifier.NewEvent(
		repo.Name, pr.Number, pr.Title, pr.Author, pr.URL,
		mode, out.Posted, findingsSummary, out.ReviewPath, verdict, summary,
	)
	evt.AutoMerge = out.AutoMerge

	// Send to per-repo Teams webhook if configured
	if repo.TeamsWebhook != "" {
		repoTeams := notifier.NewTeamsNotifier(repo.TeamsWebhook)
		if err := repoTeams.Notify(evt); err != nil {
			slog.Error("repo teams notification failed", "repo", repo.Name, "error", err)
		}
	}

	// Send to global notifiers
	if notify != nil {
		if err := notify.Notify(evt); err != nil {
			slog.Error("notification failed", "error", err)
		}
	}

	return out, nil
}

// autoMerge applies the auto-merge gates (no HIGH/MEDIUM findings, required
// label) and enables GitHub auto-merge in live mode. Returns a status string.
func autoMerge(gh GitHubActions, repo config.RepoConfig, pr github.PullRequest, review *reviewer.StructuredReview) string {
	if review != nil {
		for _, f := range review.Findings {
			if f.Severity == "HIGH" || f.Severity == "MEDIUM" {
				slog.Info("auto-merge skipped due to findings", "repo", repo.Name, "pr", pr.Number)
				return "Skipped (has HIGH/MEDIUM findings)"
			}
		}
	}

	if repo.AutoMerge.RequireLabel != "" && !hasLabel(pr.Labels, repo.AutoMerge.RequireLabel) {
		slog.Info("auto-merge skipped due to missing label", "repo", repo.Name, "pr", pr.Number, "required_label", repo.AutoMerge.RequireLabel)
		return fmt.Sprintf("Skipped (missing label %q)", repo.AutoMerge.RequireLabel)
	}

	strategy := repo.AutoMerge.Strategy
	if repo.Mode != config.ModeLive {
		slog.Info("auto-merge dry-run", "repo", repo.Name, "pr", pr.Number, "strategy", strategy)
		return fmt.Sprintf("Would merge (%s)", strategy)
	}

	if err := gh.EnableAutoMerge(repo.Name, pr.Number, strategy, repo.AutoMerge.DeleteBranch); err != nil {
		slog.Warn("auto-merge failed", "repo", repo.Name, "pr", pr.Number, "error", err)
		return fmt.Sprintf("Failed: %s", err)
	}
	slog.Info("auto-merge enabled", "repo", repo.Name, "pr", pr.Number, "strategy", strategy)
	return fmt.Sprintf("Enabled (%s)", strategy)
}
