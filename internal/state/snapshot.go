package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/mclemenceau/watchtower/internal/buildapi"
)

// Snapshot persists []Artefact to a JSON file with atomic writes.
type Snapshot struct {
	path string
}

func New(path string) *Snapshot {
	return &Snapshot{path: path}
}

// Read returns the persisted artefact list. Returns nil, nil when no snapshot exists yet.
func (s *Snapshot) Read() ([]buildapi.Artefact, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var artefacts []buildapi.Artefact
	if err := json.Unmarshal(data, &artefacts); err != nil {
		return nil, err
	}
	return artefacts, nil
}

// Write persists artefacts atomically: write to a temp file then rename.
func (s *Snapshot) Write(artefacts []buildapi.Artefact) error {
	data, err := json.MarshalIndent(artefacts, "", "  ")
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
// Status vocabulary: APPROVED | MARKED_AS_FAILED | UNDECIDED
// MARKED_AS_FAILED is treated as the failure state for alerting purposes.
func Diff(old, fresh []buildapi.Artefact) buildapi.ChangeReport {
	oldByID := make(map[int]buildapi.Artefact, len(old))
	for _, a := range old {
		oldByID[a.ID] = a
	}

	var report buildapi.ChangeReport

	for _, a := range fresh {
		prev, existed := oldByID[a.ID]
		if !existed {
			report.NewArtefacts = append(report.NewArtefacts, a)
			continue
		}
		if prev.Status == a.Status {
			continue
		}
		delta := buildapi.ArtefactDelta{
			Name:      a.Name,
			Release:   a.Release,
			Version:   a.Version,
			OldStatus: prev.Status,
			NewStatus: a.Status,
		}
		switch {
		case a.Status == "MARKED_AS_FAILED":
			report.NewFailures = append(report.NewFailures, delta)
		case prev.Status == "MARKED_AS_FAILED":
			report.Recoveries = append(report.Recoveries, delta)
		default:
			report.OtherChanges = append(report.OtherChanges, delta)
		}
	}

	return report
}

// LatestRelease returns the release name with the most recent build activity.
// Version strings may be YYYYMMDD or YYYYMMDD.N (re-spin suffix).
// Primary sort: base date (first 8 chars). Tiebreaker: artefact count (more = more active).
func LatestRelease(artefacts []buildapi.Artefact) string {
	type releaseStats struct {
		baseDate string
		count    int
	}
	stats := make(map[string]*releaseStats)

	for _, a := range artefacts {
		base := a.Version
		if len(base) > 8 {
			base = base[:8]
		}
		s, ok := stats[a.Release]
		if !ok {
			stats[a.Release] = &releaseStats{baseDate: base, count: 1}
			continue
		}
		s.count++
		if base > s.baseDate {
			s.baseDate = base
		}
	}

	var bestRelease string
	var bestDate string
	var bestCount int
	for release, s := range stats {
		if s.baseDate > bestDate || (s.baseDate == bestDate && s.count > bestCount) {
			bestDate = s.baseDate
			bestCount = s.count
			bestRelease = release
		}
	}
	return bestRelease
}
