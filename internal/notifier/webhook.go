package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// webhookClient is used for all webhook HTTP requests with a sane timeout.
var webhookClient = &http.Client{Timeout: 15 * time.Second}

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

	resp, err := webhookClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook request to %s failed: %w", redactURL(url), err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook %s returned status %d", redactURL(url), resp.StatusCode)
	}
	return nil
}

// redactURL returns only the host portion of a URL to avoid leaking tokens
// embedded in webhook paths.
func redactURL(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		return u.Scheme + "://" + u.Host + "/..."
	}
	if len(rawURL) > 30 {
		return rawURL[:30] + "..."
	}
	return rawURL
}
