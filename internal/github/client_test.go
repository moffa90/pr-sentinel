package github

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

const testGraphQLResponse = `{
  "data": {
    "repository": {
      "pullRequests": {
        "nodes": [
          {
            "number": 42,
            "title": "feat: add new feature",
            "url": "https://github.com/owner/repo/pull/42",
            "isDraft": false,
            "createdAt": "2026-03-15T10:00:00Z",
            "changedFiles": 5,
            "additions": 100,
            "deletions": 20,
            "author": { "login": "alice" },
            "headRefOid": "aaa111",
            "reviews": {
              "nodes": [
                { "author": { "login": "bob" }, "state": "COMMENTED", "submittedAt": "2026-03-15T12:00:00Z", "body": "LGTM", "commit": { "oid": "aaa111" } }
              ]
            },
            "comments": { "nodes": [] },
            "commits": {
              "nodes": [
                { "commit": { "oid": "aaa111" } }
              ]
            }
          },
          {
            "number": 43,
            "title": "wip: draft feature",
            "url": "https://github.com/owner/repo/pull/43",
            "isDraft": true,
            "createdAt": "2026-03-16T10:00:00Z",
            "changedFiles": 2,
            "additions": 30,
            "deletions": 5,
            "author": { "login": "charlie" },
            "reviews": { "nodes": [] },
            "comments": { "nodes": [] },
            "commits": { "nodes": [] }
          },
          {
            "number": 44,
            "title": "fix: already reviewed, no new commits",
            "url": "https://github.com/owner/repo/pull/44",
            "isDraft": false,
            "createdAt": "2026-03-17T10:00:00Z",
            "changedFiles": 3,
            "additions": 50,
            "deletions": 10,
            "author": { "login": "dave" },
            "headRefOid": "ddd111",
            "reviews": {
              "nodes": [
                { "author": { "login": "myuser" }, "state": "COMMENTED", "submittedAt": "2026-03-17T12:00:00Z", "body": "**Verdict: Approved** :white_check_mark:", "commit": { "oid": "ddd111" } }
              ]
            },
            "comments": { "nodes": [] },
            "commits": {
              "nodes": [
                { "commit": { "oid": "ddd111" } }
              ]
            }
          },
          {
            "number": 45,
            "title": "feat: my own pr",
            "url": "https://github.com/owner/repo/pull/45",
            "isDraft": false,
            "createdAt": "2026-03-18T10:00:00Z",
            "changedFiles": 1,
            "additions": 10,
            "deletions": 2,
            "author": { "login": "myuser" },
            "reviews": { "nodes": [] },
            "comments": { "nodes": [] },
            "commits": { "nodes": [] }
          },
          {
            "number": 46,
            "title": "feat: commented by me, no new commits",
            "url": "https://github.com/owner/repo/pull/46",
            "isDraft": false,
            "createdAt": "2026-03-19T10:00:00Z",
            "changedFiles": 2,
            "additions": 15,
            "deletions": 3,
            "author": { "login": "eve" },
            "headRefOid": "eee111",
            "reviews": { "nodes": [] },
            "comments": {
              "nodes": [
                { "author": { "login": "myuser" }, "createdAt": "2026-03-19T12:00:00Z" }
              ]
            },
            "commits": {
              "nodes": [
                { "commit": { "oid": "eee111" } }
              ]
            }
          }
        ]
      }
    }
  }
}`

func TestParseGraphQLResponse(t *testing.T) {
	prs, followUps, err := parseGraphQLResponse([]byte(testGraphQLResponse), "owner/repo", "myuser", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// #42 was never touched by myuser. #46 carries a comment from myuser but
	// no review — and a comment is not a review, so #46 is new work too.
	if len(prs) != 2 {
		t.Fatalf("expected 2 new PRs, got %d", len(prs))
	}

	pr := prs[0]
	if pr.Number != 42 {
		t.Errorf("expected PR #42, got #%d", pr.Number)
	}
	if pr.Title != "feat: add new feature" {
		t.Errorf("unexpected title: %s", pr.Title)
	}
	if pr.Author != "alice" {
		t.Errorf("expected author alice, got %s", pr.Author)
	}
	if pr.Repo != "owner/repo" {
		t.Errorf("expected repo owner/repo, got %s", pr.Repo)
	}
	if pr.URL != "https://github.com/owner/repo/pull/42" {
		t.Errorf("unexpected URL: %s", pr.URL)
	}
	if pr.IsDraft {
		t.Error("expected IsDraft to be false")
	}
	if pr.Files != 5 {
		t.Errorf("expected 5 files, got %d", pr.Files)
	}
	if pr.Additions != 100 {
		t.Errorf("expected 100 additions, got %d", pr.Additions)
	}
	if pr.Deletions != 20 {
		t.Errorf("expected 20 deletions, got %d", pr.Deletions)
	}

	if prs[1].Number != 46 {
		t.Errorf("expected the commented-but-unreviewed PR #46 to be new, got #%d", prs[1].Number)
	}

	// #44 was reviewed by myuser with nothing committed since → no follow-up.
	// #46 is counted above as new, not here.
	if len(followUps) != 0 {
		t.Errorf("expected 0 follow-up candidates, got %d", len(followUps))
	}
}

const testFollowUpResponse = `{
  "data": {
    "repository": {
      "pullRequests": {
        "nodes": [
          {
            "number": 50,
            "title": "feat: brand new PR",
            "url": "https://github.com/owner/repo/pull/50",
            "isDraft": false,
            "createdAt": "2026-03-15T10:00:00Z",
            "changedFiles": 3,
            "additions": 40,
            "deletions": 10,
            "author": { "login": "alice" },
            "headRefOid": "aaa111",
            "reviews": { "nodes": [] },
            "comments": { "nodes": [] },
            "commits": {
              "nodes": [
                { "commit": { "oid": "aaa111" } }
              ]
            }
          },
          {
            "number": 51,
            "title": "fix: PR with my review and new commits",
            "url": "https://github.com/owner/repo/pull/51",
            "isDraft": false,
            "createdAt": "2026-03-10T10:00:00Z",
            "changedFiles": 5,
            "additions": 80,
            "deletions": 20,
            "author": { "login": "bob" },
            "headRefOid": "bbb333",
            "reviews": {
              "nodes": [
                { "author": { "login": "myuser" }, "state": "COMMENTED", "submittedAt": "2026-03-12T10:00:00Z", "body": "**Verdict: Approved** :white_check_mark:", "commit": { "oid": "bbb111" } }
              ]
            },
            "comments": { "nodes": [] },
            "commits": {
              "nodes": [
                { "commit": { "oid": "bbb111" } },
                { "commit": { "oid": "bbb222" } },
                { "commit": { "oid": "bbb333" } }
              ]
            }
          },
          {
            "number": 52,
            "title": "fix: PR with my comment but no new commits",
            "url": "https://github.com/owner/repo/pull/52",
            "isDraft": false,
            "createdAt": "2026-03-10T10:00:00Z",
            "changedFiles": 2,
            "additions": 15,
            "deletions": 5,
            "author": { "login": "charlie" },
            "headRefOid": "ccc111",
            "reviews": { "nodes": [] },
            "comments": {
              "nodes": [
                { "author": { "login": "myuser" }, "createdAt": "2026-03-15T10:00:00Z" }
              ]
            },
            "commits": {
              "nodes": [
                { "commit": { "oid": "ccc111" } }
              ]
            }
          }
        ]
      }
    }
  }
}`

func TestParseGraphQLResponse_FollowUp(t *testing.T) {
	prs, followUps, err := parseGraphQLResponse([]byte(testFollowUpResponse), "owner/repo", "myuser", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// #50 has no activity at all; #52 has a comment but no review. Neither has
	// been reviewed, so both are new.
	if len(prs) != 2 {
		t.Fatalf("expected 2 new PRs, got %d", len(prs))
	}
	if prs[0].Number != 50 {
		t.Errorf("expected PR #50, got #%d", prs[0].Number)
	}
	if prs[1].Number != 52 {
		t.Errorf("expected PR #52, got #%d", prs[1].Number)
	}

	// #51 was reviewed by myuser at 2026-03-12 with 2 commits after it.
	if len(followUps) != 1 {
		t.Fatalf("expected 1 follow-up candidate, got %d", len(followUps))
	}
	if followUps[0].Number != 51 {
		t.Errorf("expected follow-up PR #51, got #%d", followUps[0].Number)
	}
	if followUps[0].NewCommitCount != 2 {
		t.Errorf("expected 2 new commits, got %d", followUps[0].NewCommitCount)
	}
	if followUps[0].NewCommitSince != "bbb222" {
		t.Errorf("expected NewCommitSince=bbb222, got %s", followUps[0].NewCommitSince)
	}
}

func TestParseGraphQLResponse_InvalidJSON(t *testing.T) {
	_, _, err := parseGraphQLResponse([]byte("not json"), "owner/repo", "myuser", false)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

const testRateLimitResponse = `{
  "data": {
    "repository": {
      "pullRequests": {
        "nodes": [
          {
            "number": 1,
            "title": "test PR",
            "url": "https://github.com/owner/repo/pull/1",
            "isDraft": false,
            "createdAt": "2026-03-15T10:00:00Z",
            "changedFiles": 1,
            "additions": 5,
            "deletions": 2,
            "author": { "login": "alice" },
            "headRefOid": "abc123",
            "reviews": { "nodes": [] },
            "comments": { "nodes": [] },
            "commits": {
              "nodes": [
                { "commit": { "oid": "abc123" } }
              ]
            }
          }
        ]
      }
    },
    "rateLimit": {
      "limit": 5000,
      "remaining": 4990,
      "cost": 1
    }
  }
}`

func TestParseGraphQLResponse_WithRateLimit(t *testing.T) {
	prs, _, err := parseGraphQLResponse([]byte(testRateLimitResponse), "owner/repo", "myuser", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}
}

const testRateLimitLowResponse = `{
  "data": {
    "repository": {
      "pullRequests": { "nodes": [] }
    },
    "rateLimit": {
      "limit": 5000,
      "remaining": 500,
      "cost": 1
    }
  }
}`

func TestParseGraphQLResponse_LowRateLimit(t *testing.T) {
	// Should not error — just logs a warning internally
	prs, followUps, err := parseGraphQLResponse([]byte(testRateLimitLowResponse), "owner/repo", "myuser", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prs) != 0 {
		t.Errorf("expected 0 PRs, got %d", len(prs))
	}
	if len(followUps) != 0 {
		t.Errorf("expected 0 follow-ups, got %d", len(followUps))
	}
}

const testNoRateLimitResponse = `{
  "data": {
    "repository": {
      "pullRequests": { "nodes": [] }
    }
  }
}`

func TestParseGraphQLResponse_MissingRateLimit(t *testing.T) {
	// Fine-grained PATs may return limit=0 or omit rateLimit entirely
	_, _, err := parseGraphQLResponse([]byte(testNoRateLimitResponse), "owner/repo", "myuser", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

const testLabelsResponse = `{
  "data": {
    "repository": {
      "pullRequests": {
        "nodes": [
          {
            "number": 60,
            "title": "feat: labeled PR",
            "url": "https://github.com/owner/repo/pull/60",
            "isDraft": false,
            "createdAt": "2026-03-15T10:00:00Z",
            "changedFiles": 1,
            "additions": 5,
            "deletions": 2,
            "labels": {
              "nodes": [
                {"name": "auto-merge"},
                {"name": "enhancement"}
              ]
            },
            "author": { "login": "alice" },
            "headRefOid": "fff111",
            "reviews": { "nodes": [] },
            "comments": { "nodes": [] },
            "commits": {
              "nodes": [
                { "commit": { "oid": "fff111" } }
              ]
            }
          }
        ]
      }
    },
    "rateLimit": { "limit": 5000, "remaining": 4990, "cost": 1 }
  }
}`

func TestParseGraphQLResponse_Labels(t *testing.T) {
	prs, _, err := parseGraphQLResponse([]byte(testLabelsResponse), "owner/repo", "myuser", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}

	if len(prs[0].Labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(prs[0].Labels))
	}
	if prs[0].Labels[0] != "auto-merge" {
		t.Errorf("expected first label 'auto-merge', got %q", prs[0].Labels[0])
	}
	if prs[0].Labels[1] != "enhancement" {
		t.Errorf("expected second label 'enhancement', got %q", prs[0].Labels[1])
	}
}

func TestParseGraphQLResponse_ReviewOwnPRs(t *testing.T) {
	// With reviewOwnPRs=false, PR #45 (authored by myuser) is filtered out
	prs, _, err := parseGraphQLResponse([]byte(testGraphQLResponse), "owner/repo", "myuser", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, pr := range prs {
		if pr.Number == 45 {
			t.Error("PR #45 (own PR) should be filtered out when reviewOwnPRs=false")
		}
	}

	// With reviewOwnPRs=true, PR #45 should be included
	prs2, _, err := parseGraphQLResponse([]byte(testGraphQLResponse), "owner/repo", "myuser", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, pr := range prs2 {
		if pr.Number == 45 {
			found = true
			break
		}
	}
	if !found {
		t.Error("PR #45 (own PR) should be included when reviewOwnPRs=true")
	}
}

func TestSplitRepo(t *testing.T) {
	tests := []struct {
		input     string
		wantOwner string
		wantName  string
	}{
		{"owner/repo", "owner", "repo"},
		{"my-org/my-repo", "my-org", "my-repo"},
		{"invalid", "", ""},
		{"", "", ""},
		{"a/b/c", "a", "b/c"},
	}

	for _, tt := range tests {
		owner, name := splitRepo(tt.input)
		if owner != tt.wantOwner || name != tt.wantName {
			t.Errorf("splitRepo(%q) = (%q, %q), want (%q, %q)",
				tt.input, owner, name, tt.wantOwner, tt.wantName)
		}
	}
}

func TestParsePRView(t *testing.T) {
	data := []byte(`{"number":42,"title":"Fix bug","url":"https://github.com/o/r/pull/42","isDraft":false,"additions":10,"deletions":3,"changedFiles":2,"author":{"login":"alice"},"labels":[{"name":"automerge"},{"name":"bug"}]}`)
	pr, err := parsePRView(data, "o/r")
	if err != nil {
		t.Fatalf("parsePRView: %v", err)
	}
	if pr.Repo != "o/r" || pr.Number != 42 || pr.Title != "Fix bug" || pr.Author != "alice" ||
		pr.URL != "https://github.com/o/r/pull/42" || pr.Files != 2 || pr.Additions != 10 || pr.Deletions != 3 {
		t.Errorf("unexpected PR: %+v", pr)
	}
	if len(pr.Labels) != 2 || pr.Labels[0] != "automerge" || pr.Labels[1] != "bug" {
		t.Errorf("Labels = %v", pr.Labels)
	}

	if _, err := parsePRView([]byte("not json"), "o/r"); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// testCommentAfterPushResponse reproduces Cellgain/spark-poc#60: a review,
// then a commit, then a comment 28 seconds after the commit. While comments
// counted toward the watermark, the comment landed after the commit and the
// follow-up was skipped — permanently, since no later commit would ever come.
const testCommentAfterPushResponse = `{
  "data": {
    "repository": {
      "pullRequests": {
        "nodes": [
          {
            "number": 60,
            "title": "feat: name what changed, not just that something did",
            "url": "https://github.com/owner/repo/pull/60",
            "isDraft": false,
            "createdAt": "2026-10-08T15:00:00Z",
            "changedFiles": 12,
            "additions": 900,
            "deletions": 40,
            "author": { "login": "myuser" },
            "headRefOid": "f7a7d87",
            "reviews": {
              "nodes": [
                { "author": { "login": "myuser" }, "state": "COMMENTED", "submittedAt": "2026-10-08T16:17:53Z", "body": "**Verdict: Approved** :white_check_mark:", "commit": { "oid": "0e5784e" } }
              ]
            },
            "comments": {
              "nodes": [
                { "author": { "login": "myuser" }, "createdAt": "2026-10-08T16:33:24Z" }
              ]
            },
            "commits": {
              "nodes": [
                { "commit": { "oid": "0e5784e" } },
                { "commit": { "oid": "f7a7d87" } }
              ]
            }
          }
        ]
      }
    }
  }
}`

func TestParseGraphQLResponse_CommentAfterPushStillFollowsUp(t *testing.T) {
	prs, followUps, err := parseGraphQLResponse([]byte(testCommentAfterPushResponse), "owner/repo", "myuser", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(prs) != 0 {
		t.Fatalf("expected no new PRs (it has been reviewed), got %d", len(prs))
	}
	if len(followUps) != 1 {
		t.Fatalf("expected 1 follow-up candidate, got %d — a comment after the push must not hide it", len(followUps))
	}

	got := followUps[0]
	if got.Number != 60 {
		t.Errorf("expected PR #60, got #%d", got.Number)
	}
	if got.NewCommitCount != 1 {
		t.Errorf("NewCommitCount = %d, want 1", got.NewCommitCount)
	}
	if got.NewCommitSince != "f7a7d87" {
		t.Errorf("NewCommitSince = %q, want f7a7d87", got.NewCommitSince)
	}

	// The watermark must be the review, not the later comment. If this reads
	// 16:33:24 the comment is being counted again and the bug is back.
	wantReview := "2026-10-08T16:17:53Z"
	if got.LastReviewAt.UTC().Format(time.RFC3339) != wantReview {
		t.Errorf("LastReviewAt = %s, want %s (the review, not the comment)",
			got.LastReviewAt.UTC().Format(time.RFC3339), wantReview)
	}
}

// The watermark is computed from the reviews connection and nothing else, so
// the query has to ask for the NEWEST page. A GraphQL connection returns
// oldest-first, so `reviews(first: 50)` quietly strands the watermark at an
// ancient timestamp once a PR passes 50 reviews — and every commit then looks
// new, so each cycle queues a follow-up whose own review also lands outside
// the window. That is a billing loop, not a missed review.
//
// This guard belongs on the query, not on parseGraphQLResponse: the parser
// only ever sees the nodes it is handed and cannot tell which page they came
// from, so a parse-level test passes with either spelling.
func TestPRQueryAsksForTheNewestReviews(t *testing.T) {
	if strings.Contains(prQuery, "reviews(first:") {
		t.Error("prQuery asks for reviews(first: N) — that is the oldest page; use last:")
	}
	if !strings.Contains(prQuery, "reviews(last:") {
		t.Error("prQuery must fetch reviews(last: N) so the newest review is present")
	}
}

// testUnorderedReviewsResponse puts the user's NEWEST review first and an older
// one last, so an implementation that trusted node order instead of comparing
// timestamps would read the wrong watermark.
const testUnorderedReviewsResponse = `{
  "data": {
    "repository": {
      "pullRequests": {
        "nodes": [
          {
            "number": 70,
            "title": "fix: something long-running",
            "url": "https://github.com/owner/repo/pull/70",
            "isDraft": false,
            "createdAt": "2026-03-01T10:00:00Z",
            "changedFiles": 1,
            "additions": 1,
            "deletions": 1,
            "author": { "login": "alice" },
            "headRefOid": "ddd111",
            "reviews": {
              "nodes": [
                { "author": { "login": "myuser" }, "state": "COMMENTED", "submittedAt": "2026-03-20T10:00:00Z", "body": "**Verdict: Approved** :white_check_mark:", "commit": { "oid": "ddd111" } },
                { "author": { "login": "someone-else" }, "state": "COMMENTED", "submittedAt": "2026-03-21T10:00:00Z", "body": "**Verdict: Approved** :white_check_mark:", "commit": { "oid": "zzz999" } },
                { "author": { "login": "myuser" }, "state": "COMMENTED", "submittedAt": "2026-03-10T10:00:00Z", "body": "**Verdict: Approved** :white_check_mark:", "commit": { "oid": "ccc000" } }
              ]
            },
            "commits": {
              "nodes": [
                { "commit": { "oid": "ddd111" } }
              ]
            }
          }
        ]
      }
    }
  }
}`

func TestParseGraphQLResponse_NewestReviewWinsWhateverTheOrder(t *testing.T) {
	prs, followUps, err := parseGraphQLResponse([]byte(testUnorderedReviewsResponse), "owner/repo", "myuser", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prs) != 0 {
		t.Fatalf("expected no new PRs, got %d", len(prs))
	}

	// myuser's newest review is 03-20; the only commit is 03-15, so there is
	// nothing to follow up. Reading 03-10 instead would wrongly queue one, and
	// counting someone-else's 03-21 review would be wrong in the other direction.
	if len(followUps) != 0 {
		t.Fatalf("expected no follow-up (newest own review postdates the commit), got %d", len(followUps))
	}
}

// oidFixture builds a one-PR GraphQL response. reviews are raw review nodes.
func oidFixture(head string, commits []string, reviews ...string) []byte {
	var c []string
	for _, oid := range commits {
		c = append(c, fmt.Sprintf(`{ "commit": { "oid": %q } }`, oid))
	}
	return []byte(fmt.Sprintf(`{"data":{"repository":{"pullRequests":{"nodes":[{
		"number": 90, "title": "t", "url": "u", "isDraft": false, "createdAt": "2026-10-01T00:00:00Z",
		"changedFiles": 1, "additions": 1, "deletions": 1,
		"author": { "login": "alice" }, "headRefOid": %q,
		"reviews": { "nodes": [%s] },
		"commits": { "nodes": [%s] }
	}]}}}}`, head, strings.Join(reviews, ","), strings.Join(c, ",")))
}

// sentinelReviewNode is a review pr-sentinel posted (it carries ReviewMarker).
func sentinelReviewNode(at, oid string) string {
	return fmt.Sprintf(`{ "author": { "login": "myuser" }, "state": "COMMENTED", "submittedAt": %q, "body": "**Verdict: Comment** :speech_balloon:", "commit": { "oid": %q } }`, at, oid)
}

func TestParseGraphQLResponse_OIDFollowUps(t *testing.T) {
	tests := []struct {
		name          string
		data          []byte
		wantNew       int
		wantFollowUps int
		wantCount     int
		wantSince     string
		wantRewritten bool
		wantLastOID   string
	}{
		{
			// Issue #5, as it happened on pr-sentinel#7: b6b3d0c was committed
			// before the review that preceded it but pushed after. Commit dates
			// say nothing is new; the head says otherwise.
			name:          "commit authored before the review but pushed after it",
			data:          oidFixture("b6b3d0c", []string{"78f5abc", "87ca90f", "b6b3d0c"}, sentinelReviewNode("2026-10-09T15:32:45Z", "87ca90f")),
			wantFollowUps: 1, wantCount: 1, wantSince: "b6b3d0c", wantLastOID: "87ca90f",
		},
		{
			name: "head is the reviewed commit",
			data: oidFixture("aaa", []string{"aaa"}, sentinelReviewNode("2026-10-01T00:00:00Z", "aaa")),
		},
		{
			name:          "force-push: reviewed commit gone from history",
			data:          oidFixture("new2", []string{"new1", "new2"}, sentinelReviewNode("2026-10-01T00:00:00Z", "gone")),
			wantFollowUps: 1, wantRewritten: true, wantLastOID: "gone",
		},
		{
			name:          "review without a commit is left to the poller",
			data:          oidFixture("aaa", []string{"aaa"}, `{ "author": { "login": "myuser" }, "state": "COMMENTED", "submittedAt": "2026-10-01T00:00:00Z", "body": "**Verdict: Approved**", "commit": null }`),
			wantFollowUps: 1, wantLastOID: "",
		},
		{
			// prism#494: the same account's thread reply is a COMMENTED review
			// stamped with the head at reply time. It must not hide the push.
			name: "same-account reply at head does not hide a push",
			data: oidFixture("bbb", []string{"aaa", "bbb"},
				sentinelReviewNode("2026-10-01T00:00:00Z", "aaa"),
				`{ "author": { "login": "myuser" }, "state": "COMMENTED", "submittedAt": "2026-10-02T00:00:00Z", "body": "", "commit": { "oid": "bbb" } }`),
			wantFollowUps: 1, wantCount: 1, wantSince: "bbb", wantLastOID: "aaa",
		},
		{
			name: "manual review only: not reviewed by pr-sentinel",
			data: oidFixture("aaa", []string{"aaa"},
				`{ "author": { "login": "myuser" }, "state": "APPROVED", "submittedAt": "2026-10-02T00:00:00Z", "body": "looks fine", "commit": { "oid": "aaa" } }`),
			wantNew: 1,
		},
		{
			name: "pending review ignored",
			data: oidFixture("bbb", []string{"aaa", "bbb"},
				sentinelReviewNode("2026-10-01T00:00:00Z", "aaa"),
				`{ "author": { "login": "myuser" }, "state": "PENDING", "submittedAt": null, "body": "**Verdict: Approved**", "commit": { "oid": "bbb" } }`),
			wantFollowUps: 1, wantCount: 1, wantSince: "bbb", wantLastOID: "aaa",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prs, fus, err := parseGraphQLResponse(tt.data, "o/r", "myuser", false)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(prs) != tt.wantNew || len(fus) != tt.wantFollowUps {
				t.Fatalf("new=%d followUps=%d, want %d/%d", len(prs), len(fus), tt.wantNew, tt.wantFollowUps)
			}
			if tt.wantFollowUps == 0 {
				return
			}
			f := fus[0]
			if f.NewCommitCount != tt.wantCount || f.NewCommitSince != tt.wantSince || f.Rewritten != tt.wantRewritten || f.LastReviewOID != tt.wantLastOID {
				t.Errorf("got count=%d since=%q rewritten=%v last=%q, want %d %q %v %q",
					f.NewCommitCount, f.NewCommitSince, f.Rewritten, f.LastReviewOID,
					tt.wantCount, tt.wantSince, tt.wantRewritten, tt.wantLastOID)
			}
			if f.HeadOID == "" || len(f.CommitOIDs) == 0 {
				t.Errorf("head/commits not populated: %+v", f.PullRequest)
			}
		})
	}
}

func TestPRQueryAsksForCommitsNotClocks(t *testing.T) {
	for _, want := range []string{"headRefOid", "commit { oid }", "author: $author", "submittedAt"} {
		if !strings.Contains(prQuery, want) {
			t.Errorf("prQuery missing %q", want)
		}
	}
	for _, unwanted := range []string{"committedDate", "PENDING"} {
		if strings.Contains(prQuery, unwanted) {
			t.Errorf("prQuery should not contain %q", unwanted)
		}
	}
}

func TestCommitsSince(t *testing.T) {
	commits := []string{"a", "b", "c"}
	tests := []struct {
		reviewed      string
		wantSince     string
		wantCount     int
		wantRewritten bool
	}{
		{"a", "b", 2, false},
		{"b", "c", 1, false},
		{"c", "", 0, false},
		{"x", "", 0, true},
	}
	for _, tt := range tests {
		since, count, rewritten := CommitsSince(commits, tt.reviewed)
		if since != tt.wantSince || count != tt.wantCount || rewritten != tt.wantRewritten {
			t.Errorf("CommitsSince(%q) = %q,%d,%v want %q,%d,%v", tt.reviewed, since, count, rewritten, tt.wantSince, tt.wantCount, tt.wantRewritten)
		}
	}
}

func TestBuildReviewRequest(t *testing.T) {
	tests := []struct{ verdict, want string }{
		{"approve", "APPROVE"},
		{"request-changes", "REQUEST_CHANGES"},
		{"comment", "COMMENT"},
		{"", "COMMENT"},
	}
	for _, tt := range tests {
		r := buildReviewRequest("body", tt.verdict, "abc")
		if r.Event != tt.want || r.CommitID != "abc" || r.Body != "body" {
			t.Errorf("verdict %q → %+v, want event %s", tt.verdict, r, tt.want)
		}
	}
}

func TestParsePRView_HeadAndCommits(t *testing.T) {
	pr, err := parsePRView([]byte(`{"number":1,"headRefOid":"ccc","commits":[{"oid":"aaa"},{"oid":"ccc"}],"author":{"login":"a"}}`), "o/r")
	if err != nil {
		t.Fatalf("parsePRView: %v", err)
	}
	if pr.HeadOID != "ccc" || strings.Join(pr.CommitOIDs, ",") != "aaa,ccc" {
		t.Errorf("head=%q commits=%v", pr.HeadOID, pr.CommitOIDs)
	}
}

func TestIsCommitNotInPR(t *testing.T) {
	real := `gh: Unprocessable Entity (HTTP 422) {"message":"Unprocessable Entity","errors":["The commitOID is not part of the pull request"],"status":"422"}`
	if !isCommitNotInPR(real) {
		t.Error("GitHub's 422 for a stale commit not recognised")
	}
	if isCommitNotInPR(`gh: Unprocessable Entity (HTTP 422) {"errors":["Can not approve your own pull request"]}`) {
		t.Error("a different 422 must not be treated as a stale commit")
	}
}

func TestIsSentinelReview(t *testing.T) {
	tests := []struct {
		body string
		want bool
	}{
		{"> AI-assisted review\n\n**Verdict: Approved** :white_check_mark:\n\nok", true},
		{"**Verdict: Changes Requested** :x:", true},
		{"**Verdict: Comment** :speech_balloon:", true},
		{"Agreeing with the bot here:\n> **Verdict: Approved**", false},
		{"see **Verdict: Approved** above", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsSentinelReview(tt.body); got != tt.want {
			t.Errorf("IsSentinelReview(%q) = %v, want %v", tt.body, got, tt.want)
		}
	}
}

func TestParsePatchID(t *testing.T) {
	tests := []struct{ out, want string }{
		{"8d40e6b0db060aef85b13fb00a1ca3ff6d0c8508 0000000000000000000000000000000000000000\n", "8d40e6b0db060aef85b13fb00a1ca3ff6d0c8508"},
		{"", ""},
		{"\n", ""},
	}
	for _, tt := range tests {
		if got := parsePatchID(tt.out); got != tt.want {
			t.Errorf("parsePatchID(%q) = %q, want %q", tt.out, got, tt.want)
		}
	}
}

// Runs the real git: whitespace must count (an indentation change moves code
// into or out of a block in Python), line numbers must not (a rebase that
// shifts the hunk is the same change).
func TestPatchIDOfDiff(t *testing.T) {
	diff := func(start int, added string) []byte {
		return []byte(fmt.Sprintf("diff --git a/p.py b/p.py\n--- a/p.py\n+++ b/p.py\n@@ -%d,2 +%d,3 @@\n if a:\n     b()\n+%s\n", start, start, added))
	}
	dedented, err := PatchIDOfDiff(diff(1, "c()"))
	if err != nil {
		t.Skipf("git patch-id --verbatim unavailable: %v", err)
	}
	indented, err := PatchIDOfDiff(diff(1, "    c()"))
	if err != nil {
		t.Fatalf("indented: %v", err)
	}
	moved, err := PatchIDOfDiff(diff(40, "c()"))
	if err != nil {
		t.Fatalf("moved: %v", err)
	}
	empty, err := PatchIDOfDiff(nil)

	if dedented == "" || dedented == indented {
		t.Errorf("indentation-only change must change the fingerprint: %q vs %q", dedented, indented)
	}
	if moved != dedented {
		t.Errorf("same change at another line must keep the fingerprint: %q vs %q", moved, dedented)
	}
	if err != nil || empty != "" {
		t.Errorf("empty diff: %q, %v", empty, err)
	}
}

func TestEscapeRef(t *testing.T) {
	tests := []struct{ ref, want string }{
		{"main", "main"},
		{"feature/x", "feature/x"},
		{"fix#12", "fix%2312"},
		{"50%?", "50%25%3F"},
	}
	for _, tt := range tests {
		if got := escapeRef(tt.ref); got != tt.want {
			t.Errorf("escapeRef(%q) = %q, want %q", tt.ref, got, tt.want)
		}
	}
}

func TestBaseRefIsFetched(t *testing.T) {
	if !strings.Contains(prQuery, "baseRefName") || !strings.Contains(prViewFields, "baseRefName") {
		t.Error("baseRefName must be fetched by both the poll query and GetPR")
	}
	pr, err := parsePRView([]byte(`{"number":1,"baseRefName":"main","headRefOid":"h"}`), "o/r")
	if err != nil || pr.BaseRef != "main" {
		t.Errorf("BaseRef = %q, err = %v", pr.BaseRef, err)
	}
}
