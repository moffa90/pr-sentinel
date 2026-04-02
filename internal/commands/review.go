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
	"github.com/moffa90/pr-sentinel/internal/publisher"
	"github.com/moffa90/pr-sentinel/internal/reviewer"
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

	// Build review prompt
	params := reviewer.ReviewParams{
		Repo:     repo,
		PRNumber: prNumber,
	}
	prompt := reviewer.BuildReviewPrompt(params)

	// Run Claude review
	fmt.Printf("%s Running Claude review...\n", ui.IconDot)

	timeout := cfg.ReviewTimeout
	if timeout == 0 {
		timeout = reviewer.DefaultTimeout
	}

	result := reviewer.RunReview(
		context.Background(),
		repoConf.Path,
		prompt,
		cfg.Review.Instructions,
		repoConf.ReviewInstructions,
		timeout,
	)

	if result.Error != nil {
		return fmt.Errorf("review failed: %w", result.Error)
	}

	fmt.Printf("  %s Review complete (%s)\n\n", ui.IconCheck, result.Duration.Truncate(1e8))

	// Show review output
	fmt.Println(ui.Separator("Review Output"))
	fmt.Println()
	fmt.Println(result.Output)
	fmt.Println()

	// Build final body with disclosure
	body := publisher.BuildReviewBody(result.Output, cfg.Review.AIDisclosure, cfg.Review.DisclosureText, "")

	// Handle mode
	if mode == config.ModeLive {
		if !confirmPost() {
			fmt.Println(ui.MutedStyle.Render("Review not posted."))
			return nil
		}

		fmt.Printf("%s Posting review to GitHub...\n", ui.IconDot)
		if err := publisher.PostLiveReview(repo, prNumber, body); err != nil {
			return fmt.Errorf("posting review: %w", err)
		}
		fmt.Printf("  %s Review posted to %s\n", ui.IconCheck, ui.PRReference(repo, prNumber))
	} else {
		// Dry-run: save to disk
		reviewsDir := filepath.Join(config.ConfigDir(), "reviews")
		savePath, err := publisher.SaveDryRunReview(publisher.SaveParams{
			ReviewsDir: reviewsDir,
			Repo:       repo,
			PRNumber:   prNumber,
			Body:       body,
		})
		if err != nil {
			return fmt.Errorf("saving review: %w", err)
		}
		fmt.Printf("  %s Review saved to %s\n", ui.IconCheck, ui.MutedStyle.Render(savePath))
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
