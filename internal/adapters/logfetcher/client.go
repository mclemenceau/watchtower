// Package logfetcher provides an HTTP implementation of ports.LogFetcher.
// It supports gzip-compressed responses and maps HTTP 404 to domain.ErrLogNotFound
// so callers can distinguish "log not yet available" from genuine errors.
package logfetcher

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/ports"
)

// Compile-time interface satisfaction check.
var _ ports.LogFetcher = (*HTTPLogFetcher)(nil)

// HTTPLogFetcher fetches build logs over HTTP with gzip support.
// A 404 response returns domain.ErrLogNotFound; all other non-200 responses
// return a wrapped error containing the status code.
type HTTPLogFetcher struct {
	client *http.Client
}

// New returns an HTTPLogFetcher with the given timeout.
// Pass 0 to use the default timeout (30 s).
func New(timeout time.Duration) *HTTPLogFetcher {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &HTTPLogFetcher{client: &http.Client{Timeout: timeout}}
}

// Fetch retrieves the content at url.
// Returns (content, nil) on HTTP 200.
// Returns ("", domain.ErrLogNotFound) on HTTP 404.
// Returns ("", err) on any other failure.
func (f *HTTPLogFetcher) Fetch(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("logfetcher: new request: %w", err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("logfetcher: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusNotFound {
		return "", domain.ErrLogNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("logfetcher: unexpected status %d for %s", resp.StatusCode, url)
	}

	body := io.Reader(resp.Body)
	// Decompress gzip responses when Go's transport has not already done so.
	needsGzip := !resp.Uncompressed && (strings.HasSuffix(strings.ToLower(url), ".gz") ||
		resp.Header.Get("Content-Type") == "application/x-gzip" ||
		resp.Header.Get("Content-Type") == "application/gzip")
	if needsGzip {
		gr, gzErr := gzip.NewReader(resp.Body)
		if gzErr != nil {
			return "", fmt.Errorf("logfetcher: gzip open: %w", gzErr)
		}
		defer gr.Close() //nolint:errcheck
		body = gr
	}

	raw, err := io.ReadAll(body)
	if err != nil {
		return "", fmt.Errorf("logfetcher: read: %w", err)
	}
	return string(raw), nil
}

// MockLogFetcher is a test double for ports.LogFetcher.
// Set Content to return a fixed string, or Err to return an error.
// Use domain.ErrLogNotFound as Err to simulate a 404.
type MockLogFetcher struct {
	Content string
	Err     error
}

// Compile-time interface satisfaction check.
var _ ports.LogFetcher = (*MockLogFetcher)(nil)

// Fetch returns the pre-configured Content or Err.
func (m *MockLogFetcher) Fetch(_ context.Context, _ string) (string, error) {
	if m.Err != nil {
		return "", m.Err
	}
	return m.Content, nil
}
