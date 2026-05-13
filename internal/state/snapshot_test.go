package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mclemenceau/argus/internal/buildapi"
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

	images := []buildapi.Image{
		{ID: "ubuntu-desktop-amd64", Status: "SUCCESS", StartedAt: time.Now().UTC()},
	}
	if err := s.Write(images); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err = s.Read()
	if err != nil {
		t.Fatalf("Read after write: %v", err)
	}
	if len(got) != 1 || got[0].ID != images[0].ID {
		t.Fatalf("roundtrip mismatch: %v", got)
	}
}

func TestWriteIsAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	s := New(path)

	if err := s.Write([]buildapi.Image{{ID: "a", Status: "SUCCESS"}}); err != nil {
		t.Fatal(err)
	}
	// Temp file must be gone after a successful write.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("tmp file should not exist after successful write")
	}
}

func TestDiffNewFailure(t *testing.T) {
	old := []buildapi.Image{{ID: "x", Status: "BUILDING"}}
	fresh := []buildapi.Image{{ID: "x", Status: "FAILED"}}

	report := Diff(old, fresh)

	if len(report.NewFailures) != 1 {
		t.Fatalf("expected 1 new failure, got %d", len(report.NewFailures))
	}
	if report.NewFailures[0].OldStatus != "BUILDING" || report.NewFailures[0].NewStatus != "FAILED" {
		t.Fatalf("unexpected delta: %+v", report.NewFailures[0])
	}
	if len(report.Recoveries)+len(report.OtherChanges)+len(report.NewImages) != 0 {
		t.Fatal("unexpected entries in other buckets")
	}
}

func TestDiffRecovery(t *testing.T) {
	old := []buildapi.Image{{ID: "x", Status: "FAILED"}}
	fresh := []buildapi.Image{{ID: "x", Status: "SUCCESS"}}

	report := Diff(old, fresh)

	if len(report.Recoveries) != 1 {
		t.Fatalf("expected 1 recovery, got %d", len(report.Recoveries))
	}
	if len(report.NewFailures)+len(report.OtherChanges)+len(report.NewImages) != 0 {
		t.Fatal("unexpected entries in other buckets")
	}
}

func TestDiffOtherChange(t *testing.T) {
	old := []buildapi.Image{{ID: "x", Status: "BUILDING"}}
	fresh := []buildapi.Image{{ID: "x", Status: "CANCELLED"}}

	report := Diff(old, fresh)

	if len(report.OtherChanges) != 1 {
		t.Fatalf("expected 1 other change, got %d", len(report.OtherChanges))
	}
}

func TestDiffNewImage(t *testing.T) {
	old := []buildapi.Image{}
	fresh := []buildapi.Image{{ID: "brand-new", Status: "BUILDING"}}

	report := Diff(old, fresh)

	if len(report.NewImages) != 1 {
		t.Fatalf("expected 1 new image, got %d", len(report.NewImages))
	}
	if report.NewImages[0].ID != "brand-new" {
		t.Fatalf("wrong image ID: %s", report.NewImages[0].ID)
	}
}

func TestDiffNoChange(t *testing.T) {
	images := []buildapi.Image{
		{ID: "a", Status: "SUCCESS"},
		{ID: "b", Status: "FAILED"},
	}
	report := Diff(images, images)

	if len(report.NewFailures)+len(report.Recoveries)+len(report.OtherChanges)+len(report.NewImages) != 0 {
		t.Fatalf("expected empty report, got %+v", report)
	}
}

func TestDiffMixed(t *testing.T) {
	old := []buildapi.Image{
		{ID: "a", Status: "SUCCESS"},
		{ID: "b", Status: "FAILED"},
		{ID: "c", Status: "BUILDING"},
	}
	fresh := []buildapi.Image{
		{ID: "a", Status: "FAILED"},   // new failure
		{ID: "b", Status: "SUCCESS"},  // recovery
		{ID: "c", Status: "BUILDING"}, // no change
		{ID: "d", Status: "BUILDING"}, // new image
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
	if len(report.NewImages) != 1 {
		t.Errorf("NewImages: want 1, got %d", len(report.NewImages))
	}
}
