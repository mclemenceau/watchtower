package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mclemenceau/watchtower/internal/buildapi"
)

func TestReadWriteRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	s := New(path)

	// No file yet — should return nil without error.
	got, err := s.Read()
	if err != nil {
		t.Fatalf("Read on missing file: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}

	artefacts := []buildapi.Artefact{
		{ID: 1001, Name: "noble-desktop-amd64.iso", Release: "noble", Version: "20260402", Status: "APPROVED"},
	}
	if err := s.Write(artefacts); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err = s.Read()
	if err != nil {
		t.Fatalf("Read after write: %v", err)
	}
	if len(got) != 1 || got[0].ID != artefacts[0].ID {
		t.Fatalf("roundtrip mismatch: %v", got)
	}
}

func TestWriteIsAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	s := New(path)

	if err := s.Write([]buildapi.Artefact{{ID: 1, Name: "a", Status: "APPROVED"}}); err != nil {
		t.Fatal(err)
	}
	// Temp file must be gone after a successful write.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("tmp file should not exist after successful write")
	}
}

func TestDiffNewFailure(t *testing.T) {
	old := []buildapi.Artefact{{ID: 1, Name: "x", Status: "UNDECIDED"}}
	fresh := []buildapi.Artefact{{ID: 1, Name: "x", Status: "MARKED_AS_FAILED"}}

	report := Diff(old, fresh)

	if len(report.NewFailures) != 1 {
		t.Fatalf("expected 1 new failure, got %d", len(report.NewFailures))
	}
	if report.NewFailures[0].OldStatus != "UNDECIDED" || report.NewFailures[0].NewStatus != "MARKED_AS_FAILED" {
		t.Fatalf("unexpected delta: %+v", report.NewFailures[0])
	}
	if len(report.Recoveries)+len(report.OtherChanges)+len(report.NewArtefacts) != 0 {
		t.Fatal("unexpected entries in other buckets")
	}
}

func TestDiffRecovery(t *testing.T) {
	old := []buildapi.Artefact{{ID: 1, Name: "x", Status: "MARKED_AS_FAILED"}}
	fresh := []buildapi.Artefact{{ID: 1, Name: "x", Status: "APPROVED"}}

	report := Diff(old, fresh)

	if len(report.Recoveries) != 1 {
		t.Fatalf("expected 1 recovery, got %d", len(report.Recoveries))
	}
	if len(report.NewFailures)+len(report.OtherChanges)+len(report.NewArtefacts) != 0 {
		t.Fatal("unexpected entries in other buckets")
	}
}

func TestDiffOtherChange(t *testing.T) {
	old := []buildapi.Artefact{{ID: 1, Name: "x", Status: "UNDECIDED"}}
	fresh := []buildapi.Artefact{{ID: 1, Name: "x", Status: "APPROVED"}}

	report := Diff(old, fresh)

	if len(report.OtherChanges) != 1 {
		t.Fatalf("expected 1 other change, got %d", len(report.OtherChanges))
	}
}

func TestDiffNewArtefact(t *testing.T) {
	old := []buildapi.Artefact{}
	fresh := []buildapi.Artefact{{ID: 999, Name: "brand-new", Status: "UNDECIDED"}}

	report := Diff(old, fresh)

	if len(report.NewArtefacts) != 1 {
		t.Fatalf("expected 1 new artefact, got %d", len(report.NewArtefacts))
	}
	if report.NewArtefacts[0].ID != 999 {
		t.Fatalf("wrong artefact ID: %d", report.NewArtefacts[0].ID)
	}
}

func TestDiffNoChange(t *testing.T) {
	artefacts := []buildapi.Artefact{
		{ID: 1, Name: "a", Status: "APPROVED"},
		{ID: 2, Name: "b", Status: "MARKED_AS_FAILED"},
	}
	report := Diff(artefacts, artefacts)

	if len(report.NewFailures)+len(report.Recoveries)+len(report.OtherChanges)+len(report.NewArtefacts) != 0 {
		t.Fatalf("expected empty report, got %+v", report)
	}
}

func TestDiffMixed(t *testing.T) {
	old := []buildapi.Artefact{
		{ID: 1, Name: "a", Status: "APPROVED"},
		{ID: 2, Name: "b", Status: "MARKED_AS_FAILED"},
		{ID: 3, Name: "c", Status: "UNDECIDED"},
	}
	fresh := []buildapi.Artefact{
		{ID: 1, Name: "a", Status: "MARKED_AS_FAILED"}, // new failure
		{ID: 2, Name: "b", Status: "APPROVED"},         // recovery
		{ID: 3, Name: "c", Status: "UNDECIDED"},        // no change
		{ID: 4, Name: "d", Status: "UNDECIDED"},        // new artefact
	}

	report := Diff(old, fresh)

	if len(report.NewFailures) != 1 {
		t.Errorf("NewFailures: want 1, got %d", len(report.NewFailures))
	}
	if len(report.Recoveries) != 1 {
		t.Errorf("Recoveries: want 1, got %d", len(report.Recoveries))
	}
	if len(report.OtherChanges) != 0 {
		t.Errorf("OtherChanges: want 0, got %d", len(report.OtherChanges))
	}
	if len(report.NewArtefacts) != 1 {
		t.Errorf("NewArtefacts: want 1, got %d", len(report.NewArtefacts))
	}
}

func TestLatestRelease(t *testing.T) {
	artefacts := []buildapi.Artefact{
		{Release: "noble", Version: "20260402"},
		{Release: "noble", Version: "20260401"},
		{Release: "plucky", Version: "20260513"},
		{Release: "bionic", Version: "20260301"},
	}
	got := LatestRelease(artefacts)
	if got != "plucky" {
		t.Fatalf("expected plucky, got %s", got)
	}
}

func TestLatestReleaseTiebreaker(t *testing.T) {
	// Same date: release with more artefacts wins (more active).
	artefacts := []buildapi.Artefact{
		{Release: "noble", Version: "20260513"},
		{Release: "noble", Version: "20260513"},
		{Release: "noble", Version: "20260513"},
		{Release: "22", Version: "20260513.1"}, // dot-suffix: base date still 20260513
	}
	got := LatestRelease(artefacts)
	if got != "noble" {
		t.Fatalf("expected noble (more artefacts on same base date), got %s", got)
	}
}

func TestLatestReleaseEmpty(t *testing.T) {
	got := LatestRelease(nil)
	if got != "" {
		t.Fatalf("expected empty string, got %s", got)
	}
}
