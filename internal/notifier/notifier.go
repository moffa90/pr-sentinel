package notifier

import (
	"errors"
	"time"
)

// Event represents a notification event for a PR review.
type Event struct {
	Repo            string `json:"repo"`
	PRNumber        int64  `json:"pr_number"`
	PRTitle         string `json:"pr_title"`
	PRAuthor        string `json:"pr_author"`
	PRURL           string `json:"pr_url"`
	Mode            string `json:"mode"`
	Posted          bool   `json:"posted"`
	FindingsSummary string `json:"findings_summary"`
	ReviewPath      string `json:"review_path,omitempty"`
	Timestamp       string `json:"timestamp"`
}

// NewEvent creates an Event with the current timestamp.
func NewEvent(repo string, prNumber int64, prTitle, prAuthor, prURL, mode string, posted bool, findingsSummary, reviewPath string) Event {
	return Event{
		Repo:            repo,
		PRNumber:        prNumber,
		PRTitle:         prTitle,
		PRAuthor:        prAuthor,
		PRURL:           prURL,
		Mode:            mode,
		Posted:          posted,
		FindingsSummary: findingsSummary,
		ReviewPath:      reviewPath,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	}
}

// Notifier is the interface for sending notifications.
type Notifier interface {
	Notify(event Event) error
}

// Dispatcher fans out notifications to multiple notifiers.
type Dispatcher struct {
	notifiers []Notifier
}

// NewDispatcher creates a Dispatcher with the given notifiers.
func NewDispatcher(notifiers ...Notifier) *Dispatcher {
	return &Dispatcher{notifiers: notifiers}
}

// Notify calls all notifiers and collects any errors.
func (d *Dispatcher) Notify(event Event) error {
	var errs []error
	for _, n := range d.notifiers {
		if err := n.Notify(event); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
