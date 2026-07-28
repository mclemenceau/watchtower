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

// makeFailedArtefact builds a FAILED artefact with an explicit BuildFailureKind.
func makeFailedArtefact(id int, name, release, os string, kind domain.BuildFailureKind, desc string) domain.Artefact {
	return domain.Artefact{
		ID:                      id,
		Name:                    name,
		Release:                 release,
		OS:                      os,
		Version:                 "20240101",
		BuildLog:                domain.BuildStatusFailed,
		BuildFailureKind:        kind,
		BuildFailureDescription: desc,
	}
}

func TestBuildContext_ReleaseFilter(t *testing.T) {
	artefacts := []domain.Artefact{
		makeArtefact(1, "noble-desktop-amd64.iso", "noble", "ubuntu-desktop", domain.BuildStatusBuilt),
		makeArtefact(2, "oracular-desktop-amd64.iso", "oracular", "ubuntu-desktop", domain.BuildStatusFailed),
		makeArtefact(3, "noble-server-amd64.iso", "noble", "ubuntu-server", domain.BuildStatusFailed),
	}

	got := BuildContext("show noble status", artefacts, domain.FailureStore{})

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

	got := BuildContext("what is the status of ubuntu-server?", artefacts, domain.FailureStore{})

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

	got := BuildContext("noble desktop status", artefacts, domain.FailureStore{})

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

// --- New tests for enriched struct fields and semantic filtering ---

func TestBuildContext_FailureKindInContextArtefact(t *testing.T) {
	// BuildFailureKind and BuildFailureDescription must appear in the context JSON.
	artefacts := []domain.Artefact{
		makeFailedArtefact(1, "noble-desktop-amd64.iso", "noble", "ubuntu-desktop",
			domain.BuildFailureKindInfra, "run_live_builds traceback"),
	}

	got := BuildContext("noble", artefacts, domain.FailureStore{})

	var payload contextPayload
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(payload.Artefacts) != 1 {
		t.Fatalf("expected 1 artefact, got %d", len(payload.Artefacts))
	}
	a := payload.Artefacts[0]
	if a.BuildFailureKind != domain.BuildFailureKindInfra {
		t.Errorf("expected BuildFailureKind=INFRA, got %q", a.BuildFailureKind)
	}
	if a.BuildFailureDescription != "run_live_builds traceback" {
		t.Errorf("expected BuildFailureDescription set, got %q", a.BuildFailureDescription)
	}
}

func TestBuildContext_FailureKindInContextFailure(t *testing.T) {
	// FailureKind, FailureDescription, and Analysis fields must appear in contextFailure.
	now := func() *domain.LogAnalysis {
		a := &domain.LogAnalysis{
			Category:   "infra",
			Hypothesis: "disk full on cdimage host",
			NextAction: "free disk space",
		}
		return a
	}()

	failures := domain.FailureStore{
		"noble": {
			"ubuntu-desktop": []domain.FailureRecord{
				{
					ArtefactID:         1,
					ArtefactName:       "noble-desktop-amd64.iso",
					Release:            "noble",
					Product:            "ubuntu-desktop",
					Occurrences:        2,
					FailureKind:        domain.BuildFailureKindInfra,
					FailureDescription: "run_live_builds traceback",
					Analysis:           now,
				},
			},
		},
	}

	got := BuildContext("noble failures", nil, failures)

	var payload contextPayload
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(payload.Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(payload.Failures))
	}
	f := payload.Failures[0]
	if f.FailureKind != domain.BuildFailureKindInfra {
		t.Errorf("expected FailureKind=INFRA, got %q", f.FailureKind)
	}
	if f.FailureDescription != "run_live_builds traceback" {
		t.Errorf("expected FailureDescription set, got %q", f.FailureDescription)
	}
	if f.AnalysisCategory != "infra" {
		t.Errorf("expected AnalysisCategory=infra, got %q", f.AnalysisCategory)
	}
	if f.AnalysisHypothesis != "disk full on cdimage host" {
		t.Errorf("expected AnalysisHypothesis set, got %q", f.AnalysisHypothesis)
	}
	if f.AnalysisNextAction != "free disk space" {
		t.Errorf("expected AnalysisNextAction set, got %q", f.AnalysisNextAction)
	}
}

func TestBuildContext_FailureSemanticsStripsBuilt(t *testing.T) {
	// When the message contains failure keywords, BUILT artefacts are stripped
	// even when the release filter matched them.
	artefacts := []domain.Artefact{
		makeArtefact(1, "noble-desktop-amd64.iso", "noble", "ubuntu-desktop", domain.BuildStatusBuilt),
		makeArtefact(2, "noble-server-amd64.iso", "noble", "ubuntu-server", domain.BuildStatusFailed),
		makeArtefact(3, "noble-server-arm64.iso", "noble", "ubuntu-server", domain.BuildStatusFailed),
	}

	got := BuildContext("what are the noble failures?", artefacts, domain.FailureStore{})

	var payload contextPayload
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// ID 1 (BUILT) must be absent; IDs 2 and 3 (FAILED) must be present.
	for _, a := range payload.Artefacts {
		if a.BuildLog != domain.BuildStatusFailed {
			t.Errorf("expected only FAILED artefacts when failure semantics active, got %+v", a)
		}
	}
	if len(payload.Artefacts) != 2 {
		t.Errorf("expected 2 failed artefacts, got %d", len(payload.Artefacts))
	}
}

func TestBuildContext_InfraKindFilter(t *testing.T) {
	// "infra" keyword narrows context to BuildFailureKindInfra artefacts only.
	artefacts := []domain.Artefact{
		makeFailedArtefact(1, "noble-desktop-amd64.iso", "noble", "ubuntu-desktop",
			domain.BuildFailureKindInfra, "run_live_builds traceback"),
		makeFailedArtefact(2, "noble-server-amd64.iso", "noble", "ubuntu-server",
			domain.BuildFailureKindProduct, "debootstrap failed"),
		makeArtefact(3, "noble-minimal-amd64.iso", "noble", "ubuntu-minimal", domain.BuildStatusBuilt),
	}

	got := BuildContext("what are the noble infra issues?", artefacts, domain.FailureStore{})

	var payload contextPayload
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// Only ID 1 (INFRA failure) should be in context.
	if len(payload.Artefacts) != 1 || payload.Artefacts[0].ID != 1 {
		t.Errorf("expected only INFRA artefact (ID 1), got %+v", payload.Artefacts)
	}
}

func TestBuildContext_ProductKindFilter(t *testing.T) {
	// "product" keyword narrows context to BuildFailureKindProduct artefacts only.
	artefacts := []domain.Artefact{
		makeFailedArtefact(1, "noble-desktop-amd64.iso", "noble", "ubuntu-desktop",
			domain.BuildFailureKindInfra, "run_live_builds traceback"),
		makeFailedArtefact(2, "noble-server-amd64.iso", "noble", "ubuntu-server",
			domain.BuildFailureKindProduct, "debootstrap failed"),
	}

	got := BuildContext("noble product failures", artefacts, domain.FailureStore{})

	var payload contextPayload
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(payload.Artefacts) != 1 || payload.Artefacts[0].ID != 2 {
		t.Errorf("expected only PRODUCT artefact (ID 2), got %+v", payload.Artefacts)
	}
}

func TestBuildContext_InfraKindFilterOnFailures(t *testing.T) {
	// "infra" keyword also filters the failures slice by FailureKind.
	failures := domain.FailureStore{
		"noble": {
			"ubuntu-desktop": []domain.FailureRecord{
				{ArtefactID: 1, ArtefactName: "noble-desktop-amd64.iso", Release: "noble", Product: "ubuntu-desktop",
					Occurrences: 1, FailureKind: domain.BuildFailureKindInfra},
			},
			"ubuntu-server": []domain.FailureRecord{
				{ArtefactID: 2, ArtefactName: "noble-server-amd64.iso", Release: "noble", Product: "ubuntu-server",
					Occurrences: 1, FailureKind: domain.BuildFailureKindProduct},
			},
		},
	}

	got := BuildContext("noble infra failures", nil, failures)

	var payload contextPayload
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(payload.Failures) != 1 || payload.Failures[0].ArtefactID != 1 {
		t.Errorf("expected only INFRA failure (ID 1), got %+v", payload.Failures)
	}
}

func TestBuildContext_FallbackInfraKindFilter(t *testing.T) {
	// When no release/product matches but "infra" is present, fallback still
	// respects the kind filter.
	artefacts := []domain.Artefact{
		makeFailedArtefact(1, "noble-desktop-amd64.iso", "noble", "ubuntu-desktop",
			domain.BuildFailureKindInfra, "crash"),
		makeFailedArtefact(2, "noble-server-amd64.iso", "noble", "ubuntu-server",
			domain.BuildFailureKindProduct, "dep conflict"),
	}

	got := BuildContext("show me all infra problems", artefacts, domain.FailureStore{})

	var payload contextPayload
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(payload.Artefacts) != 1 || payload.Artefacts[0].ID != 1 {
		t.Errorf("expected only INFRA artefact (ID 1) in fallback+kind, got %+v", payload.Artefacts)
	}
}
