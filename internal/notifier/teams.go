package notifier

import "fmt"

// TeamsNotifier sends notifications to a Microsoft Teams incoming webhook.
type TeamsNotifier struct {
	webhookURL string
}

// NewTeamsNotifier creates a new TeamsNotifier.
func NewTeamsNotifier(webhookURL string) *TeamsNotifier {
	return &TeamsNotifier{webhookURL: webhookURL}
}

type teamsPayload struct {
	Type        string            `json:"type"`
	Attachments []teamsAttachment `json:"attachments"`
}

type teamsAttachment struct {
	ContentType string       `json:"contentType"`
	Content     adaptiveCard `json:"content"`
}

type adaptiveCard struct {
	Schema  string            `json:"$schema"`
	Type    string            `json:"type"`
	Version string            `json:"version"`
	Body    []adaptiveElement `json:"body"`
	Actions []adaptiveAction  `json:"actions,omitempty"`
}

type adaptiveElement struct {
	Type   string `json:"type"`
	Text   string `json:"text"`
	Size   string `json:"size,omitempty"`
	Weight string `json:"weight,omitempty"`
	Wrap   bool   `json:"wrap,omitempty"`
}

type adaptiveAction struct {
	Type  string `json:"type"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Notify sends a Teams Adaptive Card via the configured webhook URL.
func (t *TeamsNotifier) Notify(e Event) error {
	payload := buildTeamsPayload(e)
	return postJSON(t.webhookURL, payload)
}

// buildTeamsPayload formats the event into a Teams Adaptive Card message.
func buildTeamsPayload(e Event) teamsPayload {
	status := "posted"
	if !e.Posted {
		status = "dry-run"
	}

	card := adaptiveCard{
		Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
		Type:    "AdaptiveCard",
		Version: "1.4",
		Body: []adaptiveElement{
			{Type: "TextBlock", Text: "pr-sentinel", Size: "Large", Weight: "Bolder"},
			{Type: "TextBlock", Text: fmt.Sprintf("PR: %s#%d %s", e.Repo, e.PRNumber, e.PRTitle), Wrap: true},
			{Type: "TextBlock", Text: fmt.Sprintf("Author: %s", e.PRAuthor)},
			{Type: "TextBlock", Text: fmt.Sprintf("Mode: %s | Status: %s", e.Mode, status)},
			{Type: "TextBlock", Text: fmt.Sprintf("Findings: %s", e.FindingsSummary), Wrap: true},
		},
		Actions: []adaptiveAction{
			{Type: "Action.OpenUrl", Title: "View PR", URL: e.PRURL},
		},
	}

	return teamsPayload{
		Type: "message",
		Attachments: []teamsAttachment{
			{
				ContentType: "application/vnd.microsoft.card.adaptive",
				Content:     card,
			},
		},
	}
}
