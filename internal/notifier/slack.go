package notifier

import "fmt"

// SlackNotifier sends notifications to a Slack incoming webhook.
type SlackNotifier struct {
	webhookURL string
}

// NewSlackNotifier creates a new SlackNotifier.
func NewSlackNotifier(webhookURL string) *SlackNotifier {
	return &SlackNotifier{webhookURL: webhookURL}
}

type slackPayload struct {
	Text string `json:"text"`
}

// Notify sends a Slack message via the configured webhook URL.
func (s *SlackNotifier) Notify(e Event) error {
	payload := buildSlackPayload(e)
	return postJSON(s.webhookURL, payload)
}

// buildSlackPayload formats the event into a Slack mrkdwn message.
func buildSlackPayload(e Event) slackPayload {
	verdict := e.Verdict
	if verdict == "" {
		verdict = "reviewed"
	}

	text := fmt.Sprintf(
		"*pr-sentinel*\nPR: <%s|%s#%d %s>\nAuthor: %s\nVerdict: %s\nMode: %s\nFindings: %s",
		e.PRURL, e.Repo, e.PRNumber, e.PRTitle,
		e.PRAuthor,
		verdict,
		e.Mode,
		e.FindingsSummary,
	)

	if e.Summary != "" {
		text += fmt.Sprintf("\nSummary: %s", e.Summary)
	}

	if e.AutoMerge != "" {
		text += fmt.Sprintf("\nAuto-merge: %s", e.AutoMerge)
	}

	return slackPayload{Text: text}
}
