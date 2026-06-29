package testapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mclemenceau/watchtower/internal/buildapi"
)

// TestClient fetches test execution data from the Test Observer API.
type TestClient interface {
	FetchBuilds(ctx context.Context, artefactID int) ([]buildapi.ArtefactBuild, error)
}

// HTTPTestClient calls the Test Observer API at tests-api.ubuntu.com.
type HTTPTestClient struct {
	baseURL string
	http    *http.Client
}

func NewHTTPTestClient(baseURL string) *HTTPTestClient {
	return &HTTPTestClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *HTTPTestClient) FetchBuilds(ctx context.Context, artefactID int) ([]buildapi.ArtefactBuild, error) {
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

	var builds []buildapi.ArtefactBuild
	if err := json.NewDecoder(resp.Body).Decode(&builds); err != nil {
		return nil, fmt.Errorf("FetchBuilds: decode: %w", err)
	}
	return builds, nil
}

// MockTestClient returns a fixed set of builds for local dev and tests.
// The artefact IDs match those used in buildapi.MockClient:
//
//	1001 — plucky desktop amd64  → Jenkins FAILED
//	1002 — plucky desktop arm64  → no displayable executions
//	1003 — plucky server amd64   → Jenkins PASSED
//	1004 — plucky minimal amd64  → no displayable executions
//	1005 — noble desktop amd64   → Jenkins PASSED, Manual Testing PASSED
type MockTestClient struct{}

func NewMockTestClient() *MockTestClient { return &MockTestClient{} }

func (m *MockTestClient) FetchBuilds(_ context.Context, artefactID int) ([]buildapi.ArtefactBuild, error) {
	env := func(name, arch string) buildapi.Environment {
		return buildapi.Environment{Name: name, Architecture: arch}
	}

	switch artefactID {
	case 1001:
		return []buildapi.ArtefactBuild{{
			ID: 2001, Architecture: "amd64",
			TestExecutions: []buildapi.TestExecution{
				{ID: 3001, TestPlan: "Image build", Status: "PASSED", Environment: env("cdimage.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T06:00:00"},
				{ID: 3002, TestPlan: "Jenkins image validation", Status: "FAILED", CILink: "https://platform-qa-jenkins.ps5.ubuntu.com/job/ubuntu-plucky-desktop-amd64-iso-static-validation/1/", Environment: env("platform-qa-jenkins.ps5.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T07:00:00"},
				{ID: 3003, TestPlan: "Manual Testing", Status: "IN_PROGRESS", Environment: env("user manual tests", "amd64"), CreatedAt: "2026-06-26T06:01:00"},
			},
		}}, nil
	case 1002:
		return []buildapi.ArtefactBuild{{
			ID: 2002, Architecture: "arm64",
			TestExecutions: []buildapi.TestExecution{
				{ID: 3004, TestPlan: "Image build", Status: "PASSED", Environment: env("cdimage.ubuntu.com", "arm64"), CreatedAt: "2026-06-26T06:00:00"},
				{ID: 3005, TestPlan: "Manual Testing", Status: "IN_PROGRESS", Environment: env("user manual tests", "arm64"), CreatedAt: "2026-06-26T06:01:00"},
			},
		}}, nil
	case 1003:
		return []buildapi.ArtefactBuild{{
			ID: 2003, Architecture: "amd64",
			TestExecutions: []buildapi.TestExecution{
				{ID: 3006, TestPlan: "Image build", Status: "PASSED", Environment: env("cdimage.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T06:00:00"},
				{ID: 3007, TestPlan: "Jenkins image validation", Status: "PASSED", CILink: "https://platform-qa-jenkins.ps5.ubuntu.com/job/ubuntu-plucky-server-amd64-iso-static-validation/1/", Environment: env("platform-qa-jenkins.ps5.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T07:00:00"},
				{ID: 3008, TestPlan: "Manual Testing", Status: "IN_PROGRESS", Environment: env("user manual tests", "amd64"), CreatedAt: "2026-06-26T06:01:00"},
			},
		}}, nil
	case 1004:
		return []buildapi.ArtefactBuild{{
			ID: 2004, Architecture: "amd64",
			TestExecutions: []buildapi.TestExecution{
				{ID: 3009, TestPlan: "Image build", Status: "PASSED", Environment: env("cdimage.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T06:00:00"},
				{ID: 3010, TestPlan: "Manual Testing", Status: "IN_PROGRESS", Environment: env("user manual tests", "amd64"), CreatedAt: "2026-06-26T06:01:00"},
			},
		}}, nil
	case 1005:
		return []buildapi.ArtefactBuild{{
			ID: 2005, Architecture: "amd64",
			TestExecutions: []buildapi.TestExecution{
				{ID: 3011, TestPlan: "Image build", Status: "PASSED", Environment: env("cdimage.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T06:00:00"},
				{ID: 3012, TestPlan: "Jenkins image validation", Status: "PASSED", CILink: "https://platform-qa-jenkins.ps5.ubuntu.com/job/ubuntu-noble-desktop-amd64-iso-static-validation/1/", Environment: env("platform-qa-jenkins.ps5.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T07:00:00"},
				{ID: 3013, TestPlan: "Manual Testing", Status: "PASSED", Environment: env("user manual tests", "amd64"), CreatedAt: "2026-06-26T08:00:00"},
			},
		}}, nil
	}
	return []buildapi.ArtefactBuild{}, nil
}
