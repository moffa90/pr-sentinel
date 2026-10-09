package github

import (
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
            "reviews": {
              "nodes": [
                { "author": { "login": "bob" }, "publishedAt": "2026-03-15T12:00:00Z" }
              ]
            },
            "comments": { "nodes": [] },
            "commits": {
              "nodes": [
                { "commit": { "oid": "aaa111", "committedDate": "2026-03-15T10:00:00Z" } }
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
            "reviews": {
              "nodes": [
                { "author": { "login": "myuser" }, "publishedAt": "2026-03-17T12:00:00Z" }
              ]
            },
            "comments": { "nodes": [] },
            "commits": {
              "nodes": [
                { "commit": { "oid": "ddd111", "committedDate": "2026-03-17T09:00:00Z" } }
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
            "reviews": { "nodes": [] },
            "comments": {
              "nodes": [
                { "author": { "login": "myuser" }, "createdAt": "2026-03-19T12:00:00Z" }
              ]
            },
            "commits": {
              "nodes": [
                { "commit": { "oid": "eee111", "committedDate": "2026-03-19T09:00:00Z" } }
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
            "reviews": { "nodes": [] },
            "comments": { "nodes": [] },
            "commits": {
              "nodes": [
                { "commit": { "oid": "aaa111", "committedDate": "2026-03-15T10:00:00Z" } }
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
            "reviews": {
              "nodes": [
                { "author": { "login": "myuser" }, "publishedAt": "2026-03-12T10:00:00Z" }
              ]
            },
            "comments": { "nodes": [] },
            "commits": {
              "nodes": [
                { "commit": { "oid": "bbb111", "committedDate": "2026-03-11T10:00:00Z" } },
                { "commit": { "oid": "bbb222", "committedDate": "2026-03-13T10:00:00Z" } },
                { "commit": { "oid": "bbb333", "committedDate": "2026-03-14T10:00:00Z" } }
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
            "reviews": { "nodes": [] },
            "comments": {
              "nodes": [
                { "author": { "login": "myuser" }, "createdAt": "2026-03-15T10:00:00Z" }
              ]
            },
            "commits": {
              "nodes": [
                { "commit": { "oid": "ccc111", "committedDate": "2026-03-11T10:00:00Z" } }
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
            "reviews": { "nodes": [] },
            "comments": { "nodes": [] },
            "commits": {
              "nodes": [
                { "commit": { "oid": "abc123", "committedDate": "2026-03-15T10:00:00Z" } }
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
            "reviews": { "nodes": [] },
            "comments": { "nodes": [] },
            "commits": {
              "nodes": [
                { "commit": { "oid": "fff111", "committedDate": "2026-03-15T10:00:00Z" } }
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
            "reviews": {
              "nodes": [
                { "author": { "login": "myuser" }, "publishedAt": "2026-10-08T16:17:53Z" }
              ]
            },
            "comments": {
              "nodes": [
                { "author": { "login": "myuser" }, "createdAt": "2026-10-08T16:33:24Z" }
              ]
            },
            "commits": {
              "nodes": [
                { "commit": { "oid": "0e5784e", "committedDate": "2026-10-08T16:16:18Z" } },
                { "commit": { "oid": "f7a7d87", "committedDate": "2026-10-08T16:32:56Z" } }
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
            "reviews": {
              "nodes": [
                { "author": { "login": "myuser" }, "publishedAt": "2026-03-20T10:00:00Z" },
                { "author": { "login": "someone-else" }, "publishedAt": "2026-03-21T10:00:00Z" },
                { "author": { "login": "myuser" }, "publishedAt": "2026-03-10T10:00:00Z" }
              ]
            },
            "commits": {
              "nodes": [
                { "commit": { "oid": "ddd111", "committedDate": "2026-03-15T10:00:00Z" } }
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
