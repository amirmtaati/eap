package ctxsel

import (
	"reflect"
	"testing"

	"oa-engine/internal/delta"
	"oa-engine/internal/org"
)

func parse(t *testing.T, text string) *org.Document {
	t.Helper()
	doc, err := org.Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return doc
}

func keysOf(ns []*org.Node) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.Key
	}
	return out
}

func emptyDelta() delta.Result {
	return delta.Result{Added: []string{}, Modified: []string{}, Deleted: []string{}}
}

const ctxDoc = `* Project :ctx:
** Background :noctx:
hidden
** Shared :ctx:
** Plain
* Agent
** Turn-1
** Turn-2
** Turn-3
** Turn-4
`

func TestSelectTagRules(t *testing.T) {
	doc := parse(t, ctxDoc)
	got := keysOf(Select(doc, emptyDelta(), nil, 0))
	want := []string{"project", "project/shared"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSelectParentsOfChanges(t *testing.T) {
	doc := parse(t, ctxDoc)
	d := emptyDelta()
	d.Added = []string{"agent/turn-1"}
	d.Modified = []string{"project/plain"}
	// parents: "agent" (level 1) and "project" — project already ctx-tagged.
	got := keysOf(Select(doc, d, nil, 0))
	want := []string{"project", "project/shared", "agent"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSelectFocusSubtree(t *testing.T) {
	doc := parse(t, ctxDoc)
	focus := doc.All["agent/turn-2"]
	got := keysOf(Select(doc, emptyDelta(), focus, 0))
	want := []string{"project", "project/shared", "agent/turn-2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSelectAgentHistoryLimit(t *testing.T) {
	doc := parse(t, ctxDoc)
	got := keysOf(Select(doc, emptyDelta(), nil, 2))
	want := []string{"project", "project/shared", "agent/turn-3", "agent/turn-4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSelectDedupPreorderDeterminism(t *testing.T) {
	doc := parse(t, ctxDoc)
	d := emptyDelta()
	d.Added = []string{"agent/turn-3"} // parent "agent" also focus subtree member
	got := keysOf(Select(doc, d, doc.All["agent/turn-2"], 5))
	want := []string{"project", "project/shared", "agent", "agent/turn-1", "agent/turn-2", "agent/turn-3", "agent/turn-4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// Determinism: identical inputs, identical output.
	again := keysOf(Select(doc, d, doc.All["agent/turn-2"], 5))
	if !reflect.DeepEqual(got, again) {
		t.Fatalf("nondeterministic: %v vs %v", got, again)
	}
}

func TestSelectNoAgentNode(t *testing.T) {
	doc := parse(t, "* Solo :ctx:\nbody\n")
	got := keysOf(Select(doc, emptyDelta(), nil, 3))
	want := []string{"solo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
