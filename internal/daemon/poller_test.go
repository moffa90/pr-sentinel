package daemon

import (
	"testing"
	"time"

	"github.com/moffa90/pr-sentinel/internal/config"
)

func TestShouldSkip_UnderBothLimits(t *testing.T) {
	opts := PollOptions{
		MaxReviewsPerCycle: 5,
		MaxReviewsPerDay:   20,
	}
	if shouldSkip(opts, 2, 10) {
		t.Error("expected shouldSkip to return false when under both limits")
	}
}

func TestShouldSkip_AtCycleLimit(t *testing.T) {
	opts := PollOptions{
		MaxReviewsPerCycle: 5,
		MaxReviewsPerDay:   20,
	}
	if !shouldSkip(opts, 5, 10) {
		t.Error("expected shouldSkip to return true when at cycle limit")
	}
}

func TestShouldSkip_AtDailyLimit(t *testing.T) {
	opts := PollOptions{
		MaxReviewsPerCycle: 5,
		MaxReviewsPerDay:   20,
	}
	if !shouldSkip(opts, 2, 20) {
		t.Error("expected shouldSkip to return true when at daily limit")
	}
}

func TestPollOptionsFromConfig(t *testing.T) {
	cfg := config.Config{
		MaxReviewsPerCycle: 10,
		MaxReviewsPerDay:   50,
		MaxParallelReviews: 4,
		ReviewTimeout:      3 * time.Minute,
		GitHubUser:         "testuser",
		Review: config.ReviewConfig{
			Instructions:   "be thorough",
			AIDisclosure:   true,
			DisclosureText: "AI review",
		},
	}

	opts := PollOptionsFromConfig(cfg)

	if opts.MaxReviewsPerCycle != 10 {
		t.Errorf("MaxReviewsPerCycle = %d, want 10", opts.MaxReviewsPerCycle)
	}
	if opts.MaxReviewsPerDay != 50 {
		t.Errorf("MaxReviewsPerDay = %d, want 50", opts.MaxReviewsPerDay)
	}
	if opts.ReviewTimeout != 3*time.Minute {
		t.Errorf("ReviewTimeout = %v, want 3m", opts.ReviewTimeout)
	}
	if opts.GitHubUser != "testuser" {
		t.Errorf("GitHubUser = %q, want %q", opts.GitHubUser, "testuser")
	}
	if opts.ReviewInstructions != "be thorough" {
		t.Errorf("ReviewInstructions = %q, want %q", opts.ReviewInstructions, "be thorough")
	}
	if opts.DisclosureText != "AI review" {
		t.Errorf("DisclosureText = %q, want %q", opts.DisclosureText, "AI review")
	}
	if !opts.AIDisclosure {
		t.Error("AIDisclosure should be true")
	}
	if opts.MaxParallelReviews != 4 {
		t.Errorf("MaxParallelReviews = %d, want 4", opts.MaxParallelReviews)
	}
}
