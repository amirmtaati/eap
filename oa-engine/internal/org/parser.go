// Package org parses Org-mode documents into a node tree, assigns stable
// keys, and computes content and subtree hashes.
package org

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Node is one org headline together with its body and derived identity.
type Node struct {
	Key, Title               string
	Level, Line              int // Line = 1-based line of the headline
	Tags                     []string
	Properties               map[string]string
	Body                     []string
	ContentHash, SubtreeHash string
	Children                 []*Node
	Parent                   *Node `json:"-"` // not serialized
}

// Document is a parsed org document: the root nodes plus a flat pre-order
// index of every node by key.
type Document struct {
	Roots []*Node
	All   map[string]*Node // key -> node, flat pre-order
}

var (
	headlineRe = regexp.MustCompile(`^(\*+)\s+(.*?)(?:\s+:([A-Za-z0-9_@#%:]+):)?$`)
	propRe     = regexp.MustCompile(`^:([^:\s]+):(.*)$`)
)

// Parse reads an org document and builds its tree. Malformed input is
// tolerated: lines that do not match known syntax become body text, so Parse
// never fails on any input.
func Parse(text string) (*Document, error) {
	lines := strings.Split(text, "\n")
	doc := &Document{All: make(map[string]*Node)}
	var stack []*Node
	var all []*Node // pre-order
	var cur *Node

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSuffix(lines[i], "\r")
		m := headlineRe.FindStringSubmatch(line)
		if m == nil {
			if cur != nil {
				cur.Body = append(cur.Body, line)
			}
			continue
		}
		n := &Node{
			Level:      len(m[1]),
			Line:       i + 1,
			Title:      m[2],
			Tags:       splitTags(m[3]),
			Properties: make(map[string]string),
		}
		for len(stack) > 0 && stack[len(stack)-1].Level >= n.Level {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 {
			n.Parent = stack[len(stack)-1]
			n.Parent.Children = append(n.Parent.Children, n)
		} else {
			doc.Roots = append(doc.Roots, n)
		}
		stack = append(stack, n)
		all = append(all, n)
		cur = n

		// Property drawer: :PROPERTIES: may follow the headline after blank
		// lines; drawer lines are consumed here, never added to Body.
		j := i + 1
		for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
			j++
		}
		if j < len(lines) && strings.EqualFold(strings.TrimSpace(lines[j]), ":properties:") {
			j++
			for j < len(lines) {
				t := strings.TrimSpace(lines[j])
				if strings.EqualFold(t, ":end:") {
					j++
					break
				}
				pm := propRe.FindStringSubmatch(t)
				if pm == nil {
					break // not drawer content; resume normal body handling
				}
				n.Properties[strings.ToUpper(pm[1])] = strings.TrimSpace(pm[2])
				j++
			}
			i = j - 1 // the outer i++ resumes at line j
		}
	}

	for _, n := range all {
		n.Body = trimBlanks(n.Body)
	}

	seen := make(map[string]bool, len(all))
	for _, n := range all {
		base := ""
		if v, ok := n.Properties["CUSTOM_ID"]; ok {
			base = Slug(v)
		} else if v, ok := n.Properties["ID"]; ok {
			base = Slug(v)
		} else {
			var parts []string
			for p := n.Parent; p != nil; p = p.Parent {
				parts = append([]string{Slug(p.Title)}, parts...)
			}
			parts = append(parts, Slug(n.Title))
			base = strings.Join(parts, "/")
		}
		key := base
		for i := 1; seen[key]; i++ {
			key = base + "-" + strconv.Itoa(i)
		}
		seen[key] = true
		n.Key = key
		doc.All[key] = n
	}

	// Hashes: all is in pre-order, so reversed iteration is post-order and
	// every child's SubtreeHash is final before its parent reads it.
	for idx := len(all) - 1; idx >= 0; idx-- {
		n := all[idx]
		keys := make([]string, 0, len(n.Properties))
		for k := range n.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		props := make([]string, len(keys))
		for i, k := range keys {
			props[i] = k + "=" + n.Properties[k]
		}
		n.ContentHash = hashHex(n.Title + "\n" + strings.Join(n.Tags, ",") + "\n" +
			strings.Join(props, "\n") + "\n" + strings.Join(n.Body, "\n"))
		childHashes := make([]string, len(n.Children))
		for i, c := range n.Children {
			childHashes[i] = c.SubtreeHash
		}
		n.SubtreeHash = hashHex(n.ContentHash + "\x00" + strings.Join(childHashes, "\x00"))
	}

	return doc, nil
}

func splitTags(s string) []string {
	parts := strings.Split(s, ":")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			tags = append(tags, p)
		}
	}
	if len(tags) == 0 {
		return nil
	}
	return tags
}

func trimBlanks(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func hashHex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// Slug lowercases s, replaces every run of characters outside [a-z0-9] with a
// single "-", trims leading and trailing "-", and returns "node" when empty.
func Slug(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "node"
	}
	return out
}

// Headline re-renders the node's headline line: stars, title and tags.
func (n *Node) Headline() string {
	var b strings.Builder
	b.WriteString(strings.Repeat("*", n.Level))
	if n.Title != "" || len(n.Tags) > 0 {
		b.WriteString(" ")
		b.WriteString(n.Title)
	}
	if len(n.Tags) > 0 {
		b.WriteString(" :")
		b.WriteString(strings.Join(n.Tags, ":"))
		b.WriteString(":")
	}
	return b.String()
}

// Text returns the headline followed by the body lines.
func (n *Node) Text() string {
	if len(n.Body) == 0 {
		return n.Headline()
	}
	return n.Headline() + "\n" + strings.Join(n.Body, "\n")
}

// Subtree returns the node and all descendants in pre-order.
func (n *Node) Subtree() []*Node {
	out := make([]*Node, 0, 1+len(n.Children))
	out = append(out, n)
	for _, c := range n.Children {
		out = append(out, c.Subtree()...)
	}
	return out
}
