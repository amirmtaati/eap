package snapshot

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSave(t *testing.T) {
	tests := []struct {
		name string
		path string // relative to t.TempDir(); "" = never written
		snap *Snapshot
	}{
		{
			name: "missing file yields empty schema-1 snapshot",
			path: "absent.json",
			snap: nil,
		},
		{
			name: "roundtrip",
			path: "snapshot.json",
			snap: &Snapshot{Schema: 1, Timestamp: "2026-10-14T08:30:00Z", Nodes: map[string]Entry{
				"roadmap/milestone-1": {ContentHash: "a1", SubtreeHash: "s1"},
				"legacy-task":         {ContentHash: "a2", SubtreeHash: "s2"},
			}},
		},
		{
			name: "roundtrip empty nodes",
			path: "empty.json",
			snap: &Snapshot{Schema: 1, Nodes: map[string]Entry{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tt.path)
			if tt.snap != nil {
				if err := tt.snap.Save(path); err != nil {
					t.Fatalf("Save: %v", err)
				}
			}
			got, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if tt.snap == nil {
				if got.Schema != 1 || len(got.Nodes) != 0 {
					t.Fatalf("missing file: got %+v, want empty schema-1", got)
				}
				return
			}
			if got.Schema != tt.snap.Schema || got.Timestamp != tt.snap.Timestamp {
				t.Fatalf("header: got %+v, want %+v", got, tt.snap)
			}
			if len(got.Nodes) != len(tt.snap.Nodes) {
				t.Fatalf("nodes count: got %d, want %d", len(got.Nodes), len(tt.snap.Nodes))
			}
			for k, want := range tt.snap.Nodes {
				if got.Nodes[k] != want {
					t.Fatalf("node %q: got %+v, want %+v", k, got.Nodes[k], want)
				}
			}
		})
	}
}

func TestSaveTimestampSetAndTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	s := &Snapshot{Schema: 1, Nodes: map[string]Entry{}}
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if s.Timestamp == "" {
		t.Fatal("Timestamp not set on save")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if data[len(data)-1] != '\n' {
		t.Fatal("missing trailing newline")
	}
}

func TestSaveAtomicNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot.json")
	s := &Snapshot{Schema: 1, Timestamp: "2026-10-14T08:30:00Z", Nodes: map[string]Entry{
		"k": {ContentHash: "a", SubtreeHash: "b"},
	}}
	for i := 0; i < 3; i++ {
		if err := s.Save(path); err != nil {
			t.Fatalf("Save #%d: %v", i, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "snapshot.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("temp files left behind: %v", names)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr error
	}{
		{name: "schema 2 unsupported", content: `{"schema":2,"timestamp":"","nodes":{}}`, wantErr: ErrUnsupportedSchema},
		{name: "malformed json", content: `{"schema":1,`, wantErr: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "snapshot.json")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			_, err := Load(path)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v, want %v", err, tt.wantErr)
			}
		})
	}
}
