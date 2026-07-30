package state_test

import (
	"testing"
	"time"

	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/state"
)

func TestFailureStateRoundTrip(t *testing.T) {
	path := t.TempDir() + "/failures.json"
	fs := state.NewFailureState(path)

	// Empty read on first boot should return an empty store, not an error.
	store, err := fs.ReadFailures()
	if err != nil {
		t.Fatalf("ReadFailures on missing file: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil empty store")
	}

	// Write a record and read it back.
	store.UpsertFailure(domain.Artefact{
		ID: 1, Name: "ubuntu-desktop-amd64", OS: "ubuntu-desktop",
		Release: "noble", Version: "20260720",
	})
	if err := fs.WriteFailures(store); err != nil {
		t.Fatalf("WriteFailures: %v", err)
	}

	loaded, err := fs.ReadFailures()
	if err != nil {
		t.Fatalf("ReadFailures after write: %v", err)
	}
	records := loaded["noble"]["ubuntu-desktop"]
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]
	if r.ArtefactID != 1 || r.ArtefactName != "ubuntu-desktop-amd64" {
		t.Errorf("unexpected record: %+v", r)
	}
	if r.Occurrences != 1 {
		t.Errorf("expected Occurrences=1, got %d", r.Occurrences)
	}
	if r.Resolved {
		t.Error("expected Resolved=false")
	}
}

func TestUpsertFailureIncrementOccurrences(t *testing.T) {
	store := make(domain.FailureStore)
	art := domain.Artefact{
		ID: 42, Name: "ubuntu-server-amd64", OS: "ubuntu-server",
		Release: "noble", Version: "20260720",
	}

	isNew := store.UpsertFailure(art)
	if !isNew {
		t.Error("first upsert should return isNew=true")
	}

	// Same artefact, new version → increment occurrences, clear old analysis.
	art.Version = "20260721"
	// Pre-set an analysis so we can verify it gets cleared on version change.
	store["noble"]["ubuntu-server"][0].Analysis = &domain.LogAnalysis{Category: "infra"}
	store["noble"]["ubuntu-server"][0].AnalysedVersion = "20260720"

	isNew = store.UpsertFailure(art)
	if isNew {
		t.Error("second upsert should return isNew=false")
	}

	rec := store["noble"]["ubuntu-server"][0]
	if rec.Occurrences != 2 {
		t.Errorf("expected Occurrences=2, got %d", rec.Occurrences)
	}
	if rec.LastSeenVersion != "20260721" {
		t.Errorf("expected LastSeenVersion=20260721, got %s", rec.LastSeenVersion)
	}
	// Analysis should have been cleared because version changed.
	if rec.Analysis != nil {
		t.Error("expected Analysis to be cleared on version change")
	}
}

func TestUpsertFailureSameVersionKeepsAnalysis(t *testing.T) {
	store := make(domain.FailureStore)
	art := domain.Artefact{
		ID: 7, Name: "ubuntu-base-arm64", OS: "ubuntu-base",
		Release: "plucky", Version: "20260720",
	}
	store.UpsertFailure(art)

	analysis := domain.LogAnalysis{Category: "code", Hypothesis: "missing dep"}
	store.SetAnalysis(7, "plucky", "ubuntu-base", analysis, "20260720", "", "")

	// Upsert again with same version — analysis must be preserved.
	store.UpsertFailure(art)
	rec := store["plucky"]["ubuntu-base"][0]
	if rec.Analysis == nil {
		t.Fatal("expected analysis to be preserved when version unchanged")
	}
	if rec.Analysis.Category != "code" {
		t.Errorf("unexpected category: %s", rec.Analysis.Category)
	}
}

func TestResolveFailure(t *testing.T) {
	store := make(domain.FailureStore)
	store.UpsertFailure(domain.Artefact{
		ID: 5, Name: "ubuntu-desktop-amd64", OS: "ubuntu-desktop",
		Release: "noble", Version: "20260720",
	})

	store.ResolveFailure(5, "noble", "ubuntu-desktop")

	rec := store["noble"]["ubuntu-desktop"][0]
	if !rec.Resolved {
		t.Error("expected Resolved=true after ResolveFailure")
	}
}

func TestActiveFailures(t *testing.T) {
	store := make(domain.FailureStore)
	store.UpsertFailure(domain.Artefact{ID: 1, OS: "ubuntu-desktop", Release: "noble", Version: "20260720"})
	store.UpsertFailure(domain.Artefact{ID: 2, OS: "ubuntu-server", Release: "noble", Version: "20260720"})
	store.UpsertFailure(domain.Artefact{ID: 3, OS: "ubuntu-desktop", Release: "jammy", Version: "20260720"})

	store.ResolveFailure(2, "noble", "ubuntu-server")

	// All active failures.
	all := store.ActiveFailures("", "")
	if len(all) != 2 {
		t.Errorf("expected 2 active failures, got %d", len(all))
	}

	// Filter by release.
	noble := store.ActiveFailures("noble", "")
	if len(noble) != 1 || noble[0].ArtefactID != 1 {
		t.Errorf("expected 1 noble failure, got %v", noble)
	}
}

func TestPendingAnalysis(t *testing.T) {
	store := make(domain.FailureStore)
	for i := 1; i <= 5; i++ {
		store.UpsertFailure(domain.Artefact{
			ID: i, OS: "ubuntu-desktop", Release: "noble",
			Name:    "art",
			Version: "20260720",
		})
	}

	// Set analysis on record 3.
	now := time.Now().UTC()
	store["noble"]["ubuntu-desktop"][2].Analysis = &domain.LogAnalysis{Category: "infra"}
	store["noble"]["ubuntu-desktop"][2].AnalysedAt = &now

	pending := store.PendingAnalysis(3)
	if len(pending) != 3 {
		t.Errorf("expected 3 pending (cap), got %d", len(pending))
	}

	all := store.PendingAnalysis(0)
	if len(all) != 4 { // 5 records - 1 analysed
		t.Errorf("expected 4 pending (uncapped), got %d", len(all))
	}
}
