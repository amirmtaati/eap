// Package cli implements the oa subcommands: parse, snapshot, prompt, version.
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"oa-engine/internal/ctxsel"
	"oa-engine/internal/delta"
	"oa-engine/internal/org"
	"oa-engine/internal/prompt"
	"oa-engine/internal/snapshot"
)

const (
	version    = "0.1.0"
	protocol   = 1
	usageLine  = "usage: oa parse|snapshot|prompt|version [-file PATH|-] [flags]"
	historyDef = 3
)

// Run executes one subcommand and returns the process exit code:
// 0 ok, 1 usage/IO/unknown focus, 2 nothing to send, 3 unsupported snapshot.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usageLine)
		return 1
	}
	switch args[0] {
	case "parse":
		return runParse(args[1:], stdout, stderr)
	case "snapshot":
		return runSnapshot(args[1:], stdout, stderr)
	case "prompt":
		return runPrompt(args[1:], stdout, stderr)
	case "version":
		return runVersion(args[1:], stdout, stderr)
	default:
		fmt.Fprintln(stderr, usageLine)
		return 1
	}
}

// newFlagSet builds a FlagSet that reports errors as a single stderr line
// (no multi-line usage dump) and never exits the process itself.
func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	return fs
}

// readInput reads the org document from path, "-" meaning stdin.
func readInput(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

func failf(stderr io.Writer, format string, a ...any) int {
	fmt.Fprintf(stderr, format+"\n", a...)
	return 1
}

// fileArg reports whether leftover positional args are acceptable: none, or
// a single "-" stdin marker (flag parsing stops at the bare dash).
func fileArg(fs *flag.FlagSet) bool {
	return fs.NArg() == 0 || (fs.NArg() == 1 && fs.Arg(0) == "-")
}

func runParse(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("parse", stderr)
	file := fs.String("file", "-", "org file path, - for stdin")
	asJSON := fs.Bool("json", false, "emit the document as compact JSON")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if !fileArg(fs) {
		fmt.Fprintln(stderr, usageLine)
		return 1
	}
	doc, code := loadDoc(*file, "parse", stderr)
	if code != 0 {
		return code
	}
	if !*asJSON {
		return printSummary(doc, stdout)
	}
	out, err := json.Marshal(docJSON(doc))
	if err != nil {
		return failf(stderr, "parse: encode: %v", err)
	}
	fmt.Fprintln(stdout, string(out))
	return 0
}

// loadDoc reads and parses the org document; returns (doc, 0) or (nil, exit).
func loadDoc(path, cmd string, stderr io.Writer) (*org.Document, int) {
	data, err := readInput(path)
	if err != nil {
		return nil, failf(stderr, "%s: read %s: %v", cmd, path, err)
	}
	doc, err := org.Parse(string(data))
	if err != nil {
		return nil, failf(stderr, "%s: %v", cmd, err)
	}
	return doc, 0
}

type nodeJSON struct {
	Key         string            `json:"key"`
	Title       string            `json:"title"`
	Level       int               `json:"level"`
	Line        int               `json:"line"`
	Tags        []string          `json:"tags"`
	Properties  map[string]string `json:"properties"`
	Body        []string          `json:"body"`
	ContentHash string            `json:"content_hash"`
	SubtreeHash string            `json:"subtree_hash"`
}

func docJSON(doc *org.Document) any {
	nodes := make([]nodeJSON, 0, len(doc.All))
	var walk func(ns []*org.Node)
	walk = func(ns []*org.Node) {
		for _, n := range ns {
			tags := n.Tags
			if tags == nil {
				tags = []string{}
			}
			body := n.Body
			if body == nil {
				body = []string{}
			}
			props := n.Properties
			if props == nil {
				props = map[string]string{}
			}
			nodes = append(nodes, nodeJSON{
				Key: n.Key, Title: n.Title, Level: n.Level, Line: n.Line,
				Tags: tags, Properties: props, Body: body,
				ContentHash: n.ContentHash, SubtreeHash: n.SubtreeHash,
			})
			walk(n.Children)
		}
	}
	walk(doc.Roots)
	return struct {
		Schema int        `json:"schema"`
		Nodes  []nodeJSON `json:"nodes"`
	}{protocol, nodes}
}

// printSummary writes the human summary: node count, then keys grouped by
// level (levels ascending, keys in pre-order within a level).
func printSummary(doc *org.Document, stdout io.Writer) int {
	byLevel := map[int][]string{}
	total := 0
	var walk func(ns []*org.Node)
	walk = func(ns []*org.Node) {
		for _, n := range ns {
			byLevel[n.Level] = append(byLevel[n.Level], n.Key)
			total++
			walk(n.Children)
		}
	}
	walk(doc.Roots)
	fmt.Fprintf(stdout, "nodes: %d\n", total)
	levels := make([]int, 0, len(byLevel))
	for lvl := range byLevel {
		levels = append(levels, lvl)
	}
	sort.Ints(levels)
	for _, lvl := range levels {
		fmt.Fprintf(stdout, "level %d: %s\n", lvl, strings.Join(byLevel[lvl], ", "))
	}
	return 0
}

func runSnapshot(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("snapshot", stderr)
	out := fs.String("out", "", "snapshot output path")
	file := fs.String("file", "-", "org file path, - for stdin")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *out == "" || !fileArg(fs) {
		return failf(stderr, "snapshot: -out PATH is required")
	}
	doc, code := loadDoc(*file, "snapshot", stderr)
	if code != 0 {
		return code
	}
	snap := &snapshot.Snapshot{Schema: protocol, Nodes: make(map[string]snapshot.Entry)}
	var walk func(ns []*org.Node)
	walk = func(ns []*org.Node) {
		for _, n := range ns {
			snap.Nodes[n.Key] = snapshot.Entry{ContentHash: n.ContentHash, SubtreeHash: n.SubtreeHash}
			walk(n.Children)
		}
	}
	walk(doc.Roots)
	if err := snap.Save(*out); err != nil {
		return failf(stderr, "snapshot: %v", err)
	}
	return 0
}

func runPrompt(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("prompt", stderr)
	file := fs.String("file", "-", "org file path, - for stdin")
	snapPath := fs.String("snapshot", "", "snapshot file path")
	focusKey := fs.String("focus", "", "focus node key")
	focusLine := fs.Int("focus-line", 0, "focus node line (1-based)")
	history := fs.Int("history", historyDef, "agent history limit")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *snapPath == "" || !fileArg(fs) {
		return failf(stderr, "prompt: -snapshot PATH is required")
	}
	doc, code := loadDoc(*file, "prompt", stderr)
	if code != 0 {
		return code
	}
	snap, err := snapshot.Load(*snapPath)
	if err != nil {
		if errors.Is(err, snapshot.ErrUnsupportedSchema) {
			fmt.Fprintf(stderr, "prompt: %v\n", err)
			return 3
		}
		return failf(stderr, "prompt: %v", err)
	}
	var focus *org.Node
	if *focusKey != "" || *focusLine > 0 {
		f, err := delta.ResolveFocus(doc, *focusKey, *focusLine)
		if err != nil {
			return failf(stderr, "prompt: %v", err)
		}
		focus = f
	}
	d := delta.Compute(doc, snap)
	if len(d.Added)+len(d.Modified)+len(d.Deleted) == 0 {
		fmt.Fprintln(stderr, "nothing to send")
		return 2
	}
	ctx := ctxsel.Select(doc, d, focus, *history)
	fmt.Fprint(stdout, prompt.Build(doc, d, ctx, focus))
	return 0
}

func runVersion(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("version", stderr)
	asJSON := fs.Bool("json", false, "emit version as JSON")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(stderr, usageLine)
		return 1
	}
	if *asJSON {
		out, err := json.Marshal(struct {
			Schema   int    `json:"schema"`
			Version  string `json:"version"`
			Protocol int    `json:"protocol"`
		}{protocol, version, protocol})
		if err != nil {
			return failf(stderr, "version: encode: %v", err)
		}
		fmt.Fprintln(stdout, string(out))
		return 0
	}
	fmt.Fprintf(stdout, "oa version %s (protocol %d)\n", version, protocol)
	return 0
}
