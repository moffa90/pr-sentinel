package daemon

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/moffa90/pr-sentinel/internal/config"
	"github.com/moffa90/pr-sentinel/internal/github"
	"github.com/moffa90/pr-sentinel/internal/state"
)

func TestAttemptGate(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		a      state.Attempt
		found  bool
		wantOK bool
		reason string
	}{
		{name: "never tried", found: false, wantOK: true},
		{name: "record with zero attempts", a: state.Attempt{Attempts: 0, LastAttemptAt: now}, found: true, wantOK: true},
		{name: "succeeded", a: state.Attempt{Attempts: 1, Succeeded: true, LastAttemptAt: now.Add(-48 * time.Hour)}, found: true, reason: "already reviewed"},
		{name: "first failure, inside backoff", a: state.Attempt{Attempts: 1, LastAttemptAt: now.Add(-5 * time.Minute)}, found: true, reason: "backing off"},
		{name: "first failure, backoff over", a: state.Attempt{Attempts: 1, LastAttemptAt: now.Add(-11 * time.Minute)}, found: true, wantOK: true},
		{name: "second failure doubles", a: state.Attempt{Attempts: 2, LastAttemptAt: now.Add(-15 * time.Minute)}, found: true, reason: "backing off"},
		{name: "second failure, backoff over", a: state.Attempt{Attempts: 2, LastAttemptAt: now.Add(-21 * time.Minute)}, found: true, wantOK: true},
		{name: "parked", a: state.Attempt{Attempts: 3, LastAttemptAt: now.Add(-72 * time.Hour)}, found: true, reason: "parked"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, why := attemptGate(tt.a, tt.found, now)
			if ok != tt.wantOK || !strings.Contains(why, tt.reason) {
				t.Errorf("got ok=%v why=%q, want ok=%v reason~%q", ok, why, tt.wantOK, tt.reason)
			}
		})
	}
}

// A run that fails still costs money: it is recorded as an attempt and counted
// against the daily budget, and the next cycle backs off instead of paying again.
func TestRunPollCycleWith_FailedRunIsCountedAndBacksOff(t *testing.T) {
	store := testStore(t)
	cfg := testConfig(config.RepoConfig{Name: "o/r", Path: missingRepo, Mode: config.ModeDryRun})
	fetcher := mockFetcher{prs: map[string][]github.PullRequest{
		"o/r": {{Repo: "o/r", Number: 7, HeadOID: "h1", CommitOIDs: []string{"h1"}}},
	}}
	today := time.Now().UTC().Format("2006-01-02")

	r1 := RunPollCycleWith(context.Background(), cfg, store, nil, fetcher)
	if r1.Errors != 1 {
		t.Fatalf("first cycle errors = %d, want 1 (run failed by design)", r1.Errors)
	}
	a, ok, err := store.GetAttempt("o/r", 7, "h1")
	if err != nil || !ok || a.Attempts != 1 || a.Succeeded || a.LastError == "" {
		t.Fatalf("attempt = %+v ok=%v err=%v, want 1 failed attempt with an error", a, ok, err)
	}
	if count, _ := store.GetDailyCount(today); count != 1 {
		t.Errorf("daily count = %d, want 1 (failed runs count)", count)
	}

	r2 := RunPollCycleWith(context.Background(), cfg, store, nil, fetcher)
	if r2.Errors != 0 || r2.Skipped != 1 {
		t.Errorf("second cycle errors=%d skipped=%d, want 0/1 (backing off)", r2.Errors, r2.Skipped)
	}
	if a, _, _ := store.GetAttempt("o/r", 7, "h1"); a.Attempts != 1 {
		t.Errorf("attempts = %d after backoff cycle, want 1", a.Attempts)
	}
	if count, _ := store.GetDailyCount(today); count != 1 {
		t.Errorf("daily count = %d, want 1 (no run during backoff)", count)
	}

	// A new head starts clean even while the old one is parked.
	for i := 0; i < maxAttemptsPerHead; i++ {
		if err := store.BeginAttempt("o/r", 7, "h1"); err != nil {
			t.Fatal(err)
		}
	}
	fetcher.prs["o/r"][0].HeadOID, fetcher.prs["o/r"][0].CommitOIDs = "h2", []string{"h1", "h2"}
	if r3 := RunPollCycleWith(context.Background(), cfg, store, nil, fetcher); r3.Errors != 1 {
		t.Errorf("new head: errors = %d, want 1 (attempted)", r3.Errors)
	}
}

// Even if recording the review failed, a head whose review was posted is never
// paid for again by the daemon.
func TestRunPollCycleWith_SucceededHeadNotRetried(t *testing.T) {
	store := testStore(t)
	cfg := testConfig(config.RepoConfig{Name: "o/r", Path: missingRepo, Mode: config.ModeDryRun})
	if err := store.BeginAttempt("o/r", 8, "h1"); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishAttempt("o/r", 8, "h1", true, ""); err != nil {
		t.Fatal(err)
	}

	fetcher := mockFetcher{prs: map[string][]github.PullRequest{
		"o/r": {{Repo: "o/r", Number: 8, HeadOID: "h1", CommitOIDs: []string{"h1"}}},
	}}
	if r := RunPollCycleWith(context.Background(), cfg, store, nil, fetcher); r.Errors != 0 || r.Skipped != 1 {
		t.Errorf("errors=%d skipped=%d, want 0/1", r.Errors, r.Skipped)
	}
}
