// Package delta compares a parsed document against a snapshot.
package delta

import (
	"fmt"
	"sort"

	"oa-engine/internal/org"
	"oa-engine/internal/snapshot"
)

// Result classifies node keys found in the document versus the snapshot.
type Result struct {
	Added     []string // in doc, not in snapshot (pre-order)
	Modified  []string // in both, content hash differs (pre-order)
	Deleted   []string // in snapshot, not in doc (sorted)
	Unchanged int      // in both, content hash matches
}

// Compute diffs doc against snap.
func Compute(doc *org.Document, snap *snapshot.Snapshot) Result {
	r := Result{Added: []string{}, Modified: []string{}, Deleted: []string{}}
	inSnap := func(key string) (snapshot.Entry, bool) {
		e, ok := snap.Nodes[key]
		return e, ok
	}
	var walk func(ns []*org.Node)
	walk = func(ns []*org.Node) {
		for _, n := range ns {
			if e, ok := inSnap(n.Key); !ok {
				r.Added = append(r.Added, n.Key)
			} else if e.ContentHash == n.ContentHash {
				r.Unchanged++
			} else {
				r.Modified = append(r.Modified, n.Key)
			}
			walk(n.Children)
		}
	}
	walk(doc.Roots)

	inDoc := make(map[string]bool, len(doc.All))
	for k := range doc.All {
		inDoc[k] = true
	}
	for k := range snap.Nodes {
		if !inDoc[k] {
			r.Deleted = append(r.Deleted, k)
		}
	}
	sort.Strings(r.Deleted)
	return r
}

// ResolveFocus resolves exactly one of focusKey / focusLine to a node.
func ResolveFocus(doc *org.Document, focusKey string, focusLine int) (*org.Node, error) {
	if (focusKey != "") == (focusLine > 0) {
		return nil, fmt.Errorf("delta: exactly one of focus key or focus line must be set")
	}
	if focusKey != "" {
		n, ok := doc.All[focusKey]
		if !ok {
			return nil, fmt.Errorf("delta: unknown focus key %q", focusKey)
		}
		return n, nil
	}
	var best *org.Node
	for _, n := range doc.All {
		if n.Line <= focusLine && (best == nil || n.Line > best.Line) {
			best = n
		}
	}
	if best == nil {
		return nil, fmt.Errorf("delta: no node at or before line %d", focusLine)
	}
	return best, nil
}
