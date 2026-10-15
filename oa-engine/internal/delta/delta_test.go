package delta

import (
	"reflect"
	"testing"

	"oa-engine/internal/org"
	"oa-engine/internal/snapshot"
)

const docText = `* Root
** Alpha
body alpha
** Beta :ctx:
body beta
*** Beta-Child
changed body
** Gamma
`

func parseDoc(t *testing.T) *org.Document {
	t.Helper()
	doc, err := org.Parse(docText)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return doc
}

func TestCompute(t *testing.T) {
	doc := parseDoc(t)
	// Real hashes for unchanged entries; stale hashes for modified ones.
	snap := &snapshot.Snapshot{Schema: 1, Nodes: map[string]snapshot.Entry{
		"root":                 {ContentHash: doc.All["root"].ContentHash},
		"root/alpha":           {ContentHash: doc.All["root/alpha"].ContentHash},
		"root/beta":            {ContentHash: "stale-beta"},
		"root/beta/beta-child": {ContentHash: "stale-beta-child"},
		"gone":                 {ContentHash: "x"},
		"also-gone":            {ContentHash: "y"},
	}}
	got := Compute(doc, snap)
	want := Result{
		Added:     []string{"root/gamma"},
		Modified:  []string{"root/beta", "root/beta/beta-child"},
		Deleted:   []string{"also-gone", "gone"},
		Unchanged: 2,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestComputeEmptySnapshot(t *testing.T) {
	doc := parseDoc(t)
	got := Compute(doc, &snapshot.Snapshot{Schema: 1, Nodes: map[string]snapshot.Entry{}})
	if len(got.Added) != 5 || len(got.Modified) != 0 || len(got.Deleted) != 0 || got.Unchanged != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveFocus(t *testing.T) {
	doc := parseDoc(t)
	tests := []struct {
		name      string
		key       string
		line      int
		wantKey   string
		wantError bool
	}{
		{name: "by key", key: "root/beta/beta-child", wantKey: "root/beta/beta-child"},
		{name: "by key missing", key: "nope", wantError: true},
		{name: "by line exact", line: doc.All["root/beta"].Line, wantKey: "root/beta"},
		{name: "by line inside body", line: doc.All["root/beta"].Line + 1, wantKey: "root/beta"},
		{name: "by line before any node", line: 0, wantError: true},
		{name: "both set", key: "root/beta", line: 3, wantError: true},
		{name: "neither set", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := ResolveFocus(doc, tt.key, tt.line)
			if tt.wantError {
				if err == nil {
					t.Fatalf("want error, got node %v", n)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveFocus: %v", err)
			}
			if n.Key != tt.wantKey {
				t.Fatalf("got %q, want %q", n.Key, tt.wantKey)
			}
		})
	}
}
