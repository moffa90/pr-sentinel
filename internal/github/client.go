package github

import (
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
		} `json:"nodes"`
	} `json:"reviews"`
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
          }
        }
      }
    }
  }
}`

// FetchOpenPRs runs `gh api graphql` to fetch open PRs for the given repo,
// then filters out drafts, PRs authored by githubUser, and PRs already
// reviewed by githubUser.
func FetchOpenPRs(repo string, githubUser string) ([]PullRequest, error) {
	owner, name := splitRepo(repo)
	if owner == "" || name == "" {
		return nil, fmt.Errorf("invalid repo format %q: expected owner/repo", repo)
	}

	cmd := exec.Command("gh", "api", "graphql",
		"-f", fmt.Sprintf("query=%s", prQuery),
		"-f", fmt.Sprintf("owner=%s", owner),
		"-f", fmt.Sprintf("name=%s", name),
	)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh api graphql failed: %w", err)
	}

	return parseGraphQLResponse(out, repo, githubUser)
}

// parseGraphQLResponse parses the JSON output from `gh api graphql` and
// filters PRs. Exported for testing.
func parseGraphQLResponse(data []byte, repo string, githubUser string) ([]PullRequest, error) {
	var resp graphQLResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL response: %w", err)
	}

	var prs []PullRequest
	for _, node := range resp.Data.Repository.PullRequests.Nodes {
		// Filter out drafts
		if node.IsDraft {
			continue
		}

		// Filter out PRs authored by githubUser
		if strings.EqualFold(node.Author.Login, githubUser) {
			continue
		}

		// Filter out PRs already reviewed by githubUser
		alreadyReviewed := false
		for _, review := range node.Reviews.Nodes {
			if strings.EqualFold(review.Author.Login, githubUser) {
				alreadyReviewed = true
				break
			}
		}
		if alreadyReviewed {
			continue
		}

		prs = append(prs, PullRequest{
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
		})
	}

	return prs, nil
}

// GetPRDiff returns the diff for a given PR by running `gh pr diff`.
func GetPRDiff(repo string, number int64) (string, error) {
	cmd := exec.Command("gh", "pr", "diff",
		fmt.Sprintf("%d", number),
		"-R", repo,
	)

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh pr diff failed: %w", err)
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

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh pr review failed: %w", err)
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
