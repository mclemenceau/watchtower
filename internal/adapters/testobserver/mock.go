package testobserver

import (
	"context"
	"time"

	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/ports"
)

// Compile-time interface satisfaction checks.
var _ ports.ArtefactSource = (*HTTPArtefactSource)(nil)
var _ ports.BuildSource = (*HTTPBuildSource)(nil)
var _ ports.ArtefactSource = (*MockArtefactSource)(nil)
var _ ports.BuildSource = (*MockBuildSource)(nil)

// MockArtefactSource returns a fixed set of artefacts for local dev and tests.
type MockArtefactSource struct {
	Artefacts []domain.Artefact
	Err       error
}

// FetchArtefacts returns the pre-configured artefacts (or Err if set).
// When Artefacts is nil, a realistic default set is returned.
func (m *MockArtefactSource) FetchArtefacts(_ context.Context) ([]domain.Artefact, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.Artefacts != nil {
		return m.Artefacts, nil
	}
	// Default realistic data matching the old buildapi.MockClient output.
	now := time.Now().UTC()
	today := now.Format("20060102")
	yesterday := now.AddDate(0, 0, -1).Format("20060102")
	return []domain.Artefact{
		{ID: 1001, Name: "plucky-desktop-amd64.iso", Version: today, OS: "ubuntu", Release: "plucky", Stage: "pending", Status: "APPROVED"},
		{ID: 1002, Name: "plucky-desktop-arm64.iso", Version: today, OS: "ubuntu", Release: "plucky", Stage: "pending", Status: "UNDECIDED"},
		{ID: 1003, Name: "plucky-server-amd64.iso", Version: today, OS: "ubuntu-server", Release: "plucky", Stage: "pending", Status: "MARKED_AS_FAILED"},
		{ID: 1004, Name: "plucky-minimal-amd64.iso", Version: yesterday, OS: "ubuntu-minimal", Release: "plucky", Stage: "pending", Status: "APPROVED"},
		{ID: 1005, Name: "noble-desktop-amd64.iso", Version: yesterday, OS: "ubuntu", Release: "noble", Stage: "current", Status: "APPROVED"},
	}, nil
}

// MockBuildSource returns a fixed set of builds for local dev and tests.
// The artefact IDs match those used in MockArtefactSource's default data:
//
//	1001 — plucky desktop amd64  → Jenkins FAILED
//	1002 — plucky desktop arm64  → no displayable executions
//	1003 — plucky server amd64   → Jenkins PASSED
//	1004 — plucky minimal amd64  → no displayable executions
//	1005 — noble desktop amd64   → Jenkins PASSED, Manual Testing PASSED
type MockBuildSource struct {
	Builds map[int][]domain.ArtefactBuild
	Err    error
}

// FetchBuilds returns the pre-configured builds for artefactID (or Err if set).
// When Builds is nil, a realistic default set is returned.
func (m *MockBuildSource) FetchBuilds(_ context.Context, artefactID int) ([]domain.ArtefactBuild, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.Builds != nil {
		return m.Builds[artefactID], nil
	}
	return defaultBuilds(artefactID), nil
}

func defaultBuilds(artefactID int) []domain.ArtefactBuild {
	env := func(name, arch string) domain.Environment {
		return domain.Environment{Name: name, Architecture: arch}
	}
	switch artefactID {
	case 1001:
		return []domain.ArtefactBuild{{
			ID: 2001, Architecture: "amd64",
			TestExecutions: []domain.TestExecution{
				{ID: 3001, TestPlan: "Image build", Status: "PASSED", Environment: env("cdimage.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T06:00:00"},
				{ID: 3002, TestPlan: "Jenkins image validation", Status: "FAILED", CILink: "https://platform-qa-jenkins.ps5.ubuntu.com/job/ubuntu-plucky-desktop-amd64-iso-static-validation/1/", Environment: env("platform-qa-jenkins.ps5.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T07:00:00"},
				{ID: 3003, TestPlan: "Manual Testing", Status: "IN_PROGRESS", Environment: env("user manual tests", "amd64"), CreatedAt: "2026-06-26T06:01:00"},
			},
		}}
	case 1002:
		return []domain.ArtefactBuild{{
			ID: 2002, Architecture: "arm64",
			TestExecutions: []domain.TestExecution{
				{ID: 3004, TestPlan: "Image build", Status: "PASSED", Environment: env("cdimage.ubuntu.com", "arm64"), CreatedAt: "2026-06-26T06:00:00"},
				{ID: 3005, TestPlan: "Manual Testing", Status: "IN_PROGRESS", Environment: env("user manual tests", "arm64"), CreatedAt: "2026-06-26T06:01:00"},
			},
		}}
	case 1003:
		return []domain.ArtefactBuild{{
			ID: 2003, Architecture: "amd64",
			TestExecutions: []domain.TestExecution{
				{ID: 3006, TestPlan: "Image build", Status: "PASSED", Environment: env("cdimage.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T06:00:00"},
				{ID: 3007, TestPlan: "Jenkins image validation", Status: "PASSED", CILink: "https://platform-qa-jenkins.ps5.ubuntu.com/job/ubuntu-plucky-server-amd64-iso-static-validation/1/", Environment: env("platform-qa-jenkins.ps5.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T07:00:00"},
				{ID: 3008, TestPlan: "Manual Testing", Status: "IN_PROGRESS", Environment: env("user manual tests", "amd64"), CreatedAt: "2026-06-26T06:01:00"},
			},
		}}
	case 1004:
		return []domain.ArtefactBuild{{
			ID: 2004, Architecture: "amd64",
			TestExecutions: []domain.TestExecution{
				{ID: 3009, TestPlan: "Image build", Status: "PASSED", Environment: env("cdimage.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T06:00:00"},
				{ID: 3010, TestPlan: "Manual Testing", Status: "IN_PROGRESS", Environment: env("user manual tests", "amd64"), CreatedAt: "2026-06-26T06:01:00"},
			},
		}}
	case 1005:
		return []domain.ArtefactBuild{{
			ID: 2005, Architecture: "amd64",
			TestExecutions: []domain.TestExecution{
				{ID: 3011, TestPlan: "Image build", Status: "PASSED", Environment: env("cdimage.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T06:00:00"},
				{ID: 3012, TestPlan: "Jenkins image validation", Status: "PASSED", CILink: "https://platform-qa-jenkins.ps5.ubuntu.com/job/ubuntu-noble-desktop-amd64-iso-static-validation/1/", Environment: env("platform-qa-jenkins.ps5.ubuntu.com", "amd64"), CreatedAt: "2026-06-26T07:00:00"},
				{ID: 3013, TestPlan: "Manual Testing", Status: "PASSED", Environment: env("user manual tests", "amd64"), CreatedAt: "2026-06-26T08:00:00"},
			},
		}}
	}
	return []domain.ArtefactBuild{}
}
