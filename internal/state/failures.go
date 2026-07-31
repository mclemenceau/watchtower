package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/ports"
)

// FailureState persists domain.FailureStore to a JSON file with atomic writes.
// It implements ports.FailureStorePort.
type FailureState struct {
	path string
}

// Compile-time interface check.
var _ ports.FailureStorePort = (*FailureState)(nil)

// NewFailureState creates a FailureState backed by the given file path.
func NewFailureState(path string) *FailureState {
	return &FailureState{path: path}
}

// ReadFailures returns the persisted FailureStore. Returns an empty (non-nil)
// store when the file does not exist yet (first boot).
//
// On every read a one-time backfill pass is performed: any INFRA record that
// has a non-empty FailureDescription but an empty FailureSignature receives
// the canonical signature from domain.InfraSignatureFor. If any records are
// patched the updated store is written back atomically so subsequent reads
// skip the pass entirely.
func (f *FailureState) ReadFailures() (domain.FailureStore, error) {
	data, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return make(domain.FailureStore), nil
	}
	if err != nil {
		return nil, err
	}
	var store domain.FailureStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	if store == nil {
		store = make(domain.FailureStore)
	}

	patched := backfillInfraSignatures(store)
	if patched {
		// Best-effort write-back; ignore errors so callers always get the
		// (in-memory patched) store even if the write fails.
		_ = f.WriteFailures(store)
	}

	return store, nil
}

// backfillInfraSignatures assigns FailureSignature to any INFRA record that
// has a non-empty FailureDescription but no signature yet. Returns true if at
// least one record was modified.
func backfillInfraSignatures(store domain.FailureStore) bool {
	patched := false
	for _, byProduct := range store {
		for product, records := range byProduct {
			for i := range records {
				r := &records[i]
				if r.FailureKind != domain.BuildFailureKindInfra {
					continue
				}
				if r.FailureSignature != "" || r.FailureDescription == "" {
					continue
				}
				if sig := domain.InfraSignatureFor(r.FailureDescription); sig != "" {
					r.FailureSignature = sig
					patched = true
				}
			}
			byProduct[product] = records
		}
	}
	return patched
}

// WriteFailures persists the FailureStore atomically: write to a temp file
// then rename to the target path.
func (f *FailureState) WriteFailures(store domain.FailureStore) error {
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}
