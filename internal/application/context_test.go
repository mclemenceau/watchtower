package application

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mclemenceau/watchtower/internal/domain"
)

// makeArtefact is a test helper that constructs an Artefact with the given fields.
func makeArtefact(id int, name, release, os string, buildLog domain.BuildStatusState) domain.Artefact {
	return domain.Artefact{
		ID:       id,
		Name:     name,
		Release:  release,
		OS:       os,
		Version:  "20240101",
		BuildLog: buildLog,
	}
}

func TestBuildContext_ReleaseFilter(t *testing.T) {
	artefacts := []domain.Artefact{
		makeArtefact(1, "noble-desktop-amd64.iso", "noble", "ubuntu-desktop", domain.BuildStatusBuilt),
		makeArtefact(2, "oracular-desktop-amd64.iso", "oracular", "ubuntu-desktop", domain.BuildStatusFailed),
		makeArtefact(3, "noble-server-amd64.iso", "noble", "ubuntu-server", domain.BuildStatusFailed),
	}

	got := BuildContext("show noble failures", artefacts, domain.FailureStore{})

	if got == "" {
		t.Fatal("expected non-empty contextJSON")
	}
	var payload contextPayload
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, got)
	}
	// Should include only noble artefacts (IDs 1 and 3).
	for _, a := range payload.Artefacts {
		if !strings.EqualFold(a.Release, "noble") {
			t.Errorf("unexpected non-noble artefact in context: %+v", a)
		}
	}
	if len(payload.Artefacts) != 2 {
		t.Errorf("expected 2 noble artefacts, got %d", len(payload.Artefacts))
	}
}

func TestBuildContext_ProductFilter(t *testing.T) {
	artefacts := []domain.Artefact{
		makeArtefact(1, "noble-desktop-amd64.iso", "noble", "ubuntu-desktop", domain.BuildStatusFailed),
		makeArtefact(2, "noble-server-amd64.iso", "noble", "ubuntu-server", domain.BuildStatusFailed),
		makeArtefact(3, "noble-server-arm64.iso", "noble", "ubuntu-server", domain.BuildStatusBuilt),
	}

	got := BuildContext("what is wrong with ubuntu-server?", artefacts, domain.FailureStore{})

	if got == "" {
		t.Fatal("expected non-empty contextJSON")
	}
	var payload contextPayload
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// Should only include ubuntu-server artefacts (IDs 2 and 3).
	for _, a := range payload.Artefacts {
		if !strings.Contains(strings.ToLower(a.OS), "server") {
			t.Errorf("unexpected non-server artefact in context: %+v", a)
		}
	}
	if len(payload.Artefacts) != 2 {
		t.Errorf("expected 2 server artefacts, got %d", len(payload.Artefacts))
	}
}

func TestBuildContext_ReleaseAndProductFilter(t *testing.T) {
	artefacts := []domain.Artefact{
		makeArtefact(1, "noble-desktop-amd64.iso", "noble", "ubuntu-desktop", domain.BuildStatusFailed),
		makeArtefact(2, "noble-server-amd64.iso", "noble", "ubuntu-server", domain.BuildStatusFailed),
		makeArtefact(3, "oracular-desktop-amd64.iso", "oracular", "ubuntu-desktop", domain.BuildStatusFailed),
	}

	got := BuildContext("noble desktop failures", artefacts, domain.FailureStore{})

	var payload contextPayload
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// Only noble + desktop → ID 1.
	if len(payload.Artefacts) != 1 || payload.Artefacts[0].ID != 1 {
		t.Errorf("expected artefact ID 1 only, got %+v", payload.Artefacts)
	}
}

func TestBuildContext_FallbackToFailed(t *testing.T) {
	// No known release/product tokens in the message → fallback to FAILED artefacts.
	artefacts := []domain.Artefact{
		makeArtefact(1, "noble-desktop-amd64.iso", "noble", "ubuntu-desktop", domain.BuildStatusBuilt),
		makeArtefact(2, "noble-server-amd64.iso", "noble", "ubuntu-server", domain.BuildStatusFailed),
		makeArtefact(3, "oracular-desktop-amd64.iso", "oracular", "ubuntu-desktop", domain.BuildStatusFailed),
	}

	got := BuildContext("what is going on?", artefacts, domain.FailureStore{})

	var payload contextPayload
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// Only FAILED artefacts (IDs 2 and 3).
	if len(payload.Artefacts) != 2 {
		t.Errorf("expected 2 failed artefacts, got %d: %+v", len(payload.Artefacts), payload.Artefacts)
	}
	for _, a := range payload.Artefacts {
		if a.BuildLog != domain.BuildStatusFailed {
			t.Errorf("expected only FAILED artefacts in fallback context, got %+v", a)
		}
	}
}

func TestBuildContext_EmptyWhenNoData(t *testing.T) {
	// No artefacts and no failures → empty string.
	got := BuildContext("anything", nil, domain.FailureStore{})
	if got != "" {
		t.Errorf("expected empty string for no data, got %q", got)
	}
}

func TestBuildContext_IncludesMatchingFailures(t *testing.T) {
	artefacts := []domain.Artefact{
		makeArtefact(1, "noble-desktop-amd64.iso", "noble", "ubuntu-desktop", domain.BuildStatusFailed),
	}
	failures := domain.FailureStore{
		"noble": {
			"ubuntu-desktop": []domain.FailureRecord{
				{ArtefactID: 1, ArtefactName: "noble-desktop-amd64.iso", Release: "noble", Product: "ubuntu-desktop", Occurrences: 3},
			},
		},
		"oracular": {
			"ubuntu-desktop": []domain.FailureRecord{
				{ArtefactID: 2, ArtefactName: "oracular-desktop-amd64.iso", Release: "oracular", Product: "ubuntu-desktop", Occurrences: 1},
			},
		},
	}

	got := BuildContext("noble failures", artefacts, failures)

	var payload contextPayload
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(payload.Failures) != 1 || payload.Failures[0].Release != "noble" {
		t.Errorf("expected 1 noble failure, got %+v", payload.Failures)
	}
}

func TestBuildContext_CapAtMax(t *testing.T) {
	// Create more artefacts than the cap, all matching the filter.
	artefacts := make([]domain.Artefact, maxContextArtefacts+10)
	for i := range artefacts {
		artefacts[i] = makeArtefact(i+1, "noble-desktop-amd64.iso", "noble", "ubuntu-desktop", domain.BuildStatusFailed)
	}

	got := BuildContext("noble", artefacts, domain.FailureStore{})

	var payload contextPayload
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(payload.Artefacts) != maxContextArtefacts {
		t.Errorf("expected cap of %d artefacts, got %d", maxContextArtefacts, len(payload.Artefacts))
	}
}

func TestBuildContext_BuildsFieldOmitted(t *testing.T) {
	// Verify that the heavy Builds field is not included in the context JSON.
	artefacts := []domain.Artefact{
		{
			ID:      1,
			Name:    "noble-desktop-amd64.iso",
			Release: "noble",
			OS:      "ubuntu-desktop",
			Version: "20240101",
			Builds: []domain.ArtefactBuild{
				{ID: 99, Architecture: "amd64"},
			},
		},
	}

	got := BuildContext("noble", artefacts, domain.FailureStore{})

	if strings.Contains(got, `"builds"`) {
		t.Errorf("context JSON should not contain 'builds' field, got: %s", got)
	}
}
