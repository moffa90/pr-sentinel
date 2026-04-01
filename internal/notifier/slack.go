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
	status := "posted"
	if !e.Posted {
		status = "dry-run"
	}

	text := fmt.Sprintf(
		"*pr-sentinel*\nPR: <%s|%s#%d %s>\nAuthor: %s\nMode: %s\nStatus: %s\nFindings: %s",
		e.PRURL, e.Repo, e.PRNumber, e.PRTitle,
		e.PRAuthor,
		e.Mode,
		status,
		e.FindingsSummary,
	)

	return slackPayload{Text: text}
}
