package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/moffa90/pr-sentinel/internal/config"
	"github.com/moffa90/pr-sentinel/internal/daemon"
	ghclient "github.com/moffa90/pr-sentinel/internal/github"
	"github.com/moffa90/pr-sentinel/internal/reviewer"
	"github.com/moffa90/pr-sentinel/internal/state"
	"github.com/moffa90/pr-sentinel/internal/ui"
	"github.com/spf13/cobra"
)

var prURLRegex = regexp.MustCompile(`github\.com/([^/]+/[^/]+)/pull/(\d+)`)

// NewReviewCmd creates the review command for one-shot PR review.
func NewReviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "review <pr-url>",
		Short: "Review a single pull request",
		Long:  "Fetches the PR diff, runs a Claude Code review, and optionally posts the result to GitHub.",
		Args:  cobra.ExactArgs(1),
		RunE:  runReview,
	}
}

func runReview(cmd *cobra.Command, args []string) error {
	prURL := args[0]

	// Parse PR URL
	matches := prURLRegex.FindStringSubmatch(prURL)
	if matches == nil {
		return fmt.Errorf("invalid PR URL — expected format: https://github.com/owner/repo/pull/123")
	}
	repo := matches[1]
	prNumber, _ := strconv.ParseInt(matches[2], 10, 64)

	fmt.Println(ui.Separator("Review"))
	fmt.Printf("%s Repo: %s  PR: #%d\n", ui.IconDot, ui.BrandStyle.Render(repo), prNumber)

	// Load config
	cfg, err := config.Load(config.DefaultConfigPath())
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Find repo config or auto-detect
	repoConf, found := cfg.FindRepo(repo)
	if !found {
		// Auto-detect: look in repos_dir
		reposDir := config.ExpandPath(cfg.ReposDir)
		parts := strings.SplitN(repo, "/", 2)
		if len(parts) == 2 {
			candidatePath := filepath.Join(reposDir, parts[1])
			if info, statErr := os.Stat(candidatePath); statErr == nil && info.IsDir() {
				repoConf = config.RepoConfig{
					Name: repo,
					Path: candidatePath,
					Mode: config.ModeDryRun,
				}
				found = true
				fmt.Printf("%s Auto-detected repo at %s\n", ui.IconDot, ui.MutedStyle.Render(candidatePath))
			}
		}
	}

	if !found {
		return fmt.Errorf("repo %q not found in config and could not auto-detect — run `pr-sentinel init` or add it manually", repo)
	}

	mode := repoConf.Mode
	if mode == "" {
		mode = config.ModeDryRun
	}
	fmt.Printf("%s Mode: %s\n\n", ui.IconDot, ui.ModeBadge(mode))

	// Fetch PR metadata so the prompt, body, and state match the daemon
	pr, err := ghclient.GetPR(repo, prNumber)
	if err != nil {
		return fmt.Errorf("fetching PR: %w", err)
	}
	fmt.Printf("%s %s by @%s (%d files, +%d/-%d)\n\n", ui.IconDot, pr.Title, pr.Author, pr.Files, pr.Additions, pr.Deletions)
	if pr.IsDraft {
		fmt.Println(ui.MutedStyle.Render("  Note: PR is a draft (the daemon skips drafts)."))
	}

	store, err := state.Open(state.DefaultDBPath())
	if err != nil {
		return fmt.Errorf("opening state store: %w", err)
	}
	defer store.Close()

	// Previously reviewed PRs get the daemon's follow-up prompt with the prior review.
	var prompt string
	if prev, prevErr := store.GetReview(repo, pr.Number); prevErr == nil {
		fmt.Printf("%s Previously reviewed %s, running follow-up review\n", ui.IconDot, prev.ReviewedAt.Local().Format("2006-01-02 15:04"))
		prompt = reviewer.BuildFollowUpPrompt(reviewer.FollowUpParams{
			Repo:           repo,
			PRNumber:       pr.Number,
			PRTitle:        pr.Title,
			PRAuthor:       pr.Author,
			Files:          pr.Files,
			Adds:           pr.Additions,
			Dels:           pr.Deletions,
			PreviousReview: prev.ReviewOutput,
		})
	} else {
		prompt = reviewer.BuildReviewPrompt(reviewer.ReviewParams{
			Repo:     repo,
			PRNumber: pr.Number,
			PRTitle:  pr.Title,
			PRAuthor: pr.Author,
			Files:    pr.Files,
			Adds:     pr.Additions,
			Dels:     pr.Deletions,
		})
	}

	opts := daemon.PollOptionsFromConfig(cfg)
	opts.SkipDailyCount = true
	if opts.ReviewTimeout == 0 {
		opts.ReviewTimeout = reviewer.DefaultTimeout
	}

	// Run Claude review
	fmt.Printf("%s Running Claude review...\n", ui.IconDot)

	result := reviewer.RunReviewWithModel(
		context.Background(),
		config.ExpandPath(repoConf.Path),
		prompt,
		opts.ReviewInstructions,
		repoConf.ReviewInstructions,
		opts.ReviewTimeout,
		opts.Model,
	)

	if result.Error != nil {
		return fmt.Errorf("review failed: %w", result.Error)
	}

	fmt.Printf("  %s Review complete (%s, models: %s)\n\n", ui.IconCheck, result.Duration.Truncate(1e8), strings.Join(result.Models, ", "))

	// Show the exact body that will be posted or saved
	fmt.Println(ui.Separator("Review Output"))
	fmt.Println()
	fmt.Println(daemon.ReviewBody(opts, pr, result))
	fmt.Println()

	if mode == config.ModeLive && !confirmPost() {
		fmt.Println(ui.MutedStyle.Render("Review not posted."))
		return nil
	}

	repoConf.Mode = mode
	outcome, err := daemon.ProcessReview(store, daemon.BuildNotifier(cfg), opts, repoConf, pr, result)
	if err != nil {
		return err
	}

	if outcome.Posted {
		fmt.Printf("  %s Review posted to %s\n", ui.IconCheck, ui.PRReference(repo, prNumber))
	} else {
		fmt.Printf("  %s Review saved to %s\n", ui.IconCheck, ui.MutedStyle.Render(outcome.ReviewPath))
	}
	if outcome.AutoMerge != "" {
		fmt.Printf("  %s Auto-merge: %s\n", ui.IconDot, outcome.AutoMerge)
	}
	if outcome.Issue != "" {
		fmt.Printf("  %s Issue: %s\n", ui.IconDot, outcome.Issue)
	}

	return nil
}

// confirmPost asks the user to confirm posting and defaults to yes.
func confirmPost() bool {
	fmt.Print("Post this review to GitHub? [Y/n] ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "" || input == "y" || input == "yes"
}
