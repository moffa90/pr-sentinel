package daemon

import (
	"fmt"
	"time"

	"github.com/moffa90/pr-sentinel/internal/state"
)

// Limits on paid model runs for one PR at one head commit.
const (
	// maxAttemptsPerHead parks a head after this many failed runs: a PR that
	// fails every time (timeout, unparsable output, a post GitHub keeps
	// refusing) would otherwise be retried every cycle, around the clock.
	maxAttemptsPerHead = 3
	// attemptBackoff is the wait after the first failure; it doubles after
	// each further one (10m, 20m).
	attemptBackoff = 10 * time.Minute
	// attemptRetention is how long attempt records are kept.
	attemptRetention = 30 * 24 * time.Hour
)

// attemptGate reports whether the daemon may run the model for a head now,
// given what it has already tried there. A new head (the PR moved) starts
// clean, so a parked PR resumes on its next push.
func attemptGate(a state.Attempt, found bool, now time.Time) (bool, string) {
	if !found {
		return true, ""
	}
	if a.Succeeded {
		// The review was posted; even if recording it failed, never pay for
		// the same head twice.
		return false, "already reviewed this head"
	}
	if a.Attempts >= maxAttemptsPerHead {
		return false, fmt.Sprintf("parked after %d failed attempts", a.Attempts)
	}
	wait := attemptBackoff << (a.Attempts - 1)
	if next := a.LastAttemptAt.Add(wait); now.Before(next) {
		return false, fmt.Sprintf("backing off until %s", next.Format(time.RFC3339))
	}
	return true, ""
}
