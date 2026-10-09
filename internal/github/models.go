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
}

// FollowUpCandidate is a PR the user has already reviewed that has picked up
// new commits since that review. Comments do not count — see parseGraphQLResponse.
type FollowUpCandidate struct {
	PullRequest
	LastReviewAt   time.Time // when the user last REVIEWED (comments do not count)
	NewCommitSince string    // OID of the first commit after LastReviewAt
	NewCommitCount int       // how many commits are newer than LastReviewAt
}
