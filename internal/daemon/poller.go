package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/moffa90/pr-sentinel/internal/config"
	"github.com/moffa90/pr-sentinel/internal/github"
	"github.com/moffa90/pr-sentinel/internal/notifier"
	"github.com/moffa90/pr-sentinel/internal/publisher"
	"github.com/moffa90/pr-sentinel/internal/retry"
	"github.com/moffa90/pr-sentinel/internal/reviewer"
	"github.com/moffa90/pr-sentinel/internal/state"
)

// PollOptions holds the resolved options for a single poll cycle.
type PollOptions struct {
	MaxReviewsPerCycle int
	MaxReviewsPerDay   int
	MaxParallelReviews int
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
		MaxParallelReviews: cfg.MaxParallelReviews,
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

// reviewWork represents a single PR review to be executed.
type reviewWork struct {
	repo     config.RepoConfig
	pr       github.PullRequest
	prompt   string
	repoPath string
}

// reviewOutcome holds the result of a single review execution.
type reviewOutcome struct {
	work   reviewWork
	result reviewer.ReviewResult
	body   string
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

	// Phase 1: Collect work items
	var work []reviewWork
	for _, repo := range cfg.Repos {
		if ctx.Err() != nil {
			break
		}

		slog.Info("polling repo", "repo", repo.Name, "mode", repo.Mode)

		prs, followUpCandidates, err := github.FetchOpenPRs(repo.Name, opts.GitHubUser)
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

			prompt := reviewer.BuildReviewPrompt(reviewer.ReviewParams{
				Repo:     repo.Name,
				PRNumber: pr.Number,
				PRTitle:  pr.Title,
				PRAuthor: pr.Author,
				Files:    pr.Files,
				Adds:     pr.Additions,
				Dels:     pr.Deletions,
			})

			repoPath := config.ExpandPath(repo.Path)
			work = append(work, reviewWork{
				repo:     repo,
				pr:       pr,
				prompt:   prompt,
				repoPath: repoPath,
			})

			cycleCount++
			dailyCount++
		}

		// Collect follow-up work items (PRs with new commits since last user comment)
		for _, candidate := range followUpCandidates {
			if ctx.Err() != nil {
				break
			}

			if shouldSkip(opts, cycleCount, dailyCount) {
				slog.Info("review limit reached, skipping follow-up", "repo", repo.Name, "pr", candidate.Number)
				result.Skipped++
				continue
			}

			// Get previous review from state store for follow-up context
			previousReview := ""
			prevRecord, prevErr := store.GetReview(repo.Name, candidate.Number)
			if prevErr == nil {
				previousReview = prevRecord.ReviewOutput
			}

			prompt := reviewer.BuildFollowUpPrompt(reviewer.FollowUpParams{
				Repo:           repo.Name,
				PRNumber:       candidate.Number,
				PRTitle:        candidate.Title,
				PRAuthor:       candidate.Author,
				Files:          candidate.Files,
				Adds:           candidate.Additions,
				Dels:           candidate.Deletions,
				PreviousReview: previousReview,
				NewCommitCount: candidate.NewCommitCount,
			})

			repoPath := config.ExpandPath(repo.Path)
			work = append(work, reviewWork{
				repo:     repo,
				pr:       candidate.PullRequest,
				prompt:   prompt,
				repoPath: repoPath,
			})

			slog.Info("queued follow-up review", "repo", repo.Name, "pr", candidate.Number, "new_commits", candidate.NewCommitCount)
			cycleCount++
			dailyCount++
		}
	}

	slog.Debug("work items collected", "items", len(work))

	if len(work) == 0 {
		slog.Info("poll cycle complete",
			"reviewed", result.Reviewed,
			"posted", result.Posted,
			"dry_run", result.DryRun,
			"skipped", result.Skipped,
			"errors", result.Errors,
		)
		return result
	}

	slog.Info("starting reviews", "count", len(work), "parallel", opts.MaxParallelReviews)

	// Phase 2: Execute reviews in parallel
	parallel := opts.MaxParallelReviews
	if parallel <= 0 {
		parallel = 1
	}
	sem := make(chan struct{}, parallel)
	outcomes := make(chan reviewOutcome, len(work))
	var wg sync.WaitGroup

	for _, w := range work {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(w reviewWork) {
			defer wg.Done()
			select {
			case sem <- struct{}{}: // acquire
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }() // release

			slog.Info("reviewing PR", "repo", w.repo.Name, "pr", w.pr.Number, "title", w.pr.Title)
			slog.Debug("starting review subprocess", "repo", w.repo.Name, "pr", w.pr.Number, "repoPath", w.repoPath)
			rr := reviewer.RunReview(ctx, w.repoPath, w.prompt, opts.ReviewInstructions, w.repo.ReviewInstructions, opts.ReviewTimeout)
			body := ""
			if rr.Error == nil {
				if rr.Review != nil {
					body = publisher.BuildReviewBody(rr.Review.FormatMarkdown(), opts.AIDisclosure, opts.DisclosureText, w.pr.Author)
					slog.Info("review complete", "repo", w.repo.Name, "pr", w.pr.Number, "duration", rr.Duration.Round(time.Second), "verdict", rr.Review.Verdict, "findings", rr.Review.FindingsSummary(), "cost_usd", rr.CostUSD)
				} else {
					body = publisher.BuildReviewBody(rr.Output, opts.AIDisclosure, opts.DisclosureText, w.pr.Author)
					slog.Info("review complete", "repo", w.repo.Name, "pr", w.pr.Number, "duration", rr.Duration.Round(time.Second), "cost_usd", rr.CostUSD)
				}
			} else {
				slog.Info("review complete", "repo", w.repo.Name, "pr", w.pr.Number, "duration", rr.Duration.Round(time.Second))
			}
			outcomes <- reviewOutcome{work: w, result: rr, body: body}
		}(w)
	}

	// Close outcomes channel when all goroutines finish
	go func() {
		wg.Wait()
		close(outcomes)
	}()

	// Phase 3: Process outcomes sequentially
	for o := range outcomes {
		if o.result.Error != nil {
			slog.Error("review failed", "repo", o.work.repo.Name, "pr", o.work.pr.Number, "error", o.result.Error)
			result.Errors++
			continue
		}

		posted := false
		reviewPath := ""
		mode := o.work.repo.Mode

		if mode == config.ModeLive {
			if err := retry.Do(3, 2*time.Second, "post review", func() error {
				return publisher.PostLiveReview(o.work.repo.Name, o.work.pr.Number, o.body)
			}); err != nil {
				slog.Error("failed to post review", "repo", o.work.repo.Name, "pr", o.work.pr.Number, "error", err)
				result.Errors++
				continue
			}
			posted = true
			result.Posted++
		} else {
			reviewsDir := filepath.Join(config.ConfigDir(), "reviews")
			savedPath, err := publisher.SaveDryRunReview(publisher.SaveParams{
				ReviewsDir: reviewsDir,
				Repo:       o.work.repo.Name,
				PRNumber:   o.work.pr.Number,
				PRTitle:    o.work.pr.Title,
				PRAuthor:   o.work.pr.Author,
				Body:       o.body,
			})
			if err != nil {
				slog.Error("failed to save dry-run review", "repo", o.work.repo.Name, "pr", o.work.pr.Number, "error", err)
				result.Errors++
				continue
			}
			reviewPath = savedPath
			result.DryRun++
		}

		findingsSummary := fmt.Sprintf("%d files, %d additions, %d deletions", o.work.pr.Files, o.work.pr.Additions, o.work.pr.Deletions)
		if o.result.Review != nil {
			findingsSummary = o.result.Review.FindingsSummary()
		}

		if err := store.RecordReview(state.ReviewRecord{
			Repo:            o.work.repo.Name,
			PRNumber:        o.work.pr.Number,
			PRTitle:         o.work.pr.Title,
			PRAuthor:        o.work.pr.Author,
			ReviewOutput:    o.result.Output,
			FindingsSummary: findingsSummary,
			Mode:            mode,
			Posted:          posted,
			ReviewedAt:      time.Now().UTC(),
		}); err != nil {
			slog.Error("failed to record review", "repo", o.work.repo.Name, "pr", o.work.pr.Number, "error", err)
		}

		if err := store.IncrementDailyCount(today); err != nil {
			slog.Error("failed to increment daily count", "error", err)
		}

		evt := notifier.NewEvent(
			o.work.repo.Name, o.work.pr.Number, o.work.pr.Title, o.work.pr.Author, o.work.pr.URL,
			mode, posted, findingsSummary, reviewPath,
		)

		// Send to per-repo Teams webhook if configured
		if o.work.repo.TeamsWebhook != "" {
			repoTeams := notifier.NewTeamsNotifier(o.work.repo.TeamsWebhook)
			if err := repoTeams.Notify(evt); err != nil {
				slog.Error("repo teams notification failed", "repo", o.work.repo.Name, "error", err)
			}
		}

		// Send to global notifiers
		if notify != nil {
			if err := notify.Notify(evt); err != nil {
				slog.Error("notification failed", "error", err)
			}
		}

		result.Reviewed++
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
// tick of cfg.PollInterval until the context is cancelled. If a cycle is still
// running when the next tick fires, the tick is skipped.
func RunDaemon(ctx context.Context, cfg config.Config, store *state.Store, notify *notifier.Dispatcher) error {
	slog.Info("daemon starting", "poll_interval", cfg.PollInterval)

	running := false
	cycleCount := 0

	runCycle := func() {
		running = true
		result := RunPollCycle(ctx, cfg, store, notify)
		running = false
		cycleCount++

		if err := WriteHealth(HealthStatus{
			LastPoll:   time.Now().UTC(),
			CycleCount: cycleCount,
			LastErrors: result.Errors,
			PID:        os.Getpid(),
		}); err != nil {
			slog.Error("failed to write health file", "error", err)
		}
	}

	// Run immediately on start.
	runCycle()

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("daemon stopping", "reason", ctx.Err())
			return ctx.Err()
		case <-ticker.C:
			if running {
				slog.Info("poll cycle still running, skipping tick")
				continue
			}
			runCycle()
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
