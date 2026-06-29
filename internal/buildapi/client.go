package buildapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ArtefactClient fetches Ubuntu image artefacts from a data source.
type ArtefactClient interface {
	FetchArtefacts(ctx context.Context) ([]Artefact, error)
}

// HTTPClient calls the Test Observer API at tests-api.ubuntu.com.
type HTTPClient struct {
	baseURL string
	http    *http.Client
}

func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *HTTPClient) FetchArtefacts(ctx context.Context) ([]Artefact, error) {
	url := c.baseURL + "/v1/artefacts?family=image"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("FetchArtefacts: new request: %w", err)
	}
	req.Header.Set("X-CSRF-Token", "1")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("FetchArtefacts: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("FetchArtefacts: unexpected status %d", resp.StatusCode)
	}

	var artefacts []Artefact
	if err := json.NewDecoder(resp.Body).Decode(&artefacts); err != nil {
		return nil, fmt.Errorf("FetchArtefacts: decode: %w", err)
	}
	return artefacts, nil
}

// MockClient returns a fixed set of artefacts for local dev and tests.
type MockClient struct{}

func NewMockClient() *MockClient { return &MockClient{} }

func (m *MockClient) FetchArtefacts(_ context.Context) ([]Artefact, error) {
	now := time.Now().UTC()
	today := now.Format("20060102")
	yesterday := now.AddDate(0, 0, -1).Format("20060102")
	return []Artefact{
		{ID: 1001, Name: "plucky-desktop-amd64.iso", Version: today, OS: "ubuntu", Release: "plucky", Stage: "pending", Status: "APPROVED"},
		{ID: 1002, Name: "plucky-desktop-arm64.iso", Version: today, OS: "ubuntu", Release: "plucky", Stage: "pending", Status: "UNDECIDED"},
		{ID: 1003, Name: "plucky-server-amd64.iso", Version: today, OS: "ubuntu-server", Release: "plucky", Stage: "pending", Status: "MARKED_AS_FAILED"},
		{ID: 1004, Name: "plucky-minimal-amd64.iso", Version: yesterday, OS: "ubuntu-minimal", Release: "plucky", Stage: "pending", Status: "APPROVED"},
		{ID: 1005, Name: "noble-desktop-amd64.iso", Version: yesterday, OS: "ubuntu", Release: "noble", Stage: "current", Status: "APPROVED"},
	}, nil
}
