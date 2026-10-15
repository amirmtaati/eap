// Package snapshot persists node hash snapshots (.oa/snapshot.json).
package snapshot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Entry holds the two hashes recorded for one node key.
type Entry struct {
	ContentHash string `json:"content_hash"`
	SubtreeHash string `json:"subtree_hash"`
}

// Snapshot is the on-disk document state.
type Snapshot struct {
	Schema    int              `json:"schema"`
	Timestamp string           `json:"timestamp"`
	Nodes     map[string]Entry `json:"nodes"`
}

// ErrUnsupportedSchema reports a snapshot written by an incompatible version.
var ErrUnsupportedSchema = errors.New("unsupported snapshot schema")

// Load reads path. A missing file yields an empty schema-1 snapshot.
func Load(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Snapshot{Schema: 1, Nodes: map[string]Entry{}}, nil
		}
		return nil, fmt.Errorf("snapshot: read %s: %w", path, err)
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("snapshot: decode %s: %w", path, err)
	}
	if s.Schema != 1 {
		return nil, fmt.Errorf("snapshot: %s: schema %d: %w", path, s.Schema, ErrUnsupportedSchema)
	}
	if s.Nodes == nil {
		s.Nodes = map[string]Entry{}
	}
	return &s, nil
}

// Save atomically writes s to path (temp file in the same directory, then
// rename) with 0644 permissions and a trailing newline.
func (s *Snapshot) Save(path string) error {
	if s.Timestamp == "" {
		s.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("snapshot: encode: %w", err)
	}
	data = append(data, '\n')

	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	tmp, err := os.CreateTemp(dir, base+".tmp-*")
	if err != nil {
		return fmt.Errorf("snapshot: write %s: %w", path, err)
	}
	defer os.Remove(tmp.Name()) // no-op after successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("snapshot: write %s: %w", path, err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return fmt.Errorf("snapshot: write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("snapshot: write %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("snapshot: write %s: %w", path, err)
	}
	return nil
}
