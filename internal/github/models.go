package github

import "time"

// PullRequest represents a GitHub pull request with relevant metadata.
type PullRequest struct {
	Repo      string
	Number    int64
	Title     string
	Author    string
	URL       string
	IsDraft   bool
	CreatedAt time.Time
	Files     int
	Additions int
	Deletions int
	Labels    []string
	// HeadOID is the PR's head commit when it was fetched. Reviews are pinned
	// to it, and follow-up detection compares against it.
	HeadOID string
	// CommitOIDs lists the PR's commits oldest first (up to the last 100),
	// used to count commits added since a reviewed OID.
	CommitOIDs []string
}

// FollowUpCandidate is a PR pr-sentinel has already reviewed whose head has
// moved since that review. Only pr-sentinel's own reviews count — see
// parseGraphQLResponse.
type FollowUpCandidate struct {
	PullRequest
	LastReviewAt   time.Time // when pr-sentinel last reviewed it on GitHub
	LastReviewOID  string    // commit that review was written against; "" if GitHub has none
	NewCommitSince string    // OID of the first commit after LastReviewOID
	NewCommitCount int       // commits after LastReviewOID; 0 when Rewritten
	Rewritten      bool      // LastReviewOID is not in the fetched history (force-push, rebase, or >100 commits since)
}
