package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

var pushHTTP = &http.Client{Timeout: 5 * time.Second}

// PushToFeed POSTs a message to the server's /internal/push endpoint,
// which fans it out to all connected SSE clients.
func (a *Activities) PushToFeed(ctx context.Context, data string) error {
	body, err := json.Marshal(map[string]string{"data": data})
	if err != nil {
		return fmt.Errorf("PushToFeed: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.FeedURL+"/internal/push", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("PushToFeed: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := pushHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("PushToFeed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("PushToFeed: unexpected status %d", resp.StatusCode)
	}
	return nil
}
