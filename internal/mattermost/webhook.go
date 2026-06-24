// Package mattermost provides the Mattermost I/O abstraction for ARGUS.
// In production, WebhookClient sends messages to a real Mattermost incoming webhook.
// In simulation mode (no MATTERMOST_WEBHOOK_URL), StdoutWebhookClient prints to stdout.
package mattermost

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WebhookClient is the interface used to send messages to a Mattermost channel.
// The same interface is used by activities (proactive notifications) and the
// REPL dispatcher (reactive replies).
type WebhookClient interface {
	Send(text string) error
}

// StdoutWebhookClient writes messages to stdout — simulates a Mattermost channel.
// Each message is prefixed with "[ARGUS →]" to distinguish agent output from user input.
type StdoutWebhookClient struct{}

func (s *StdoutWebhookClient) Send(text string) error {
	fmt.Printf("\n[ARGUS →]\n%s\n", text)
	return nil
}

// HTTPWebhookClient sends messages to a real Mattermost incoming webhook URL.
type HTTPWebhookClient struct {
	url  string
	http *http.Client
}

func NewHTTPWebhookClient(webhookURL string) *HTTPWebhookClient {
	return &HTTPWebhookClient{
		url:  webhookURL,
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

type webhookPayload struct {
	Text string `json:"text"`
}

func (h *HTTPWebhookClient) Send(text string) error {
	payload, err := json.Marshal(webhookPayload{Text: text})
	if err != nil {
		return fmt.Errorf("mattermost webhook: marshal: %w", err)
	}

	resp, err := h.http.Post(h.url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("mattermost webhook: post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mattermost webhook: unexpected status %d", resp.StatusCode)
	}
	return nil
}
