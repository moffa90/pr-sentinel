package github

import (
	"testing"
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
	prs, followUps, err := parseGraphQLResponse([]byte(testGraphQLResponse), "owner/repo", "myuser")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only PR #42 should be a new PR (not reviewed/commented by myuser)
	if len(prs) != 1 {
		t.Fatalf("expected 1 new PR, got %d", len(prs))
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

	// PR #44 reviewed by myuser but no new commits → skipped (no follow-up)
	// PR #46 commented by myuser but no new commits → skipped (no follow-up)
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
	prs, followUps, err := parseGraphQLResponse([]byte(testFollowUpResponse), "owner/repo", "myuser")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// PR #50 is new (no user activity)
	if len(prs) != 1 {
		t.Fatalf("expected 1 new PR, got %d", len(prs))
	}
	if prs[0].Number != 50 {
		t.Errorf("expected PR #50, got #%d", prs[0].Number)
	}

	// PR #51 has review by myuser at 2026-03-12 and 2 commits after that
	// PR #52 has comment by myuser at 2026-03-15 but no commits after that → skipped
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
	_, _, err := parseGraphQLResponse([]byte("not json"), "owner/repo", "myuser")
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
	prs, _, err := parseGraphQLResponse([]byte(testRateLimitResponse), "owner/repo", "myuser")
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
	prs, followUps, err := parseGraphQLResponse([]byte(testRateLimitLowResponse), "owner/repo", "myuser")
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
	_, _, err := parseGraphQLResponse([]byte(testNoRateLimitResponse), "owner/repo", "myuser")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
