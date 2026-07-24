package activities

import (
	"context"
	"testing"

	"github.com/mclemenceau/watchtower/internal/domain"
)

// inMemoryFailureStore is a minimal ports.FailureStorePort backed by an in-memory
// FailureStore. Used only in tests to avoid touching the filesystem.
type inMemoryFailureStore struct {
	store domain.FailureStore
}

func (m *inMemoryFailureStore) ReadFailures() (domain.FailureStore, error) {
	if m.store == nil {
		m.store = make(domain.FailureStore)
	}
	return m.store, nil
}

func (m *inMemoryFailureStore) WriteFailures(store domain.FailureStore) error {
	m.store = store
	return nil
}

// TestUpdateFailureRecords_NewFailures verifies that artefacts arriving via the
// NewFailures bucket of a ChangeReport are upserted into the failure store.
func TestUpdateFailureRecords_NewFailures(t *testing.T) {
	fs := &inMemoryFailureStore{}
	act := &Activities{Failures: fs}

	art := domain.Artefact{
		ID: 1, Name: "noble-desktop-amd64.iso",
		OS: "ubuntu", Release: "noble", Version: "20260720",
		Status: "MARKED_AS_FAILED",
	}
	report := domain.ChangeReport{
		NewFailures: []domain.ArtefactDelta{
			{Name: art.Name, Release: art.Release, Version: art.Version,
				OldStatus: "UNDECIDED", NewStatus: "MARKED_AS_FAILED"},
		},
	}

	if err := act.UpdateFailureRecords(context.Background(), report, []domain.Artefact{art}); err != nil {
		t.Fatalf("UpdateFailureRecords: %v", err)
	}

	active := fs.store.ActiveFailures("noble", "ubuntu")
	if len(active) != 1 {
		t.Fatalf("expected 1 active failure, got %d", len(active))
	}
	if active[0].ArtefactID != 1 {
		t.Errorf("unexpected artefact ID: %d", active[0].ArtefactID)
	}
}

// TestUpdateFailureRecords_NewArtefactAlreadyFailed verifies that a brand-new
// artefact (no previous snapshot entry) that is already MARKED_AS_FAILED is
// seeded into the failure store via the NewArtefacts path. This is the
// first-boot scenario: Diff produces NewArtefacts, not NewFailures, because
// there is no prior status to transition from.
func TestUpdateFailureRecords_NewArtefactAlreadyFailed(t *testing.T) {
	fs := &inMemoryFailureStore{}
	act := &Activities{Failures: fs}

	art := domain.Artefact{
		ID: 42, Name: "noble-server-amd64.iso",
		OS: "ubuntu-server", Release: "noble", Version: "20260721",
		Status: "MARKED_AS_FAILED",
	}
	// Diff on first boot produces NewArtefacts (no old snapshot), not NewFailures.
	report := domain.ChangeReport{
		NewArtefacts: []domain.Artefact{art},
	}

	if err := act.UpdateFailureRecords(context.Background(), report, []domain.Artefact{art}); err != nil {
		t.Fatalf("UpdateFailureRecords: %v", err)
	}

	active := fs.store.ActiveFailures("noble", "ubuntu-server")
	if len(active) != 1 {
		t.Fatalf("expected 1 active failure seeded from NewArtefacts, got %d", len(active))
	}
	if active[0].ArtefactName != "noble-server-amd64.iso" {
		t.Errorf("unexpected artefact name: %s", active[0].ArtefactName)
	}
}

// TestUpdateFailureRecords_NewArtefactNotFailed verifies that a brand-new
// artefact that is NOT MARKED_AS_FAILED is not added to the failure store.
func TestUpdateFailureRecords_NewArtefactNotFailed(t *testing.T) {
	fs := &inMemoryFailureStore{}
	act := &Activities{Failures: fs}

	art := domain.Artefact{
		ID: 99, Name: "noble-minimal-amd64.iso",
		OS: "ubuntu-minimal", Release: "noble", Version: "20260722",
		Status: "UNDECIDED",
	}
	report := domain.ChangeReport{
		NewArtefacts: []domain.Artefact{art},
	}

	if err := act.UpdateFailureRecords(context.Background(), report, []domain.Artefact{art}); err != nil {
		t.Fatalf("UpdateFailureRecords: %v", err)
	}

	active := fs.store.ActiveFailures("noble", "ubuntu-minimal")
	if len(active) != 0 {
		t.Errorf("expected no failures for non-failed new artefact, got %d", len(active))
	}
}
