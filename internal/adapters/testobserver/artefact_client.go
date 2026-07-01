// Package testobserver provides adapters for the Ubuntu Test Observer API.
package testobserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mclemenceau/watchtower/internal/domain"
)

// HTTPArtefactSource fetches Ubuntu image artefacts from the Test Observer API.
type HTTPArtefactSource struct {
	baseURL string
	http    *http.Client
}

// NewHTTPArtefactSource creates a new HTTPArtefactSource targeting the given base URL.
func NewHTTPArtefactSource(baseURL string) *HTTPArtefactSource {
	return &HTTPArtefactSource{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchArtefacts calls GET /v1/artefacts?family=image and returns the result.
func (c *HTTPArtefactSource) FetchArtefacts(ctx context.Context) ([]domain.Artefact, error) {
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

	var artefacts []domain.Artefact
	if err := json.NewDecoder(resp.Body).Decode(&artefacts); err != nil {
		return nil, fmt.Errorf("FetchArtefacts: decode: %w", err)
	}
	return artefacts, nil
}

// HTTPBuildSource fetches build and test execution data from the Test Observer API.
type HTTPBuildSource struct {
	baseURL string
	http    *http.Client
}

// NewHTTPBuildSource creates a new HTTPBuildSource targeting the given base URL.
func NewHTTPBuildSource(baseURL string) *HTTPBuildSource {
	return &HTTPBuildSource{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchBuilds calls GET /v1/artefacts/{id}/builds and returns the result.
func (c *HTTPBuildSource) FetchBuilds(ctx context.Context, artefactID int) ([]domain.ArtefactBuild, error) {
	url := fmt.Sprintf("%s/v1/artefacts/%d/builds", c.baseURL, artefactID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("FetchBuilds: new request: %w", err)
	}
	req.Header.Set("X-CSRF-Token", "1")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("FetchBuilds: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("FetchBuilds: unexpected status %d", resp.StatusCode)
	}

	var builds []domain.ArtefactBuild
	if err := json.NewDecoder(resp.Body).Decode(&builds); err != nil {
		return nil, fmt.Errorf("FetchBuilds: decode: %w", err)
	}
	return builds, nil
}
