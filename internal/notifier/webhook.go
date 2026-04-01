package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// WebhookNotifier sends the raw Event as JSON to a URL.
type WebhookNotifier struct {
	url string
}

// NewWebhookNotifier creates a new WebhookNotifier.
func NewWebhookNotifier(url string) *WebhookNotifier {
	return &WebhookNotifier{url: url}
}

// Notify posts the Event as JSON to the configured URL.
func (w *WebhookNotifier) Notify(e Event) error {
	return postJSON(w.url, e)
}

// postJSON marshals payload to JSON and POSTs it to the given URL.
// This helper is shared by all webhook-based notifiers.
func postJSON(url string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("post to %s: %w", url, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook %s returned status %d", url, resp.StatusCode)
	}
	return nil
}
