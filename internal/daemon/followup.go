package daemon

import (
	"github.com/moffa90/pr-sentinel/internal/github"
	"github.com/moffa90/pr-sentinel/internal/state"
)

// planKind is what a poll cycle should do with one open PR.
type planKind int

const (
	planSkip planKind = iota
	planNew
	planFollowUp
)

// reviewPlan is the decision for one PR, with the follow-up context.
type reviewPlan struct {
	kind       planKind
	prevOID    string // commit the previous review was written against
	newCommits int
	rewritten  bool
	// adoptBaseline: a stored review predates head_oid and nothing else says
	// which commit it covered. Record the current head and skip, rather than
	// treat "unknown" as "changed" and re-review every such PR at once.
	adoptBaseline bool
	reason        string
}

// planUnreviewedOnGitHub decides for a PR with no pr-sentinel review on
// GitHub. The stored review (if any) is the only evidence: dry-run repos never
// post, and a repo promoted from dry-run has state but no GitHub review.
func planUnreviewedOnGitHub(rec *state.ReviewRecord, pr github.PullRequest) reviewPlan {
	if rec == nil {
		return reviewPlan{kind: planNew}
	}
	if pr.HeadOID == "" {
		return reviewPlan{kind: planSkip, reason: "head commit unknown"}
	}
	if rec.HeadOID == "" {
		return reviewPlan{kind: planSkip, adoptBaseline: true, reason: "review predates head tracking"}
	}
	if rec.HeadOID == pr.HeadOID {
		return reviewPlan{kind: planSkip, reason: "already reviewed at head"}
	}
	return followUpFrom(pr, rec.HeadOID)
}

// planForCandidate decides for a PR whose pr-sentinel review on GitHub was
// written against a commit other than the head (or against no known commit).
//
// The newer of the two records wins: the stored review is written after the
// GitHub post, so for the same review it is always newer, and a GitHub review
// newer than the stored one came from another host running pr-sentinel.
func planForCandidate(rec *state.ReviewRecord, c github.FollowUpCandidate) reviewPlan {
	stored := rec != nil && rec.HeadOID != ""
	if stored && rec.HeadOID == c.HeadOID {
		return reviewPlan{kind: planSkip, reason: "already reviewed at head"}
	}

	prev := c.LastReviewOID
	if stored && (prev == "" || rec.ReviewedAt.After(c.LastReviewAt)) {
		prev = rec.HeadOID
	}

	if prev == "" {
		// GitHub returned no commit for the review and state has none either.
		// Unknown never counts as different.
		if rec != nil {
			return reviewPlan{kind: planSkip, adoptBaseline: true, reason: "reviewed commit unknown"}
		}
		return reviewPlan{kind: planSkip, reason: "reviewed commit unknown"}
	}
	if prev == c.HeadOID {
		return reviewPlan{kind: planSkip, reason: "already reviewed at head"}
	}
	return followUpFrom(c.PullRequest, prev)
}

// followUpFrom builds a follow-up plan relative to the reviewed commit.
func followUpFrom(pr github.PullRequest, prevOID string) reviewPlan {
	_, count, rewritten := github.CommitsSince(pr.CommitOIDs, prevOID)
	return reviewPlan{kind: planFollowUp, prevOID: prevOID, newCommits: count, rewritten: rewritten}
}
