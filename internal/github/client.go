package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
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
		RateLimit struct {
			Limit     int `json:"limit"`
			Remaining int `json:"remaining"`
			Cost      int `json:"cost"`
		} `json:"rateLimit"`
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
	Labels       struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
	Author struct {
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
        labels(first: 20) {
          nodes {
            name
          }
        }
        author { login }
        reviews(last: 50) {
          nodes {
            author { login }
            publishedAt
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
  rateLimit {
    limit
    remaining
    cost
  }
}`

// FetchOpenPRs runs `gh api graphql` to fetch open PRs for the given repo.
// Returns two lists: new PRs (never reviewed by githubUser) and follow-up
// candidates (previously reviewed/commented by githubUser with new commits since).
func FetchOpenPRs(repo string, githubUser string, reviewOwnPRs bool) ([]PullRequest, []FollowUpCandidate, error) {
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

	return parseGraphQLResponse(out, repo, githubUser, reviewOwnPRs)
}

// parseGraphQLResponse parses the JSON output from `gh api graphql` and
// categorises PRs into new (never reviewed) and follow-up candidates
// (reviewed/commented by githubUser with new commits since).
func parseGraphQLResponse(data []byte, repo string, githubUser string, reviewOwnPRs bool) ([]PullRequest, []FollowUpCandidate, error) {
	var resp graphQLResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse GraphQL response: %w", err)
	}

	// Log rate limit info
	rl := resp.Data.RateLimit
	if rl.Limit > 0 {
		pct := float64(rl.Remaining) / float64(rl.Limit) * 100
		slog.Debug("GitHub GraphQL rate limit", "remaining", rl.Remaining, "limit", rl.Limit, "cost", rl.Cost, "pct_remaining", fmt.Sprintf("%.0f%%", pct))
		if pct < 20 {
			slog.Warn("GitHub API rate limit low", "remaining", rl.Remaining, "limit", rl.Limit, "pct_remaining", fmt.Sprintf("%.0f%%", pct))
		}
	}

	var prs []PullRequest
	var followUps []FollowUpCandidate

	for _, node := range resp.Data.Repository.PullRequests.Nodes {
		if node.IsDraft {
			continue
		}
		if !reviewOwnPRs && strings.EqualFold(node.Author.Login, githubUser) {
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
		for _, l := range node.Labels.Nodes {
			pr.Labels = append(pr.Labels, l.Name)
		}

		// The watermark answers one question: has this PR been reviewed
		// since its newest commit? Only a review answers it. pr-sentinel
		// always publishes through `gh pr review`, never as a plain comment,
		// so a comment is not evidence that anything was reviewed.
		//
		// Counting comments here is actively harmful on a PR you authored
		// yourself (review_own_prs: true). Push a commit, then comment to
		// explain the push, and the comment moves the watermark past your own
		// commit — the follow-up review is then skipped for good, because no
		// later commit will ever arrive. Cellgain/spark-poc#60 sat that way:
		// review 16:17:53Z, commit 16:32:56Z, comment 16:33:24Z.
		// The query asks for reviews(last: 50): a GraphQL connection returns
		// oldest-first, so `first` would hand back the opening 50 reviews and
		// hide the most recent one on a long-lived PR. That is not a missed
		// follow-up but a loop — a stale watermark makes every commit look
		// new, every cycle queues a review, and each review lands outside the
		// window too, until the daily cap stops it.
		var lastReview time.Time
		for _, review := range node.Reviews.Nodes {
			if strings.EqualFold(review.Author.Login, githubUser) {
				if review.PublishedAt.After(lastReview) {
					lastReview = review.PublishedAt
				}
			}
		}

		if lastReview.IsZero() {
			// Never reviewed — new PR.
			prs = append(prs, pr)
			continue
		}

		// Reviewed before — re-review only if something landed since.
		var newCommitSince string
		newCommitCount := 0
		for _, c := range node.Commits.Nodes {
			if c.Commit.CommittedDate.After(lastReview) {
				if newCommitCount == 0 {
					newCommitSince = c.Commit.OID
				}
				newCommitCount++
			}
		}

		if newCommitCount > 0 {
			followUps = append(followUps, FollowUpCandidate{
				PullRequest:    pr,
				LastReviewAt:   lastReview,
				NewCommitSince: newCommitSince,
				NewCommitCount: newCommitCount,
			})
		}
		// Nothing new since the last review → skip entirely
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

// PostReview posts a review on a PR by running `gh pr review`.
// Verdict maps to: "approve" → --approve, "request-changes" → --request-changes,
// anything else → --comment.
func PostReview(repo string, number int64, body string, verdict string) error {
	flag := "--comment"
	switch verdict {
	case "approve":
		flag = "--approve"
	case "request-changes":
		flag = "--request-changes"
	}

	cmd := exec.Command("gh", "pr", "review",
		fmt.Sprintf("%d", number),
		"-R", repo,
		flag,
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

// GetPRState returns the state of a PR ("OPEN", "CLOSED", or "MERGED").
func GetPRState(repo string, number int64) (string, error) {
	cmd := exec.Command("gh", "pr", "view",
		fmt.Sprintf("%d", number),
		"-R", repo,
		"--json", "state",
		"--jq", ".state",
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("gh pr view %s#%d failed: %s: %w", repo, number, errMsg, err)
		}
		return "", fmt.Errorf("gh pr view %s#%d failed: %w", repo, number, err)
	}

	return strings.TrimSpace(string(out)), nil
}

// splitRepo splits a "owner/repo" string into its owner and name components.
func splitRepo(repo string) (owner string, name string) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// prViewJSON is the subset of `gh pr view --json` fields used by GetPR.
type prViewJSON struct {
	Number       int64  `json:"number"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	IsDraft      bool   `json:"isDraft"`
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
	ChangedFiles int    `json:"changedFiles"`
	Author       struct {
		Login string `json:"login"`
	} `json:"author"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

const prViewFields = "number,title,url,isDraft,additions,deletions,changedFiles,author,labels"

// GetPR fetches a single PR's metadata via `gh pr view`.
func GetPR(repo string, number int64) (PullRequest, error) {
	cmd := exec.Command("gh", "pr", "view",
		fmt.Sprintf("%d", number),
		"-R", repo,
		"--json", prViewFields,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return PullRequest{}, fmt.Errorf("gh pr view %s#%d failed: %s: %w", repo, number, errMsg, err)
		}
		return PullRequest{}, fmt.Errorf("gh pr view %s#%d failed: %w", repo, number, err)
	}

	return parsePRView(out, repo)
}

// parsePRView converts `gh pr view --json` output into a PullRequest.
func parsePRView(data []byte, repo string) (PullRequest, error) {
	var v prViewJSON
	if err := json.Unmarshal(data, &v); err != nil {
		return PullRequest{}, fmt.Errorf("parsing gh pr view output: %w", err)
	}
	pr := PullRequest{
		Repo:      repo,
		Number:    v.Number,
		Title:     v.Title,
		Author:    v.Author.Login,
		URL:       v.URL,
		IsDraft:   v.IsDraft,
		Files:     v.ChangedFiles,
		Additions: v.Additions,
		Deletions: v.Deletions,
	}
	for _, l := range v.Labels {
		pr.Labels = append(pr.Labels, l.Name)
	}
	return pr, nil
}
