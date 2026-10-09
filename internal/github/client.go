package github

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
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
	HeadRefOID string `json:"headRefOid"`
	Reviews    struct {
		Nodes []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			State       string    `json:"state"`
			SubmittedAt time.Time `json:"submittedAt"`
			Body        string    `json:"body"`
			Commit      *struct {
				OID string `json:"oid"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"reviews"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				OID string `json:"oid"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

// ReviewMarker starts the verdict line of every review pr-sentinel formats
// (reviewer.StructuredReview.FormatMarkdown). It tells pr-sentinel's own
// reviews apart from the same account's manual reviews and thread replies,
// which GitHub stamps with whatever the head is when they are posted.
const ReviewMarker = "**Verdict: "

// sentinelVerdictRe matches the full verdict line at the start of a line, so a
// manual review that merely quotes an earlier pr-sentinel review is not
// mistaken for one.
var sentinelVerdictRe = regexp.MustCompile(`(?m)^\*\*Verdict: (Approved|Changes Requested|Comment)\*\*`)

// IsSentinelReview reports whether a review body was written by pr-sentinel.
func IsSentinelReview(body string) bool {
	return sentinelVerdictRe.MatchString(body)
}

// GraphQL query template that fetches open PRs (first 50) with reviews.
const prQuery = `query($owner: String!, $name: String!, $author: String) {
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
        headRefOid
        reviews(last: 20, author: $author, states: [APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED]) {
          nodes {
            author { login }
            state
            submittedAt
            body
            commit { oid }
          }
        }
        commits(last: 100) {
          nodes {
            commit {
              oid
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
// Returns two lists: PRs pr-sentinel has not reviewed on GitHub, and follow-up
// candidates (reviewed by pr-sentinel, head moved since that review). The
// poller makes the final call against its own state.
func FetchOpenPRs(repo string, githubUser string, reviewOwnPRs bool) ([]PullRequest, []FollowUpCandidate, error) {
	owner, name := splitRepo(repo)
	if owner == "" || name == "" {
		return nil, nil, fmt.Errorf("invalid repo format %q: expected owner/repo", repo)
	}

	args := []string{"api", "graphql",
		"-f", fmt.Sprintf("query=%s", prQuery),
		"-f", fmt.Sprintf("owner=%s", owner),
		"-f", fmt.Sprintf("name=%s", name),
	}
	// Filtering by author server-side keeps other reviewers from pushing the
	// user's reviews out of the last-50 window. Omitted (null) when unset.
	if githubUser != "" {
		args = append(args, "-f", fmt.Sprintf("author=%s", githubUser))
	}
	cmd := exec.Command("gh", args...)

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

		pr.HeadOID = node.HeadRefOID
		for _, c := range node.Commits.Nodes {
			pr.CommitOIDs = append(pr.CommitOIDs, c.Commit.OID)
		}

		last, ok := lastSentinelReview(node, githubUser)
		if !ok {
			// pr-sentinel has never reviewed it on GitHub. The poller still
			// checks its own state, which covers dry-run and promoted repos.
			prs = append(prs, pr)
			continue
		}

		// Re-review only if the head moved off the commit that review was
		// written against. No clock is compared anywhere: a commit authored
		// before the review but pushed after it (a date-preserving rebase, or a
		// commit made locally long before the push) still moves the head.
		// An unknown OID never counts as "different"; the poller decides from
		// its own state instead.
		if last.oid != "" && last.oid == pr.HeadOID {
			continue
		}

		candidate := FollowUpCandidate{
			PullRequest:   pr,
			LastReviewAt:  last.at,
			LastReviewOID: last.oid,
		}
		if last.oid != "" {
			candidate.NewCommitSince, candidate.NewCommitCount, candidate.Rewritten = commitsSince(pr.CommitOIDs, last.oid)
		}
		followUps = append(followUps, candidate)
	}

	return prs, followUps, nil
}

// sentinelReview is the newest review pr-sentinel posted on a PR.
type sentinelReview struct {
	at  time.Time
	oid string // "" if GitHub returned no commit for it
}

// lastSentinelReview finds pr-sentinel's newest published review on the PR.
//
// Only reviews carrying ReviewMarker count. The same account's manual reviews
// and thread replies are COMMENTED reviews too, stamped with whatever the head
// is when they are posted, so counting them would hide a push the same way
// counting comments did. PENDING reviews are excluded by the query and by the
// zero submittedAt check.
//
// The query asks for reviews(last: 20): a GraphQL connection returns oldest
// first, so `first` would return the opening page and miss the newest review
// on a long-lived PR. The author filter keeps other reviewers out of the
// window and bodies are full reviews, so 20 is plenty; if a burst of the
// user's own replies pushes pr-sentinel's review out, the poller falls back to
// the stored head_oid. The newest is picked by time, not by node order.
func lastSentinelReview(node graphQLPullRequest, githubUser string) (sentinelReview, bool) {
	var last sentinelReview
	found := false
	for _, r := range node.Reviews.Nodes {
		if !strings.EqualFold(r.Author.Login, githubUser) || r.SubmittedAt.IsZero() || r.State == "PENDING" {
			continue
		}
		if !IsSentinelReview(r.Body) {
			continue
		}
		if !found || r.SubmittedAt.After(last.at) {
			last = sentinelReview{at: r.SubmittedAt}
			if r.Commit != nil {
				last.oid = r.Commit.OID
			}
			found = true
		}
	}
	return last, found
}

// CommitsSince locates reviewedOID in the PR's commits (oldest first) and
// returns the first commit after it and how many follow it. If reviewedOID is
// not in the list, the branch was rewritten since (force-push or rebase), or
// it is older than the last 100 commits: rewritten is true and the count is 0.
func CommitsSince(commits []string, reviewedOID string) (since string, count int, rewritten bool) {
	return commitsSince(commits, reviewedOID)
}

func commitsSince(commits []string, reviewedOID string) (since string, count int, rewritten bool) {
	for i := len(commits) - 1; i >= 0; i-- {
		if commits[i] == reviewedOID {
			if i+1 < len(commits) {
				since = commits[i+1]
			}
			return since, len(commits) - 1 - i, false
		}
	}
	return "", 0, true
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

// reviewEvents maps a verdict to the REST review event. Anything else
// (including "comment") is a plain comment review, as in PostReview.
var reviewEvents = map[string]string{
	"approve":         "APPROVE",
	"request-changes": "REQUEST_CHANGES",
}

// reviewRequest is the body of POST /repos/{owner}/{repo}/pulls/{n}/reviews.
type reviewRequest struct {
	CommitID string `json:"commit_id"`
	Body     string `json:"body"`
	Event    string `json:"event"`
}

// buildReviewRequest builds the REST review payload for a verdict.
func buildReviewRequest(body, verdict, commitOID string) reviewRequest {
	event, ok := reviewEvents[verdict]
	if !ok {
		event = "COMMENT"
	}
	return reviewRequest{CommitID: commitOID, Body: body, Event: event}
}

// ErrCommitNotInPR means GitHub refused a pinned review because the commit is
// no longer part of the PR (it was force-pushed away while the review ran).
// Retrying cannot help; the next cycle reviews the new head.
var ErrCommitNotInPR = errors.New("commit is no longer part of the pull request")

// PostReviewAtCommit posts a review pinned to commitOID through the REST API.
//
// `gh pr review` has no commit option, so GitHub stamps its review with the
// head at the moment it is posted. If a push lands while the review is
// running, that stamp names a commit nobody reviewed, and the follow-up for it
// is lost. Pinning to the commit that was polled keeps the stamp honest: a
// later push leaves head != stamp and is reviewed next cycle. GitHub rejects
// (422) a commit that is no longer part of the PR, e.g. after a force-push;
// that surfaces as an error and the PR is retried against the new head.
func PostReviewAtCommit(repo string, number int64, body, verdict, commitOID string) error {
	payload, err := json.Marshal(buildReviewRequest(body, verdict, commitOID))
	if err != nil {
		return fmt.Errorf("encoding review for %s#%d: %w", repo, number, err)
	}

	cmd := exec.Command("gh", "api", "-X", "POST",
		fmt.Sprintf("repos/%s/pulls/%d/reviews", repo, number),
		"--input", "-",
	)
	cmd.Stdin = bytes.NewReader(payload)

	// On an API error gh prints the response body (with GitHub's reason) to
	// stdout and only "HTTP 422" to stderr, so both are kept.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String() + " " + stdout.String())
		if isCommitNotInPR(detail) {
			return fmt.Errorf("posting review on %s#%d at %s: %w", repo, number, shortOID(commitOID), ErrCommitNotInPR)
		}
		if detail != "" {
			return fmt.Errorf("posting review on %s#%d at %s failed: %s: %w", repo, number, shortOID(commitOID), detail, err)
		}
		return fmt.Errorf("posting review on %s#%d at %s failed: %w", repo, number, shortOID(commitOID), err)
	}
	return nil
}

// isCommitNotInPR matches GitHub's 422 for a commit_id outside the PR:
// {"errors":["The commitOID is not part of the pull request"],"status":"422"}.
func isCommitNotInPR(output string) bool {
	return strings.Contains(strings.ToLower(output), "not part of the pull request")
}

// shortOID abbreviates a commit OID for messages.
func shortOID(oid string) string {
	if len(oid) > 7 {
		return oid[:7]
	}
	return oid
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
	HeadRefOID string `json:"headRefOid"`
	Commits    []struct {
		OID string `json:"oid"`
	} `json:"commits"`
}

const prViewFields = "number,title,url,isDraft,additions,deletions,changedFiles,author,labels,headRefOid,commits"

// GetPR fetches a single PR's metadata via `gh pr view`.
func GetPR(repo string, number int64) (PullRequest, error) {
	// Note: `commits` from gh pr view may be capped at 100 entries; the review
	// command treats a reviewed commit missing from a full list as unknown.
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
	pr.HeadOID = v.HeadRefOID
	for _, c := range v.Commits {
		pr.CommitOIDs = append(pr.CommitOIDs, c.OID)
	}
	return pr, nil
}
