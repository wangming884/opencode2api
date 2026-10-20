package limits

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DayUsage is the persisted part of one key's counters: which UTC day it
// belongs to and what was spent during it. The rolling minute is deliberately
// not persisted, because a burst window that outlived a restart would describe
// traffic that is already over.
type DayUsage struct {
	Day      int64 `json:"day"`
	Requests int   `json:"requests"`
	Tokens   int64 `json:"tokens"`
}

// Store reads and writes the persisted counters. An empty path disables
// persistence, which is what a runtime built without a configuration file
// gets.
type Store struct {
	path string
}

// NewStore returns a store for path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

type limitsFile struct {
	Keys map[string]DayUsage `json:"keys"`
}

// Load reads the stored counters. A missing file is not an error: it is what a
// first start looks like.
func (s *Store) Load() (map[string]DayUsage, error) {
	if s == nil || s.path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var file limitsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("invalid limits state: %w", err)
	}
	return file.Keys, nil
}

// Save writes the counters through a temporary file and a rename, so a crash
// mid-write leaves the previous file intact. The file holds no secrets, only
// key fingerprints and counts, and is created owner-only like the rotation
// state.
func (s *Store) Save(usage map[string]DayUsage) error {
	if s == nil || s.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(limitsFile{Keys: usage}, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(s.path), ".limits-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err = temp.Write(data); err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Chmod(temp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(temp.Name(), s.path)
}
