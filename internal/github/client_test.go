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
                { "author": { "login": "bob" } }
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
            "reviews": { "nodes": [] }
          },
          {
            "number": 44,
            "title": "fix: already reviewed",
            "url": "https://github.com/owner/repo/pull/44",
            "isDraft": false,
            "createdAt": "2026-03-17T10:00:00Z",
            "changedFiles": 3,
            "additions": 50,
            "deletions": 10,
            "author": { "login": "dave" },
            "reviews": {
              "nodes": [
                { "author": { "login": "myuser" } }
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
            "reviews": { "nodes": [] }
          }
        ]
      }
    }
  }
}`

func TestParseGraphQLResponse(t *testing.T) {
	prs, err := parseGraphQLResponse([]byte(testGraphQLResponse), "owner/repo", "myuser")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
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
}

func TestParseGraphQLResponse_InvalidJSON(t *testing.T) {
	_, err := parseGraphQLResponse([]byte("not json"), "owner/repo", "myuser")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
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
