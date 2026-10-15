package prompt

import (
	"strings"
	"testing"

	"oa-engine/internal/delta"
	"oa-engine/internal/org"
)

const wantHeader = "You are assisting with an org-mode project document. Respond in org-mode syntax. Use headlines at level ** or deeper. Plain text only, no rich text."

func parse(t *testing.T, text string) *org.Document {
	t.Helper()
	doc, err := org.Parse(text)
	if err != nil {
		t.Fatalf("org.Parse: %v", err)
	}
	return doc
}

func TestBuildExactLayoutWithFocus(t *testing.T) {
	doc := parse(t, "* Project\n** Task One :ctx:\ndetails here\n** Task Two\n")
	d := delta.Result{
		Added:    []string{"project/task-one"},
		Modified: []string{"project/task-two"},
		Deleted:  []string{"legacy"},
	}
	ctx := []*org.Node{doc.All["project"]}
	got := Build(doc, d, ctx, doc.All["project/task-two"])
	want := `You are assisting with an org-mode project document. Respond in org-mode syntax. Use headlines at level ** or deeper. Plain text only, no rich text.

# Context

* Project

# Changes

Added: project/task-one
Modified:
  ** Task Two
Deleted: legacy

# Request

Focus on the following subtree:

** Task Two
`
	if got != want {
		t.Errorf("Build mismatch got=%q want=%q", got, want)
	}
}

func TestBuildNoFocus(t *testing.T) {
	doc := parse(t, "* Project\n** Task One :ctx:\ndetails here\n** Task Two\n")
	d := delta.Result{
		Added:    []string{"project/task-one"},
		Modified: []string{"project/task-two"},
		Deleted:  []string{"legacy"},
	}
	got := Build(doc, d, []*org.Node{doc.All["project"]}, nil)
	if !strings.HasSuffix(got, "\n# Request\n\nUpdate the changed nodes.\n") {
		t.Errorf("missing no-focus request, got:\n%s", got)
	}
	if strings.Contains(got, "Focus on the following subtree") {
		t.Errorf("unexpected focus request, got:\n%s", got)
	}
}

func TestBuildEmptyContext(t *testing.T) {
	doc := parse(t, "* Project\n** Task One :ctx:\ndetails here\n** Task Two\n")
	d := delta.Result{Added: []string{"project/task-one"}}
	got := Build(doc, d, nil, nil)
	want := wantHeader + "\n\n# Context\n\n# Changes\n\n" +
		"Added: project/task-one\nModified:\nDeleted: none\n\n" +
		"# Request\n\nUpdate the changed nodes.\n"
	if got != want {
		t.Errorf("Build mismatch got=%q want=%q", got, want)
	}
}

func TestBuildMultiLineBodiesAndContextSeparators(t *testing.T) {
	doc := parse(t, "* A\nbody a\n* B\nbody b\n")
	d := delta.Result{Modified: []string{"a"}}
	ctx := []*org.Node{doc.All["a"], doc.All["b"]}
	got := Build(doc, d, ctx, nil)
	want := wantHeader + "\n\n# Context\n\n* A\nbody a\n\n* B\nbody b\n\n" +
		"# Changes\n\nAdded: none\nModified:\n  * A\n  body a\nDeleted: none\n\n" +
		"# Request\n\nUpdate the changed nodes.\n"
	if got != want {
		t.Errorf("Build mismatch got=%q want=%q", got, want)
	}
}

func TestBuildDeterministic(t *testing.T) {
	doc := parse(t, "* Project\n** Task One :ctx:\ndetails here\n** Task Two\n")
	d := delta.Result{
		Added:    []string{"project/task-one"},
		Modified: []string{"project/task-two"},
		Deleted:  []string{"legacy"},
	}
	ctx := []*org.Node{doc.All["project"]}
	a := Build(doc, d, ctx, doc.All["project/task-two"])
	b := Build(doc, d, ctx, doc.All["project/task-two"])
	if a != b {
		t.Error("Build is not deterministic")
	}
}
