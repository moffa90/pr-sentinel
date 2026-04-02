package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Internal GraphQL response types for parsing `gh api graphql` output.

type graphQLResponse struct {
	Data struct {
		Repository struct {
			PullRequests struct {
				Nodes []graphQLPullRequest `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
	} `json:"data"`
}

type graphQLPullRequest struct {
	Number       int64     `json:"number"`
	Title        string    `json:"title"`
	URL          string    `json:"url"`
	IsDraft      bool      `json:"isDraft"`
	CreatedAt    time.Time `json:"createdAt"`
	ChangedFiles int       `json:"changedFiles"`
	Additions    int       `json:"additions"`
	Deletions    int       `json:"deletions"`
	Author       struct {
		Login string `json:"login"`
	} `json:"author"`
	Reviews struct {
		Nodes []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			PublishedAt time.Time `json:"publishedAt"`
		} `json:"nodes"`
	} `json:"reviews"`
	Comments struct {
		Nodes []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			CreatedAt time.Time `json:"createdAt"`
		} `json:"nodes"`
	} `json:"comments"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				OID           string    `json:"oid"`
				CommittedDate time.Time `json:"committedDate"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

// GraphQL query template that fetches open PRs (first 50) with reviews.
const prQuery = `query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    pullRequests(first: 50, states: OPEN) {
      nodes {
        number
        title
        url
        isDraft
        createdAt
        changedFiles
        additions
        deletions
        author { login }
        reviews(first: 50) {
          nodes {
            author { login }
            publishedAt
          }
        }
        comments(first: 50) {
          nodes {
            author { login }
            createdAt
          }
        }
        commits(last: 100) {
          nodes {
            commit {
              oid
              committedDate
            }
          }
        }
      }
    }
  }
}`

// FetchOpenPRs runs `gh api graphql` to fetch open PRs for the given repo.
// Returns two lists: new PRs (never reviewed by githubUser) and follow-up
// candidates (previously reviewed/commented by githubUser with new commits since).
func FetchOpenPRs(repo string, githubUser string) ([]PullRequest, []FollowUpCandidate, error) {
	owner, name := splitRepo(repo)
	if owner == "" || name == "" {
		return nil, nil, fmt.Errorf("invalid repo format %q: expected owner/repo", repo)
	}

	cmd := exec.Command("gh", "api", "graphql",
		"-f", fmt.Sprintf("query=%s", prQuery),
		"-f", fmt.Sprintf("owner=%s", owner),
		"-f", fmt.Sprintf("name=%s", name),
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, nil, fmt.Errorf("gh api graphql failed for %s: %s: %w", repo, errMsg, err)
		}
		return nil, nil, fmt.Errorf("gh api graphql failed for %s: %w", repo, err)
	}

	return parseGraphQLResponse(out, repo, githubUser)
}

// parseGraphQLResponse parses the JSON output from `gh api graphql` and
// categorises PRs into new (never reviewed) and follow-up candidates
// (reviewed/commented by githubUser with new commits since).
func parseGraphQLResponse(data []byte, repo string, githubUser string) ([]PullRequest, []FollowUpCandidate, error) {
	var resp graphQLResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse GraphQL response: %w", err)
	}

	var prs []PullRequest
	var followUps []FollowUpCandidate

	for _, node := range resp.Data.Repository.PullRequests.Nodes {
		if node.IsDraft {
			continue
		}
		if strings.EqualFold(node.Author.Login, githubUser) {
			continue
		}

		pr := PullRequest{
			Repo:      repo,
			Number:    node.Number,
			Title:     node.Title,
			Author:    node.Author.Login,
			URL:       node.URL,
			IsDraft:   node.IsDraft,
			CreatedAt: node.CreatedAt,
			Files:     node.ChangedFiles,
			Additions: node.Additions,
			Deletions: node.Deletions,
		}

		// Find the latest time the user reviewed or commented
		var lastUserActivity time.Time
		for _, review := range node.Reviews.Nodes {
			if strings.EqualFold(review.Author.Login, githubUser) {
				if review.PublishedAt.After(lastUserActivity) {
					lastUserActivity = review.PublishedAt
				}
			}
		}
		for _, comment := range node.Comments.Nodes {
			if strings.EqualFold(comment.Author.Login, githubUser) {
				if comment.CreatedAt.After(lastUserActivity) {
					lastUserActivity = comment.CreatedAt
				}
			}
		}

		if lastUserActivity.IsZero() {
			// User has never reviewed/commented — new PR
			prs = append(prs, pr)
			continue
		}

		// User has reviewed/commented — check for new commits since
		var newCommitSince string
		newCommitCount := 0
		for _, c := range node.Commits.Nodes {
			if c.Commit.CommittedDate.After(lastUserActivity) {
				if newCommitCount == 0 {
					newCommitSince = c.Commit.OID
				}
				newCommitCount++
			}
		}

		if newCommitCount > 0 {
			followUps = append(followUps, FollowUpCandidate{
				PullRequest:    pr,
				LastCommentAt:  lastUserActivity,
				NewCommitSince: newCommitSince,
				NewCommitCount: newCommitCount,
			})
		}
		// No new commits since last comment → skip entirely
	}

	return prs, followUps, nil
}

// GetPRDiff returns the diff for a given PR by running `gh pr diff`.
func GetPRDiff(repo string, number int64) (string, error) {
	cmd := exec.Command("gh", "pr", "diff",
		fmt.Sprintf("%d", number),
		"-R", repo,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("gh pr diff %s#%d failed: %s: %w", repo, number, errMsg, err)
		}
		return "", fmt.Errorf("gh pr diff %s#%d failed: %w", repo, number, err)
	}

	return string(out), nil
}

// PostReview posts a review comment on a PR by running `gh pr review`.
func PostReview(repo string, number int64, body string) error {
	cmd := exec.Command("gh", "pr", "review",
		fmt.Sprintf("%d", number),
		"-R", repo,
		"--comment",
		"--body", body,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("gh pr review %s#%d failed: %s: %w", repo, number, errMsg, err)
		}
		return fmt.Errorf("gh pr review %s#%d failed: %w", repo, number, err)
	}

	return nil
}

// splitRepo splits a "owner/repo" string into its owner and name components.
func splitRepo(repo string) (owner string, name string) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}
