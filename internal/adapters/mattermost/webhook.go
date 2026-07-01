// Package mattermost provides the Mattermost adapter implementing ports.Notifier.
package mattermost

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mclemenceau/watchtower/internal/ports"
)

// Compile-time interface satisfaction checks.
var _ ports.Notifier = (*StdoutNotifier)(nil)
var _ ports.Notifier = (*HTTPNotifier)(nil)

// StdoutNotifier writes messages to stdout — simulates a Mattermost channel.
// Each message is prefixed with "[Watchtower →]" to distinguish agent output from user input.
type StdoutNotifier struct{}

// Send prints the message to stdout.
func (s *StdoutNotifier) Send(text string) error {
	fmt.Printf("\n[Watchtower →]\n%s\n", text)
	return nil
}

// HTTPNotifier sends messages to a real Mattermost incoming webhook URL.
type HTTPNotifier struct {
	url  string
	http *http.Client
}

// NewHTTPNotifier creates a new HTTPNotifier that posts to the given Mattermost
// incoming webhook URL.
func NewHTTPNotifier(webhookURL string) *HTTPNotifier {
	return &HTTPNotifier{
		url:  webhookURL,
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

type webhookPayload struct {
	Text string `json:"text"`
}

// Send posts the message to the Mattermost incoming webhook.
func (h *HTTPNotifier) Send(text string) error {
	payload, err := json.Marshal(webhookPayload{Text: text})
	if err != nil {
		return fmt.Errorf("mattermost webhook: marshal: %w", err)
	}

	resp, err := h.http.Post(h.url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("mattermost webhook: post: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mattermost webhook: unexpected status %d", resp.StatusCode)
	}
	return nil
}
