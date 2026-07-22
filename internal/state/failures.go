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
	return store, nil
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
