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
}
