// Package ctxsel selects the context nodes sent to the model.
package ctxsel

import (
	"sort"

	"oa-engine/internal/delta"
	"oa-engine/internal/org"
)

// Select returns the deduplicated context nodes in document pre-order:
// :ctx: nodes (minus :noctx:), parents of changed nodes, the focus subtree,
// and the most recent agent-history entries.
func Select(doc *org.Document, d delta.Result, focus *org.Node, historyLimit int) []*org.Node {
	set := make(map[*org.Node]bool)

	hasTag := func(n *org.Node, tag string) bool {
		for _, t := range n.Tags {
			if t == tag {
				return true
			}
		}
		return false
	}
	add := func(n *org.Node) {
		if n != nil {
			set[n] = true
		}
	}

	for _, n := range doc.All { // map order irrelevant: membership only
		if hasTag(n, "ctx") && !hasTag(n, "noctx") {
			add(n)
		}
	}
	for _, keys := range [][]string{d.Added, d.Modified} {
		for _, k := range keys {
			if n, ok := doc.All[k]; ok {
				add(n.Parent)
			}
		}
	}
	if focus != nil {
		for _, n := range focus.Subtree() {
			add(n)
		}
	}
	var agent *org.Node
	if n, ok := doc.All["agent"]; ok {
		agent = n
	} else {
		var keys []string
		for k, n := range doc.All {
			if n.Title == "Agent" {
				keys = append(keys, k)
			}
		}
		if len(keys) > 0 {
			sort.Strings(keys)
			agent = doc.All[keys[0]]
		}
	}
	if agent != nil && historyLimit > 0 {
		var recent []*org.Node // level-2 children, pre-order
		for _, c := range agent.Children {
			if c.Level == 2 {
				recent = append(recent, c)
			}
		}
		if len(recent) > historyLimit {
			recent = recent[len(recent)-historyLimit:]
		}
		for _, n := range recent {
			add(n)
		}
	}

	var out []*org.Node
	var walk func(ns []*org.Node)
	walk = func(ns []*org.Node) {
		for _, n := range ns {
			if set[n] {
				out = append(out, n)
			}
			walk(n.Children)
		}
	}
	walk(doc.Roots)
	return out
}
