package daemon

import (
	"os"
	"strings"
	"testing"

	"github.com/moffa90/pr-sentinel/internal/config"
	"github.com/moffa90/pr-sentinel/internal/github"
	"github.com/moffa90/pr-sentinel/internal/reviewer"
)

func structuredResult() reviewer.ReviewResult {
	review := &reviewer.StructuredReview{
		Verdict:  reviewer.VerdictApprove,
		Summary:  "Looks good.",
		Findings: []reviewer.Finding{{Severity: "LOW", File: "main.go", Line: 3, Message: "rename var"}},
	}
	return reviewer.ReviewResult{
		Output: `{"verdict":"approve","summary":"Looks good.","findings":[]}`,
		Review: review,
	}
}

func TestReviewBody(t *testing.T) {
	pr := github.PullRequest{Number: 1, Author: "alice"}
	opts := PollOptions{AIDisclosure: true, DisclosureText: "> AI review"}

	t.Run("structured review renders markdown, not JSON", func(t *testing.T) {
		body := ReviewBody(opts, pr, structuredResult())
		if strings.Contains(body, `"verdict"`) {
			t.Errorf("body contains raw JSON:\n%s", body)
		}
		for _, want := range []string{"> AI review", "**Verdict: Approved**", "`main.go:3`", "cc @alice"} {
			if !strings.Contains(body, want) {
				t.Errorf("body missing %q:\n%s", want, body)
			}
		}
	})

	t.Run("unparsed review falls back to raw output", func(t *testing.T) {
		body := ReviewBody(opts, pr, reviewer.ReviewResult{Output: "plain text review"})
		if !strings.Contains(body, "plain text review") {
			t.Errorf("body missing raw output:\n%s", body)
		}
	})
}

func TestIsSelfAuthored(t *testing.T) {
	tests := []struct {
		user, author string
		want         bool
	}{
		{"moffa90", "moffa90", true},
		{"moffa90", "Moffa90", true},
		{"moffa90", "alice", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got := isSelfAuthored(PollOptions{GitHubUser: tt.user}, github.PullRequest{Author: tt.author})
		if got != tt.want {
			t.Errorf("isSelfAuthored(user=%q, author=%q) = %v, want %v", tt.user, tt.author, got, tt.want)
		}
	}
}

func TestAutoMergeGates(t *testing.T) {
	repo := config.RepoConfig{Name: "o/r", Mode: config.ModeDryRun, AutoMerge: config.AutoMergeConfig{Enabled: true, Strategy: "squash"}}
	pr := github.PullRequest{Number: 1}

	blocking := &reviewer.StructuredReview{Findings: []reviewer.Finding{{Severity: "MEDIUM"}}}
	if got := autoMerge(&mockGitHub{}, repo, pr, blocking); !strings.HasPrefix(got, "Skipped (has HIGH/MEDIUM") {
		t.Errorf("blocking findings: got %q", got)
	}

	labelled := repo
	labelled.AutoMerge.RequireLabel = "automerge"
	if got := autoMerge(&mockGitHub{}, labelled, pr, nil); !strings.HasPrefix(got, "Skipped (missing label") {
		t.Errorf("missing label: got %q", got)
	}

	pr.Labels = []string{"AutoMerge"}
	if got := autoMerge(&mockGitHub{}, labelled, pr, nil); got != "Would merge (squash)" {
		t.Errorf("label present, dry-run: got %q", got)
	}
}

func TestProcessReview_DryRunRecordsState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	store := testStore(t)
	repo := config.RepoConfig{Name: "o/r", Mode: config.ModeDryRun}
	pr := github.PullRequest{Number: 9, Title: "t", Author: "alice"}

	out, err := ProcessReview(store, nil, PollOptions{}, repo, pr, structuredResult())
	if err != nil {
		t.Fatalf("ProcessReview: %v", err)
	}
	if out.Posted {
		t.Error("dry-run should not post")
	}
	if !strings.HasPrefix(out.ReviewPath, home) {
		t.Errorf("ReviewPath = %q, want under %q", out.ReviewPath, home)
	}
	saved, err := os.ReadFile(out.ReviewPath)
	if err != nil {
		t.Fatalf("reading saved review: %v", err)
	}
	if strings.Contains(string(saved), `"verdict"`) {
		t.Errorf("saved review contains raw JSON:\n%s", saved)
	}

	reviewed, err := store.HasReviewed("o/r", 9)
	if err != nil || !reviewed {
		t.Errorf("HasReviewed = %v, err = %v; want true", reviewed, err)
	}
	rec, err := store.GetReview("o/r", 9)
	if err != nil {
		t.Fatalf("GetReview: %v", err)
	}
	if rec.FindingsSummary != "1 LOW" {
		t.Errorf("FindingsSummary = %q, want %q", rec.FindingsSummary, "1 LOW")
	}
}

func TestProcessReviewWith_Live(t *testing.T) {
	approve := func(findings ...reviewer.Finding) reviewer.ReviewResult {
		return reviewer.ReviewResult{
			Output: "{}",
			Review: &reviewer.StructuredReview{Verdict: reviewer.VerdictApprove, Summary: "ok", Findings: findings},
		}
	}
	repo := config.RepoConfig{
		Name:      "o/r",
		Mode:      config.ModeLive,
		AutoMerge: config.AutoMergeConfig{Enabled: true, Strategy: "squash"},
		Issues:    config.IssuesConfig{Enabled: true, MinSeverity: "LOW"},
	}
	opts := PollOptions{GitHubUser: "me"}

	t.Run("posts, merges, opens issue, records", func(t *testing.T) {
		store, gh := testStore(t), &mockGitHub{}
		pr := github.PullRequest{Number: 1, Author: "alice"}

		out, err := ProcessReviewWith(store, nil, opts, repo, pr, approve(reviewer.Finding{Severity: "LOW", File: "a.go", Message: "nit"}), gh)
		if err != nil {
			t.Fatalf("ProcessReviewWith: %v", err)
		}
		if !out.Posted || len(gh.posted) != 1 || gh.posted[0] != "approve" {
			t.Errorf("posted=%v verdicts=%v", out.Posted, gh.posted)
		}
		if gh.merged != 1 || out.AutoMerge != "Enabled (squash)" {
			t.Errorf("merged=%d status=%q", gh.merged, out.AutoMerge)
		}
		if out.Issue != "Created #100" {
			t.Errorf("issue status = %q", out.Issue)
		}
		if reviewed, _ := store.HasReviewed("o/r", 1); !reviewed {
			t.Error("review not recorded")
		}
	})

	t.Run("own PR posts as comment and skips auto-merge", func(t *testing.T) {
		store, gh := testStore(t), &mockGitHub{}
		pr := github.PullRequest{Number: 2, Author: "Me"}

		out, err := ProcessReviewWith(store, nil, opts, repo, pr, approve(), gh)
		if err != nil {
			t.Fatalf("ProcessReviewWith: %v", err)
		}
		if len(gh.posted) != 1 || gh.posted[0] != "comment" {
			t.Errorf("verdicts = %v, want [comment]", gh.posted)
		}
		if gh.merged != 0 || out.AutoMerge != "" {
			t.Errorf("merged=%d status=%q, want no auto-merge", gh.merged, out.AutoMerge)
		}
	})
}
