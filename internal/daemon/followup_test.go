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

func TestPlanUnreviewedOnGitHub(t *testing.T) {
	pr := github.PullRequest{Number: 1, HeadOID: "c", CommitOIDs: []string{"a", "b", "c"}}
	tests := []struct {
		name      string
		rec       *state.ReviewRecord
		pr        github.PullRequest
		wantKind  planKind
		wantAdopt bool
		wantCount int
		wantRewr  bool
	}{
		{name: "never reviewed", rec: nil, pr: pr, wantKind: planNew},
		{name: "head unknown", rec: &state.ReviewRecord{HeadOID: "a"}, pr: github.PullRequest{Number: 1}, wantKind: planSkip},
		{name: "legacy row adopts head", rec: &state.ReviewRecord{}, pr: pr, wantKind: planSkip, wantAdopt: true},
		{name: "reviewed at head", rec: &state.ReviewRecord{HeadOID: "c"}, pr: pr, wantKind: planSkip},
		{name: "dry-run follow-up", rec: &state.ReviewRecord{HeadOID: "a"}, pr: pr, wantKind: planFollowUp, wantCount: 2},
		{name: "rewritten", rec: &state.ReviewRecord{HeadOID: "gone"}, pr: pr, wantKind: planFollowUp, wantRewr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := planUnreviewedOnGitHub(tt.rec, tt.pr)
			if got.kind != tt.wantKind || got.adoptBaseline != tt.wantAdopt || got.newCommits != tt.wantCount || got.rewritten != tt.wantRewr {
				t.Errorf("got %+v", got)
			}
		})
	}
}

func TestPlanForCandidate(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	cand := func(lastOID string) github.FollowUpCandidate {
		return github.FollowUpCandidate{
			PullRequest:   github.PullRequest{Number: 1, HeadOID: "c", CommitOIDs: []string{"a", "b", "c"}},
			LastReviewAt:  t0,
			LastReviewOID: lastOID,
		}
	}
	tests := []struct {
		name      string
		rec       *state.ReviewRecord
		c         github.FollowUpCandidate
		wantKind  planKind
		wantPrev  string
		wantAdopt bool
	}{
		{name: "github review older than head", rec: nil, c: cand("a"), wantKind: planFollowUp, wantPrev: "a"},
		{name: "state already at head", rec: &state.ReviewRecord{HeadOID: "c", ReviewedAt: t0.Add(-time.Hour)}, c: cand("a"), wantKind: planSkip},
		{name: "newer state wins", rec: &state.ReviewRecord{HeadOID: "b", ReviewedAt: t0.Add(time.Minute)}, c: cand("a"), wantKind: planFollowUp, wantPrev: "b"},
		{name: "newer github review wins", rec: &state.ReviewRecord{HeadOID: "a", ReviewedAt: t0.Add(-time.Hour)}, c: cand("b"), wantKind: planFollowUp, wantPrev: "b"},
		{name: "null github commit falls back to state", rec: &state.ReviewRecord{HeadOID: "a"}, c: cand(""), wantKind: planFollowUp, wantPrev: "a"},
		{name: "null commit and legacy row adopts", rec: &state.ReviewRecord{}, c: cand(""), wantKind: planSkip, wantAdopt: true},
		{name: "null commit and no state skips", rec: nil, c: cand(""), wantKind: planSkip},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := planForCandidate(tt.rec, tt.c)
			if got.kind != tt.wantKind || got.prevOID != tt.wantPrev || got.adoptBaseline != tt.wantAdopt {
				t.Errorf("got %+v", got)
			}
		})
	}
}

// missingRepo makes phase 2 fail fast: claude cannot start in a directory that
// does not exist, so these tests never spawn a model run.
const missingRepo = "/nonexistent/pr-sentinel-test-repo"

func TestRunPollCycleWith_DryRunFollowsUpOnNewHead(t *testing.T) {
	store := testStore(t)
	cfg := testConfig(config.RepoConfig{Name: "o/r", Path: missingRepo, Mode: config.ModeDryRun})
	store.RecordReview(state.ReviewRecord{Repo: "o/r", PRNumber: 5, HeadOID: "a", ReviewedAt: time.Now().UTC()})

	fetcher := mockFetcher{prs: map[string][]github.PullRequest{
		"o/r": {{Repo: "o/r", Number: 5, HeadOID: "b", CommitOIDs: []string{"a", "b"}}},
	}}
	result := RunPollCycleWith(context.Background(), cfg, store, nil, fetcher)

	// Queued (then failed to start, by design): not skipped.
	if result.Skipped != 0 || result.Errors != 1 {
		t.Errorf("skipped=%d errors=%d, want 0/1 (follow-up queued)", result.Skipped, result.Errors)
	}
}

func TestRunPollCycleWith_LegacyRowAdoptsHead(t *testing.T) {
	store := testStore(t)
	cfg := testConfig(config.RepoConfig{Name: "o/r", Path: missingRepo, Mode: config.ModeDryRun})
	store.RecordReview(state.ReviewRecord{Repo: "o/r", PRNumber: 5, ReviewedAt: time.Now().UTC()})

	fetcher := mockFetcher{prs: map[string][]github.PullRequest{
		"o/r": {{Repo: "o/r", Number: 5, HeadOID: "b", CommitOIDs: []string{"a", "b"}}},
	}}
	result := RunPollCycleWith(context.Background(), cfg, store, nil, fetcher)

	if result.Skipped != 1 || result.Errors != 0 {
		t.Fatalf("skipped=%d errors=%d, want 1/0 (no rollout burst)", result.Skipped, result.Errors)
	}
	rec, _ := store.GetReview("o/r", 5)
	if rec.HeadOID != "b" {
		t.Errorf("baseline head = %q, want b", rec.HeadOID)
	}

	// Next cycle: same head, still skipped; a new head is a follow-up.
	fetcher.prs["o/r"][0].HeadOID, fetcher.prs["o/r"][0].CommitOIDs = "c", []string{"a", "b", "c"}
	result = RunPollCycleWith(context.Background(), cfg, store, nil, fetcher)
	if result.Errors != 1 {
		t.Errorf("after push: errors=%d, want 1 (follow-up queued)", result.Errors)
	}
}

func TestRunPollCycleWith_StateAtHeadSkipsGitHubCandidate(t *testing.T) {
	store := testStore(t)
	cfg := testConfig(config.RepoConfig{Name: "o/r", Path: missingRepo, Mode: config.ModeLive})
	store.RecordReview(state.ReviewRecord{Repo: "o/r", PRNumber: 5, HeadOID: "b", ReviewedAt: time.Now().UTC()})

	fetcher := mockFetcher{followUps: map[string][]github.FollowUpCandidate{
		"o/r": {{
			PullRequest:   github.PullRequest{Repo: "o/r", Number: 5, HeadOID: "b", CommitOIDs: []string{"a", "b"}},
			LastReviewAt:  time.Now().Add(-time.Hour),
			LastReviewOID: "a",
		}},
	}}
	result := RunPollCycleWith(context.Background(), cfg, store, nil, fetcher)
	if result.Skipped != 1 || result.Errors != 0 {
		t.Errorf("skipped=%d errors=%d, want 1/0", result.Skipped, result.Errors)
	}
}

func TestProcessReviewWith_PinsToHeadAndRecordsIt(t *testing.T) {
	store, gh := testStore(t), &mockGitHub{}
	repo := config.RepoConfig{Name: "o/r", Mode: config.ModeLive}

	if _, err := ProcessReviewWith(store, nil, PollOptions{}, repo, github.PullRequest{Number: 1, HeadOID: "abc"}, structuredResult(), gh); err != nil {
		t.Fatalf("ProcessReviewWith: %v", err)
	}
	if len(gh.pinned) != 1 || gh.pinned[0] != "abc" {
		t.Errorf("pinned = %v, want [abc]", gh.pinned)
	}
	rec, _ := store.GetReview("o/r", 1)
	if rec.HeadOID != "abc" {
		t.Errorf("recorded head = %q, want abc", rec.HeadOID)
	}

	// No head known: unpinned post, as before.
	gh2 := &mockGitHub{}
	if _, err := ProcessReviewWith(testStore(t), nil, PollOptions{}, repo, github.PullRequest{Number: 2}, structuredResult(), gh2); err != nil {
		t.Fatalf("ProcessReviewWith: %v", err)
	}
	if len(gh2.posted) != 1 || len(gh2.pinned) != 0 {
		t.Errorf("posted=%v pinned=%v, want one unpinned post", gh2.posted, gh2.pinned)
	}
}

// The GitHub side recognises pr-sentinel's own reviews by github.ReviewMarker.
// If the review format changes and drops it, follow-ups silently stop.
func TestReviewBodyCarriesReviewMarker(t *testing.T) {
	for _, disclosure := range []bool{true, false} {
		body := ReviewBody(PollOptions{AIDisclosure: disclosure, DisclosureText: "> AI"}, github.PullRequest{Author: "a"}, structuredResult())
		if !strings.Contains(body, github.ReviewMarker) {
			t.Errorf("disclosure=%v: body lacks %q:\n%s", disclosure, github.ReviewMarker, body)
		}
	}
}
