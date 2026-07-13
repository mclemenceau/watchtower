// Package launchpad provides a client for the Launchpad REST API,
// used to resolve livefs build log URLs from build page URLs.
package launchpad

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const apiBase = "https://api.launchpad.net/devel"

// HTTPLaunchpadSource implements ports.LaunchpadSource using the Launchpad REST API.
type HTTPLaunchpadSource struct {
	client  *http.Client
	apiBase string // overridable for testing
}

// NewHTTPLaunchpadSource returns an HTTPLaunchpadSource with a sensible timeout.
func NewHTTPLaunchpadSource() *HTTPLaunchpadSource {
	return &HTTPLaunchpadSource{
		client:  &http.Client{Timeout: 15 * time.Second},
		apiBase: apiBase,
	}
}

// NewHTTPLaunchpadSourceWithClient returns an HTTPLaunchpadSource using the
// provided HTTP client and base URL. Intended for testing with httptest servers.
func NewHTTPLaunchpadSourceWithClient(client *http.Client, baseURL string) *HTTPLaunchpadSource {
	return &HTTPLaunchpadSource{client: client, apiBase: baseURL}
}

// FetchBuildLogURL calls the Launchpad REST API for the given build page URL and
// returns the URL of the build log artifact (the build_log_url field).
// Returns ("", nil) when the build has no log yet (build in progress or log unavailable).
//
// The build page URL is expected to follow the pattern:
//
//	https://launchpad.net/~{team}/+livefs/{distro}/{series}/{name}/+build/{id}
//
// which is transformed to:
//
//	https://api.launchpad.net/devel/~{team}/+livefs/{distro}/{series}/{name}/+build/{id}
func (s *HTTPLaunchpadSource) FetchBuildLogURL(ctx context.Context, buildPageURL string) (string, error) {
	apiURL, err := s.pageURLToAPIURL(buildPageURL)
	if err != nil {
		return "", fmt.Errorf("launchpad FetchBuildLogURL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("launchpad FetchBuildLogURL: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("launchpad FetchBuildLogURL: http: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return "", fmt.Errorf("launchpad FetchBuildLogURL: API returned %d: %s", resp.StatusCode, body)
	}

	var payload struct {
		BuildLogURL *string `json:"build_log_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("launchpad FetchBuildLogURL: decode: %w", err)
	}

	if payload.BuildLogURL == nil || *payload.BuildLogURL == "" {
		return "", nil // log not yet available
	}
	return *payload.BuildLogURL, nil
}

// pageURLToAPIURL converts a Launchpad web UI URL to the equivalent REST API URL
// using the client's configured apiBase.
// e.g. https://launchpad.net/~team/+livefs/... → https://api.launchpad.net/devel/~team/+livefs/...
func (s *HTTPLaunchpadSource) pageURLToAPIURL(pageURL string) (string, error) {
	const lpHost = "https://launchpad.net/"
	if !strings.HasPrefix(pageURL, lpHost) {
		return "", fmt.Errorf("not a launchpad.net URL: %q", pageURL)
	}
	path := pageURL[len(lpHost)-1:] // keep leading "/"
	return s.apiBase + path, nil
}

// MockLaunchpadSource is a test double for ports.LaunchpadSource.
type MockLaunchpadSource struct {
	// URL is returned by FetchBuildLogURL when Err is nil.
	URL string
	// Err is returned by FetchBuildLogURL when non-nil.
	Err error
}

func (m *MockLaunchpadSource) FetchBuildLogURL(_ context.Context, _ string) (string, error) {
	return m.URL, m.Err
}
