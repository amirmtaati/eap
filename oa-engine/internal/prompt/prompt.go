// Package prompt assembles the plain-text LLM prompt from context, delta
// and focus nodes. Output is deterministic: sections in fixed order, keys
// in delta order, no map iteration.
package prompt

import (
	"strings"

	"oa-engine/internal/delta"
	"oa-engine/internal/org"
)

const header = "You are assisting with an org-mode project document. Respond in org-mode syntax. Use headlines at level ** or deeper. Plain text only, no rich text."

// Build renders the prompt: instruction header, context node texts,
// the delta summary, and the request (focused subtree or generic update).
func Build(doc *org.Document, d delta.Result, ctx []*org.Node, focus *org.Node) string {
	lines := []string{header}

	lines = append(lines, "", "# Context")
	for i, n := range ctx {
		if i == 0 {
			lines = append(lines, "")
		} else {
			lines = append(lines, "") // blank line between context nodes
		}
		lines = append(lines, strings.Split(n.Text(), "\n")...)
	}

	lines = append(lines, "", "# Changes", "",
		"Added: "+keysOrNone(d.Added),
		"Modified:")
	for _, k := range d.Modified {
		if n := doc.All[k]; n != nil {
			for _, l := range strings.Split(n.Text(), "\n") {
				lines = append(lines, "  "+l)
			}
		}
	}
	lines = append(lines, "Deleted: "+keysOrNone(d.Deleted))

	lines = append(lines, "", "# Request", "")
	if focus != nil {
		lines = append(lines, "Focus on the following subtree:", "")
		lines = append(lines, strings.Split(focus.Text(), "\n")...)
	} else {
		lines = append(lines, "Update the changed nodes.")
	}
	return strings.Join(lines, "\n") + "\n"
}

func keysOrNone(keys []string) string {
	if len(keys) == 0 {
		return "none"
	}
	return strings.Join(keys, ", ")
}
