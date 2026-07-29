package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/mclemenceau/watchtower/internal/domain"
)

// Snapshot persists []domain.Artefact to a JSON file with atomic writes.
type Snapshot struct {
	path string
}

func New(path string) *Snapshot {
	return &Snapshot{path: path}
}

// Read returns the persisted artefact list. Returns nil, nil when no snapshot exists yet.
func (s *Snapshot) Read() ([]domain.Artefact, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var artefacts []domain.Artefact
	if err := json.Unmarshal(data, &artefacts); err != nil {
		return nil, err
	}
	return artefacts, nil
}

// Write persists artefacts atomically: write to a temp file then rename.
func (s *Snapshot) Write(artefacts []domain.Artefact) error {
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
//
// Failure detection uses two independent signals:
//
//  1. Test Observer status transitions (MARKED_AS_FAILED / recovery from it).
//     This is driven by human review decisions in Test Observer.
//
//  2. BuildLog transitions: NOT_STARTED/IN_PROGRESS/BUILT/UNKNOWN → FAILED fires
//     a NewFailure; FAILED → BUILT fires a Recovery. This is driven by the
//     cd-build-log enrichment and is the primary signal in practice, since most
//     artefacts are never explicitly MARKED_AS_FAILED in Test Observer.
//     Guard: prev.BuildLog must be non-empty (i.e. the prior snapshot was enriched)
//     to avoid first-boot noise.
//
// The two signals are independent — both are checked for every artefact, and each
// appends to NewFailures/Recoveries independently. An artefact can appear in both
// buckets only if it has contradictory Status and BuildLog signals, which should
// not happen in normal operation.
//
// NewBuilds is populated when a known artefact's BuildLog transitions to BUILT.
//
// Artefacts with no prior snapshot entry land in NewArtefacts.
func Diff(old, fresh []domain.Artefact) domain.ChangeReport {
	oldByID := make(map[int]domain.Artefact, len(old))
	for _, a := range old {
		oldByID[a.ID] = a
	}

	var report domain.ChangeReport

	for _, a := range fresh {
		prev, existed := oldByID[a.ID]
		if !existed {
			report.NewArtefacts = append(report.NewArtefacts, a)
			continue
		}

		// NewBuilds: BuildLog transition to BUILT (requires known prior version).
		if prev.Version != "" &&
			a.BuildLog == domain.BuildStatusBuilt &&
			prev.BuildLog != domain.BuildStatusBuilt {
			report.NewBuilds = append(report.NewBuilds, a)
		}

		// Signal 1: Test Observer status-based transitions.
		if prev.Status != a.Status {
			delta := domain.ArtefactDelta{
				ArtefactID: a.ID,
				Name:       a.Name,
				Release:    a.Release,
				Version:    a.Version,
				OldStatus:  prev.Status,
				NewStatus:  a.Status,
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

		// Signal 2: BuildLog-based failure/recovery transitions.
		// Only fires when the prior snapshot had an enriched BuildLog value —
		// prevents first-boot noise when a stale snapshot has empty BuildLog fields.
		if prev.BuildLog == "" {
			continue
		}
		if prev.BuildLog != domain.BuildStatusFailed &&
			a.BuildLog == domain.BuildStatusFailed {
			report.NewFailures = append(report.NewFailures, domain.ArtefactDelta{
				ArtefactID: a.ID,
				Name:       a.Name,
				Release:    a.Release,
				Version:    a.Version,
				OldStatus:  string(prev.BuildLog),
				NewStatus:  string(a.BuildLog),
			})
		}
		if prev.BuildLog == domain.BuildStatusFailed &&
			a.BuildLog == domain.BuildStatusBuilt {
			report.Recoveries = append(report.Recoveries, domain.ArtefactDelta{
				ArtefactID: a.ID,
				Name:       a.Name,
				Release:    a.Release,
				Version:    a.Version,
				OldStatus:  string(prev.BuildLog),
				NewStatus:  string(a.BuildLog),
			})
		}
	}

	return report
}

// LatestRelease returns the release name with the most recent build activity.
// Version strings may be YYYYMMDD or YYYYMMDD.N (re-spin suffix).
// Primary sort: base date (first 8 chars). Tiebreaker: artefact count (more = more active).
func LatestRelease(artefacts []domain.Artefact) string {
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
