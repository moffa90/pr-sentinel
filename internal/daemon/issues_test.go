package daemon

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/moffa90/pr-sentinel/internal/config"
	"github.com/moffa90/pr-sentinel/internal/github"
	"github.com/moffa90/pr-sentinel/internal/reviewer"
)

type mockGitHub struct {
	created    int
	commented  []string
	posted     []string // verdicts passed to PostReview
	merged     int
	lastBody   string
	lastLabel  []string
	issueState string // returned by GetIssueState; "" means OPEN
	stateErr   error
	createURL  string // overrides the URL returned by CreateIssue
	createErr  error
	err        error // returned by comment/post/merge
}

func (m *mockGitHub) PostReview(_ string, _ int64, body, verdict string) error {
	if m.err != nil {
		return m.err
	}
	m.posted = append(m.posted, verdict)
	m.lastBody = body
	return nil
}

func (m *mockGitHub) EnableAutoMerge(string, int64, string, bool) error {
	if m.err != nil {
		return m.err
	}
	m.merged++
	return nil
}

func (m *mockGitHub) CreateIssue(repo, title, body string, labels []string) (int64, string, error) {
	if m.createErr != nil {
		return 0, m.createURL, m.createErr
	}
	m.created++
	m.lastBody = body
	m.lastLabel = labels
	return 100 + int64(m.created) - 1, "https://github.com/" + repo + "/issues/100", nil
}

func (m *mockGitHub) CommentOnIssue(_, ref, body string) error {
	if m.err != nil {
		return m.err
	}
	m.commented = append(m.commented, ref)
	m.lastBody = body
	return nil
}

func (m *mockGitHub) GetIssueState(string, string) (string, error) {
	if m.stateErr != nil {
		return "", m.stateErr
	}
	if m.issueState == "" {
		return "OPEN", nil
	}
	return m.issueState, nil
}

func TestQualifyingFindings(t *testing.T) {
	findings := []reviewer.Finding{
		{Severity: "HIGH", File: "a.go"},
		{Severity: "MEDIUM", File: "b.go"},
		{Severity: "LOW", File: "c.go"},
		{Severity: "bogus", File: "d.go"},
	}
	tests := []struct {
		min  string
		want int
	}{
		{"", 1}, // defaults to HIGH
		{"HIGH", 1},
		{"medium", 2},
		{"LOW", 3},
	}
	for _, tt := range tests {
		t.Run("min="+tt.min, func(t *testing.T) {
			got := qualifyingFindings(config.IssuesConfig{MinSeverity: tt.min}, findings)
			if len(got) != tt.want {
				t.Errorf("got %d findings, want %d", len(got), tt.want)
			}
		})
	}
}

func TestBuildIssueBody(t *testing.T) {
	pr := github.PullRequest{Number: 7, Author: "alice"}
	findings := []reviewer.Finding{
		{Severity: "HIGH", File: "main.go", Line: 12, Message: "nil deref"},
		{Severity: "HIGH", File: "util.go", Message: "leak"},
	}

	body := buildIssueBody(pr, findings, false)
	for _, want := range []string{"#7 (by @alice) was approved with 2 finding(s)", "- [ ] **HIGH** `main.go:12` — nil deref", "`util.go` — leak"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}

	later := buildIssueBody(pr, findings, true)
	if !strings.Contains(later, "A later approved review of #7") {
		t.Errorf("later body missing header:\n%s", later)
	}

	pinging := buildIssueBody(pr, []reviewer.Finding{{Severity: "LOW", File: "a.go", Message: "ask @bob, see #12"}}, false)
	if strings.Contains(pinging, "@bob") || strings.Contains(pinging, "#12") {
		t.Errorf("finding text should not ping or cross-link:\n%s", pinging)
	}
}

func TestHandleIssues(t *testing.T) {
	pr := github.PullRequest{Number: 5, Title: "t", Author: "bob"}
	high := &reviewer.StructuredReview{Verdict: reviewer.VerdictApprove, Findings: []reviewer.Finding{{Severity: "HIGH", File: "x.go", Message: "bad"}}}
	lowOnly := &reviewer.StructuredReview{Verdict: reviewer.VerdictApprove, Findings: []reviewer.Finding{{Severity: "LOW", File: "x.go", Message: "nit"}}}
	liveRepo := config.RepoConfig{Name: "o/r", Mode: config.ModeLive, Issues: config.IssuesConfig{Enabled: true, Labels: []string{"pr-sentinel"}}}

	t.Run("disabled does nothing", func(t *testing.T) {
		store, gh := testStore(t), &mockGitHub{}
		repo := liveRepo
		repo.Issues.Enabled = false
		if got := handleIssues(store, gh, repo, pr, high); got != "" || gh.created != 0 {
			t.Errorf("status=%q created=%d", got, gh.created)
		}
	})

	t.Run("nil review does nothing", func(t *testing.T) {
		store, gh := testStore(t), &mockGitHub{}
		if got := handleIssues(store, gh, liveRepo, pr, nil); got != "" {
			t.Errorf("status=%q", got)
		}
	})

	for _, verdict := range []reviewer.Verdict{reviewer.VerdictComment, reviewer.VerdictRequestChanges} {
		t.Run("verdict "+string(verdict)+" does nothing", func(t *testing.T) {
			store, gh := testStore(t), &mockGitHub{}
			review := &reviewer.StructuredReview{Verdict: verdict, Findings: high.Findings}
			if got := handleIssues(store, gh, liveRepo, pr, review); got != "" || gh.created != 0 {
				t.Errorf("status=%q created=%d", got, gh.created)
			}
		})
	}

	t.Run("fresh claim elsewhere skips", func(t *testing.T) {
		store, gh := testStore(t), &mockGitHub{}
		if claimed, _ := store.ClaimIssue("o/r", 5, time.Hour); !claimed {
			t.Fatal("setup claim failed")
		}
		if got := handleIssues(store, gh, liveRepo, pr, high); got != "Skipped (issue creation in progress)" {
			t.Errorf("status = %q", got)
		}
		if gh.created != 0 {
			t.Errorf("created=%d, want 0", gh.created)
		}
	})

	t.Run("create failure releases claim", func(t *testing.T) {
		store, gh := testStore(t), &mockGitHub{createErr: errors.New("boom")}
		handleIssues(store, gh, liveRepo, pr, high)
		if _, found, _ := store.GetIssue("o/r", 5); found {
			t.Error("claim should be released after failed create")
		}
		gh.createErr = nil
		if got := handleIssues(store, gh, liveRepo, pr, high); got != "Created #100" {
			t.Errorf("retry status = %q", got)
		}
	})

	t.Run("below threshold does nothing", func(t *testing.T) {
		store, gh := testStore(t), &mockGitHub{}
		if got := handleIssues(store, gh, liveRepo, pr, lowOnly); got != "" || gh.created != 0 {
			t.Errorf("status=%q created=%d", got, gh.created)
		}
	})

	t.Run("creates then comments on follow-up", func(t *testing.T) {
		store, gh := testStore(t), &mockGitHub{}

		if got := handleIssues(store, gh, liveRepo, pr, high); got != "Created #100" {
			t.Fatalf("first status = %q", got)
		}
		if gh.created != 1 || len(gh.lastLabel) != 1 || gh.lastLabel[0] != "pr-sentinel" {
			t.Fatalf("created=%d labels=%v", gh.created, gh.lastLabel)
		}

		if got := handleIssues(store, gh, liveRepo, pr, high); got != "Commented on #100" {
			t.Fatalf("second status = %q", got)
		}
		if gh.created != 1 || len(gh.commented) != 1 || gh.commented[0] != "100" {
			t.Errorf("created=%d commented=%v", gh.created, gh.commented)
		}
	})

	t.Run("closed issue is replaced by a new one", func(t *testing.T) {
		store, gh := testStore(t), &mockGitHub{}
		handleIssues(store, gh, liveRepo, pr, high)

		gh.issueState = "CLOSED"
		if got := handleIssues(store, gh, liveRepo, pr, high); got != "Created #101" {
			t.Fatalf("status = %q, want Created #101", got)
		}
		if len(gh.commented) != 0 {
			t.Errorf("should not comment on closed issue, commented=%v", gh.commented)
		}
		rec, _, _ := store.GetIssue("o/r", 5)
		if rec.IssueNumber != 101 {
			t.Errorf("recorded issue = %d, want 101", rec.IssueNumber)
		}
	})

	t.Run("state check failure falls back to comment", func(t *testing.T) {
		store, gh := testStore(t), &mockGitHub{}
		handleIssues(store, gh, liveRepo, pr, high)

		gh.stateErr = errors.New("network")
		if got := handleIssues(store, gh, liveRepo, pr, high); got != "Commented on #100" {
			t.Errorf("status = %q", got)
		}
	})

	t.Run("created with unknown number records URL and comments by URL", func(t *testing.T) {
		store, gh := testStore(t), &mockGitHub{}
		url := "https://github.com/o/r/issues/weird"
		gh.createURL, gh.createErr = url, errors.New("parse failure")

		if got := handleIssues(store, gh, liveRepo, pr, high); got != "Created "+url {
			t.Fatalf("status = %q", got)
		}
		rec, found, _ := store.GetIssue("o/r", 5)
		if !found || rec.IssueURL != url || rec.IssueNumber != 0 {
			t.Fatalf("record = %+v found=%v", rec, found)
		}

		gh.createErr = nil
		if got := handleIssues(store, gh, liveRepo, pr, high); got != "Commented on "+url {
			t.Errorf("follow-up status = %q", got)
		}
		if len(gh.commented) != 1 || gh.commented[0] != url {
			t.Errorf("commented = %v, want [%s]", gh.commented, url)
		}
	})

	t.Run("dry-run does not call GitHub", func(t *testing.T) {
		store, gh := testStore(t), &mockGitHub{}
		repo := liveRepo
		repo.Mode = config.ModeDryRun
		if got := handleIssues(store, gh, repo, pr, high); got != "Would create issue" {
			t.Errorf("status = %q", got)
		}
		if gh.created != 0 {
			t.Errorf("created=%d, want 0", gh.created)
		}
		if _, found, _ := store.GetIssue("o/r", 5); found {
			t.Error("dry-run should not record an issue")
		}
	})

	t.Run("create failure is not recorded", func(t *testing.T) {
		store, gh := testStore(t), &mockGitHub{createErr: errors.New("label not found")}
		if got := handleIssues(store, gh, liveRepo, pr, high); !strings.HasPrefix(got, "Failed:") {
			t.Errorf("status = %q", got)
		}
		if _, found, _ := store.GetIssue("o/r", 5); found {
			t.Error("failed create should not record an issue")
		}
	})
}
