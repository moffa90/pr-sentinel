package daemon

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/moffa90/pr-sentinel/internal/config"
	"github.com/moffa90/pr-sentinel/internal/github"
	"github.com/moffa90/pr-sentinel/internal/state"
)

// mockFetcher implements PRFetcher for testing.
type mockFetcher struct {
	prs       map[string][]github.PullRequest
	followUps map[string][]github.FollowUpCandidate
	err       error
}

func (m mockFetcher) FetchOpenPRs(repo string, _ string) ([]github.PullRequest, []github.FollowUpCandidate, error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.prs[repo], m.followUps[repo], nil
}

func testConfig(repos ...config.RepoConfig) config.Config {
	return config.Config{
		PollInterval:       10 * time.Minute,
		MaxReviewsPerCycle: 5,
		MaxReviewsPerDay:   20,
		MaxParallelReviews: 1,
		ReposDir:           "/tmp",
		ReviewTimeout:      5 * time.Minute,
		GitHubUser:         "testuser",
		Repos:              repos,
	}
}

func testStore(t *testing.T) *state.Store {
	t.Helper()
	s, err := state.Open(":memory:")
	if err != nil {
		t.Fatalf("open in-memory store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRunPollCycleWith_NoPRs(t *testing.T) {
	store := testStore(t)
	cfg := testConfig(config.RepoConfig{
		Name: "owner/repo",
		Path: "/tmp/repo",
		Mode: config.ModeDryRun,
	})

	fetcher := mockFetcher{
		prs: map[string][]github.PullRequest{},
	}

	result := RunPollCycleWith(context.Background(), cfg, store, nil, fetcher)

	if result.Reviewed != 0 {
		t.Errorf("Reviewed = %d, want 0", result.Reviewed)
	}
	if result.Errors != 0 {
		t.Errorf("Errors = %d, want 0", result.Errors)
	}
}

func TestRunPollCycleWith_FetchError(t *testing.T) {
	store := testStore(t)
	cfg := testConfig(config.RepoConfig{
		Name: "owner/repo",
		Path: "/tmp/repo",
		Mode: config.ModeDryRun,
	})

	fetcher := mockFetcher{
		err: fmt.Errorf("network error"),
	}

	result := RunPollCycleWith(context.Background(), cfg, store, nil, fetcher)

	if result.Errors != 1 {
		t.Errorf("Errors = %d, want 1", result.Errors)
	}
}

func TestRunPollCycleWith_SkipsAlreadyReviewed(t *testing.T) {
	store := testStore(t)
	cfg := testConfig(config.RepoConfig{
		Name: "owner/repo",
		Path: "/tmp/repo",
		Mode: config.ModeDryRun,
	})

	// Record a review for PR #10
	store.RecordReview(state.ReviewRecord{
		Repo:       "owner/repo",
		PRNumber:   10,
		PRTitle:    "old review",
		ReviewedAt: time.Now().UTC(),
	})

	fetcher := mockFetcher{
		prs: map[string][]github.PullRequest{
			"owner/repo": {
				{Repo: "owner/repo", Number: 10, Title: "already reviewed", Author: "alice"},
			},
		},
	}

	result := RunPollCycleWith(context.Background(), cfg, store, nil, fetcher)

	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Skipped)
	}
	if result.Reviewed != 0 {
		t.Errorf("Reviewed = %d, want 0", result.Reviewed)
	}
}

func TestRunPollCycleWith_RespectsPerCycleLimit(t *testing.T) {
	store := testStore(t)
	cfg := testConfig(config.RepoConfig{
		Name: "owner/repo",
		Path: "/tmp/repo",
		Mode: config.ModeDryRun,
	})
	cfg.MaxReviewsPerCycle = 1

	fetcher := mockFetcher{
		prs: map[string][]github.PullRequest{
			"owner/repo": {
				{Repo: "owner/repo", Number: 1, Title: "pr1", Author: "alice"},
				{Repo: "owner/repo", Number: 2, Title: "pr2", Author: "bob"},
			},
		},
	}

	result := RunPollCycleWith(context.Background(), cfg, store, nil, fetcher)

	// One should be queued for review, one skipped due to limit
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Skipped)
	}
}

func TestRunPollCycleWith_ContextCancelled(t *testing.T) {
	store := testStore(t)
	cfg := testConfig(config.RepoConfig{
		Name: "owner/repo",
		Path: "/tmp/repo",
		Mode: config.ModeDryRun,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	fetcher := mockFetcher{
		prs: map[string][]github.PullRequest{
			"owner/repo": {
				{Repo: "owner/repo", Number: 1, Title: "pr1", Author: "alice"},
			},
		},
	}

	result := RunPollCycleWith(ctx, cfg, store, nil, fetcher)

	// Nothing should be processed when context is already cancelled
	if result.Reviewed != 0 {
		t.Errorf("Reviewed = %d, want 0", result.Reviewed)
	}
}

func TestRunPollCycleWith_MultipleRepos(t *testing.T) {
	store := testStore(t)
	cfg := testConfig(
		config.RepoConfig{Name: "owner/repo-a", Path: "/tmp/repo-a", Mode: config.ModeDryRun},
		config.RepoConfig{Name: "owner/repo-b", Path: "/tmp/repo-b", Mode: config.ModeDryRun},
	)

	fetcher := mockFetcher{
		prs: map[string][]github.PullRequest{
			"owner/repo-a": {
				{Repo: "owner/repo-a", Number: 1, Title: "pr in A", Author: "alice"},
			},
			"owner/repo-b": {
				{Repo: "owner/repo-b", Number: 2, Title: "pr in B", Author: "bob"},
			},
		},
	}

	result := RunPollCycleWith(context.Background(), cfg, store, nil, fetcher)

	// Both should error since claude CLI won't be available in test,
	// but the work collection should have found 2 items
	// (errors come from Phase 2 review execution, not collection)
	if result.Errors+result.Reviewed+result.DryRun != 2 {
		t.Errorf("expected 2 total work items processed, got errors=%d reviewed=%d dry_run=%d",
			result.Errors, result.Reviewed, result.DryRun)
	}
}
