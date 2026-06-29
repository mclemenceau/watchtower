package activities

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var fetchLogHTTP = &http.Client{Timeout: 30 * time.Second}

// FetchLog GETs the log URL and returns the last 200 lines.
func (a *Activities) FetchLog(ctx context.Context, logURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, logURL, nil)
	if err != nil {
		return "", fmt.Errorf("FetchLog: new request: %w", err)
	}

	resp, err := fetchLogHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("FetchLog: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("FetchLog: unexpected status %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("FetchLog: read: %w", err)
	}

	return lastN(string(raw), 200), nil
}

func lastN(text string, n int) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= n {
		return text
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
