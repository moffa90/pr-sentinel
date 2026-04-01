package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/moffa90/pr-sentinel/internal/config"
	"github.com/moffa90/pr-sentinel/internal/github"
	"github.com/moffa90/pr-sentinel/internal/notifier"
	"github.com/moffa90/pr-sentinel/internal/publisher"
	"github.com/moffa90/pr-sentinel/internal/reviewer"
	"github.com/moffa90/pr-sentinel/internal/state"
)

// PollOptions holds the resolved options for a single poll cycle.
type PollOptions struct {
	MaxReviewsPerCycle int
	MaxReviewsPerDay   int
	ReviewTimeout      time.Duration
	GitHubUser         string
	ReviewInstructions string
	DisclosureText     string
	AIDisclosure       bool
}

// PollResult summarises the outcome of a single poll cycle.
type PollResult struct {
	Reviewed int
	Posted   int
	DryRun   int
	Skipped  int
	Errors   int
}

// PollOptionsFromConfig derives PollOptions from a Config.
func PollOptionsFromConfig(cfg config.Config) PollOptions {
	return PollOptions{
		MaxReviewsPerCycle: cfg.MaxReviewsPerCycle,
		MaxReviewsPerDay:   cfg.MaxReviewsPerDay,
		ReviewTimeout:      cfg.ReviewTimeout,
		GitHubUser:         cfg.GitHubUser,
		ReviewInstructions: cfg.Review.Instructions,
		DisclosureText:     cfg.Review.DisclosureText,
		AIDisclosure:       cfg.Review.AIDisclosure,
	}
}

// shouldSkip returns true when either the per-cycle or per-day limit has been reached.
func shouldSkip(opts PollOptions, cycleCount int, dailyCount int) bool {
	if cycleCount >= opts.MaxReviewsPerCycle {
		return true
	}
	if dailyCount >= opts.MaxReviewsPerDay {
		return true
	}
	return false
}

// RunPollCycle iterates configured repos, fetches open PRs, runs reviews,
// publishes results, records state, and sends notifications.
func RunPollCycle(ctx context.Context, cfg config.Config, store *state.Store, notify *notifier.Dispatcher) PollResult {
	opts := PollOptionsFromConfig(cfg)
	today := time.Now().UTC().Format("2006-01-02")
	dailyCount, err := store.GetDailyCount(today)
	if err != nil {
		slog.Error("failed to get daily count", "error", err)
	}

	var result PollResult
	cycleCount := 0

	for _, repo := range cfg.Repos {
		if ctx.Err() != nil {
			break
		}

		slog.Info("polling repo", "repo", repo.Name, "mode", repo.Mode)

		prs, err := github.FetchOpenPRs(repo.Name, opts.GitHubUser)
		if err != nil {
			slog.Error("failed to fetch PRs", "repo", repo.Name, "error", err)
			result.Errors++
			continue
		}

		for _, pr := range prs {
			if ctx.Err() != nil {
				break
			}

			if shouldSkip(opts, cycleCount, dailyCount) {
				slog.Info("review limit reached, skipping", "repo", repo.Name, "pr", pr.Number)
				result.Skipped++
				continue
			}

			reviewed, err := store.HasReviewed(repo.Name, pr.Number)
			if err != nil {
				slog.Error("failed to check review state", "repo", repo.Name, "pr", pr.Number, "error", err)
				result.Errors++
				continue
			}
			if reviewed {
				slog.Debug("already reviewed, skipping", "repo", repo.Name, "pr", pr.Number)
				result.Skipped++
				continue
			}

			slog.Info("reviewing PR", "repo", repo.Name, "pr", pr.Number, "title", pr.Title)

			diff, err := github.GetPRDiff(repo.Name, pr.Number)
			if err != nil {
				slog.Error("failed to get PR diff", "repo", repo.Name, "pr", pr.Number, "error", err)
				result.Errors++
				continue
			}

			prompt := reviewer.BuildReviewPrompt(reviewer.ReviewParams{
				Repo:     repo.Name,
				PRNumber: pr.Number,
				PRTitle:  pr.Title,
				PRAuthor: pr.Author,
				Diff:     diff,
				Files:    pr.Files,
				Adds:     pr.Additions,
				Dels:     pr.Deletions,
			})

			repoPath := config.ExpandPath(filepath.Join(cfg.ReposDir, filepath.Base(repo.Name)))
			reviewResult := reviewer.RunReview(ctx, repoPath, prompt, opts.ReviewInstructions, repo.ReviewInstructions, opts.ReviewTimeout)
			if reviewResult.Error != nil {
				slog.Error("review failed", "repo", repo.Name, "pr", pr.Number, "error", reviewResult.Error)
				result.Errors++
				continue
			}

			body := publisher.BuildReviewBody(reviewResult.Output, opts.AIDisclosure, opts.DisclosureText, pr.Author)

			posted := false
			reviewPath := ""
			mode := repo.Mode

			if mode == config.ModeLive {
				if err := publisher.PostLiveReview(repo.Name, pr.Number, body); err != nil {
					slog.Error("failed to post review", "repo", repo.Name, "pr", pr.Number, "error", err)
					result.Errors++
					continue
				}
				posted = true
				result.Posted++
			} else {
				reviewsDir := filepath.Join(config.ConfigDir(), "reviews")
				savedPath, err := publisher.SaveDryRunReview(publisher.SaveParams{
					ReviewsDir: reviewsDir,
					Repo:       repo.Name,
					PRNumber:   pr.Number,
					PRTitle:    pr.Title,
					PRAuthor:   pr.Author,
					Body:       body,
				})
				if err != nil {
					slog.Error("failed to save dry-run review", "repo", repo.Name, "pr", pr.Number, "error", err)
					result.Errors++
					continue
				}
				reviewPath = savedPath
				result.DryRun++
			}

			if err := store.RecordReview(state.ReviewRecord{
				Repo:            repo.Name,
				PRNumber:        pr.Number,
				PRTitle:         pr.Title,
				PRAuthor:        pr.Author,
				ReviewOutput:    reviewResult.Output,
				FindingsSummary: fmt.Sprintf("%d files, %d additions, %d deletions", pr.Files, pr.Additions, pr.Deletions),
				Mode:            mode,
				Posted:          posted,
				ReviewedAt:      time.Now().UTC(),
			}); err != nil {
				slog.Error("failed to record review", "repo", repo.Name, "pr", pr.Number, "error", err)
			}

			if err := store.IncrementDailyCount(today); err != nil {
				slog.Error("failed to increment daily count", "error", err)
			}

			if notify != nil {
				evt := notifier.NewEvent(
					repo.Name, pr.Number, pr.Title, pr.Author, pr.URL,
					mode, posted, fmt.Sprintf("%d files changed", pr.Files), reviewPath,
				)
				if err := notify.Notify(evt); err != nil {
					slog.Error("notification failed", "error", err)
				}
			}

			result.Reviewed++
			cycleCount++
			dailyCount++
		}
	}

	slog.Info("poll cycle complete",
		"reviewed", result.Reviewed,
		"posted", result.Posted,
		"dry_run", result.DryRun,
		"skipped", result.Skipped,
		"errors", result.Errors,
	)

	return result
}

// RunDaemon starts the poll loop. It runs a cycle immediately, then on every
// tick of cfg.PollInterval until the context is cancelled.
func RunDaemon(ctx context.Context, cfg config.Config, store *state.Store, notify *notifier.Dispatcher) error {
	slog.Info("daemon starting", "poll_interval", cfg.PollInterval)

	// Run immediately on start.
	RunPollCycle(ctx, cfg, store, notify)

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("daemon stopping", "reason", ctx.Err())
			return ctx.Err()
		case <-ticker.C:
			RunPollCycle(ctx, cfg, store, notify)
		}
	}
}

// BuildNotifier creates a Dispatcher from the notification settings in cfg.
func BuildNotifier(cfg config.Config) *notifier.Dispatcher {
	var notifiers []notifier.Notifier

	if cfg.Notifications.MacOS {
		notifiers = append(notifiers, notifier.NewMacOSNotifier())
	}
	if cfg.Notifications.Slack.Enabled && cfg.Notifications.Slack.WebhookURL != "" {
		notifiers = append(notifiers, notifier.NewSlackNotifier(cfg.Notifications.Slack.WebhookURL))
	}
	if cfg.Notifications.Teams.Enabled && cfg.Notifications.Teams.WebhookURL != "" {
		notifiers = append(notifiers, notifier.NewTeamsNotifier(cfg.Notifications.Teams.WebhookURL))
	}
	if cfg.Notifications.Webhook.Enabled && cfg.Notifications.Webhook.URL != "" {
		notifiers = append(notifiers, notifier.NewWebhookNotifier(cfg.Notifications.Webhook.URL))
	}

	return notifier.NewDispatcher(notifiers...)
}
