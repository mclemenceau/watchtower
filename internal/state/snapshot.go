package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/mclemenceau/argus/internal/buildapi"
)

// Snapshot persists []Image to a JSON file with atomic writes.
type Snapshot struct {
	path string
}

func New(path string) *Snapshot {
	return &Snapshot{path: path}
}

// Read returns the persisted image list. Returns nil, nil when no snapshot exists yet.
func (s *Snapshot) Read() ([]buildapi.Image, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var images []buildapi.Image
	if err := json.Unmarshal(data, &images); err != nil {
		return nil, err
	}
	return images, nil
}

// Write persists images atomically: write to a temp file then rename.
func (s *Snapshot) Write(images []buildapi.Image) error {
	data, err := json.MarshalIndent(images, "", "  ")
	if err != nil {
		return err
	}

	tmp := s.path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Diff compares an old snapshot against a fresh fetch and categorises every change.
func Diff(old, fresh []buildapi.Image) buildapi.ChangeReport {
	oldByID := make(map[string]buildapi.Image, len(old))
	for _, img := range old {
		oldByID[img.ID] = img
	}

	var report buildapi.ChangeReport

	for _, img := range fresh {
		prev, existed := oldByID[img.ID]
		if !existed {
			report.NewImages = append(report.NewImages, img)
			continue
		}
		if prev.Status == img.Status {
			continue
		}
		delta := buildapi.ImageDelta{
			Image:     img.ID,
			OldStatus: prev.Status,
			NewStatus: img.Status,
			Since:     img.FinishedAt,
		}
		switch {
		case img.Status == "FAILED":
			report.NewFailures = append(report.NewFailures, delta)
		case prev.Status == "FAILED":
			report.Recoveries = append(report.Recoveries, delta)
		default:
			report.OtherChanges = append(report.OtherChanges, delta)
		}
	}

	return report
}
