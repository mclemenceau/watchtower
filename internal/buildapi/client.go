package buildapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type BuildClient interface {
	FetchBuilds(ctx context.Context) ([]Image, error)
}

// HTTPClient calls the real FastAPI build-status service.
type HTTPClient struct {
	baseURL string
	http    *http.Client
}

func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *HTTPClient) FetchBuilds(ctx context.Context) ([]Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/builds", nil)
	if err != nil {
		return nil, fmt.Errorf("FetchBuilds: new request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("FetchBuilds: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("FetchBuilds: unexpected status %d", resp.StatusCode)
	}

	var images []Image
	if err := json.NewDecoder(resp.Body).Decode(&images); err != nil {
		return nil, fmt.Errorf("FetchBuilds: decode: %w", err)
	}
	return images, nil
}

// MockClient returns a fixed set of 5 mixed-status images for local dev and tests.
type MockClient struct{}

func NewMockClient() *MockClient { return &MockClient{} }

func (m *MockClient) FetchBuilds(_ context.Context) ([]Image, error) {
	now := time.Now().UTC()
	return []Image{
		{
			ID:         "ubuntu-desktop-amd64",
			Package:    "ubuntu-desktop",
			Series:     "plucky",
			Arch:       "amd64",
			Status:     "SUCCESS",
			StartedAt:  now.Add(-90 * time.Minute),
			FinishedAt: now.Add(-60 * time.Minute),
			LogURL:     "http://localhost:8000/logs/ubuntu-desktop-amd64",
		},
		{
			ID:         "ubuntu-server-amd64",
			Package:    "ubuntu-server",
			Series:     "plucky",
			Arch:       "amd64",
			Status:     "FAILED",
			StartedAt:  now.Add(-45 * time.Minute),
			FinishedAt: now.Add(-30 * time.Minute),
			LogURL:     "http://localhost:8000/logs/ubuntu-server-amd64",
		},
		{
			ID:        "ubuntu-desktop-arm64",
			Package:   "ubuntu-desktop",
			Series:    "plucky",
			Arch:      "arm64",
			Status:    "BUILDING",
			StartedAt: now.Add(-20 * time.Minute),
			LogURL:    "http://localhost:8000/logs/ubuntu-desktop-arm64",
		},
		{
			ID:         "ubuntu-server-arm64",
			Package:    "ubuntu-server",
			Series:     "plucky",
			Arch:       "arm64",
			Status:     "CANCELLED",
			StartedAt:  now.Add(-120 * time.Minute),
			FinishedAt: now.Add(-110 * time.Minute),
			LogURL:     "http://localhost:8000/logs/ubuntu-server-arm64",
		},
		{
			ID:         "ubuntu-minimal-amd64",
			Package:    "ubuntu-minimal",
			Series:     "plucky",
			Arch:       "amd64",
			Status:     "SUCCESS",
			StartedAt:  now.Add(-180 * time.Minute),
			FinishedAt: now.Add(-150 * time.Minute),
			LogURL:     "http://localhost:8000/logs/ubuntu-minimal-amd64",
		},
	}, nil
}
