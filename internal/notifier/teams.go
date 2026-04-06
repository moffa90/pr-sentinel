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

// Notify sends a Teams Adaptive Card via the configured webhook URL.
func (t *TeamsNotifier) Notify(e Event) error {
	payload := buildTeamsPayload(e)
	return postJSON(t.webhookURL, payload)
}

// buildTeamsPayload formats the event into a Teams Adaptive Card message.
func buildTeamsPayload(e Event) map[string]interface{} {
	statusText, modeEmoji := verdictDisplay(e)

	title := fmt.Sprintf("%s #%d", e.Repo, e.PRNumber)
	if e.PRNumber == 0 {
		title = e.Repo
	}

	body := []interface{}{
		map[string]interface{}{
			"type": "ColumnSet",
			"columns": []interface{}{
				map[string]interface{}{
					"type":  "Column",
					"width": "auto",
					"items": []interface{}{
						map[string]interface{}{
							"type":  "TextBlock",
							"text":  "🛡",
							"size":  "Large",
							"style": "heading",
						},
					},
					"verticalContentAlignment": "Center",
				},
				map[string]interface{}{
					"type":  "Column",
					"width": "stretch",
					"items": []interface{}{
						map[string]interface{}{
							"type":   "TextBlock",
							"text":   "pr-sentinel",
							"size":   "Medium",
							"weight": "Bolder",
						},
						map[string]interface{}{
							"type":     "TextBlock",
							"text":     title,
							"spacing":  "None",
							"isSubtle": true,
							"size":     "Small",
						},
					},
				},
				map[string]interface{}{
					"type":  "Column",
					"width": "auto",
					"items": []interface{}{
						map[string]interface{}{
							"type":   "TextBlock",
							"text":   fmt.Sprintf("%s %s", modeEmoji, statusText),
							"weight": "Bolder",
							"size":   "Small",
						},
					},
					"verticalContentAlignment": "Center",
				},
			},
		},
		map[string]interface{}{
			"type":    "TextBlock",
			"text":    e.PRTitle,
			"weight":  "Bolder",
			"wrap":    true,
			"spacing": "Medium",
		},
	}

	if e.Summary != "" {
		body = append(body, map[string]interface{}{
			"type":     "TextBlock",
			"text":     e.Summary,
			"wrap":     true,
			"isSubtle": true,
			"spacing":  "Small",
		})
	}

	facts := []interface{}{
		map[string]interface{}{
			"title": "Author",
			"value": fmt.Sprintf("@%s", e.PRAuthor),
		},
		map[string]interface{}{
			"title": "Changes",
			"value": e.FindingsSummary,
		},
	}

	if e.AutoMerge != "" {
		facts = append(facts, map[string]interface{}{
			"title": "Auto-merge",
			"value": e.AutoMerge,
		})
	}

	body = append(body, map[string]interface{}{
		"type":  "FactSet",
		"facts": facts,
	})

	actions := []interface{}{}
	if e.PRURL != "" {
		actions = append(actions, map[string]interface{}{
			"type":  "Action.OpenUrl",
			"title": "View Pull Request",
			"url":   e.PRURL,
		})
	}

	card := map[string]interface{}{
		"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
		"type":    "AdaptiveCard",
		"version": "1.4",
		"body":    body,
		"actions": actions,
	}

	return map[string]interface{}{
		"type": "message",
		"attachments": []interface{}{
			map[string]interface{}{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content":     card,
			},
		},
	}
}

func verdictDisplay(e Event) (string, string) {
	if e.Mode == "test" {
		return "Test", "🧪"
	}

	suffix := ""
	if e.Mode == "dry-run" && !e.Posted {
		suffix = " (Dry Run)"
	}

	switch e.Verdict {
	case "approve":
		return "Approved" + suffix, "✅"
	case "comment":
		return "Comment" + suffix, "💬"
	case "request-changes":
		return "Changes Requested" + suffix, "❌"
	default:
		if e.Posted {
			return "Posted" + suffix, "🟢"
		}
		return "Reviewed" + suffix, "📋"
	}
}
