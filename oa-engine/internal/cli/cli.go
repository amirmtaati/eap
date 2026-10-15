// Package cli implements the oa subcommands: parse, snapshot, prompt, version, models, chat.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"oa-engine/internal/ctxsel"
	"oa-engine/internal/delta"
	"oa-engine/internal/events"
	"oa-engine/internal/org"
	"oa-engine/internal/prompt"
	"oa-engine/internal/provider"
	"oa-engine/internal/snapshot"
)

const (
	version    = "0.1.0"
	protocol   = 1
	usageLine  = "usage: oa parse|snapshot|prompt|version|models|chat [flags]"
	historyDef = 3
)

// Run executes one subcommand and returns the process exit code:
// 0 ok, 1 usage/IO/unknown focus, 2 nothing to send, 3 unsupported snapshot,
// 4 network/timeout, 5 auth, 6 provider, 7 insecure endpoint, 130 cancelled.
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
	case "models":
		return runModels(args[1:], stdout, stderr)
	case "chat":
		return runChat(args[1:], stdout, stderr)
	default:
		fmt.Fprintln(stderr, usageLine)
		return 1
	}
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	return fs
}

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

func failCode(stderr io.Writer, code int, format string, a ...any) int {
	fmt.Fprintf(stderr, format+"\n", a...)
	return code
}

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

func runModels(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("models", stderr)
	urlFlag := fs.String("url", "", "endpoint base URL")
	timeoutFlag := fs.Duration("timeout", 10*time.Second, "request timeout")
	allowInsecure := fs.Bool("allow-insecure-http", false, "allow plain http for non-loopback hosts")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *urlFlag == "" || fs.NArg() > 0 {
		return failf(stderr, "models: -url URL is required")
	}

	apiKey := os.Getenv("OA_API_KEY")
	client, err := provider.NewClient(*urlFlag, apiKey, *allowInsecure, *timeoutFlag, 0)
	if err != nil {
		if errors.Is(err, provider.ErrInsecureEndpoint) {
			return failCode(stderr, 7, "models: %v", err)
		}
		return failCode(stderr, 1, "models: %v", err)
	}

	models, err := client.ListModels(context.Background())
	if err != nil {
		if errors.Is(err, provider.ErrInsecureEndpoint) {
			return failCode(stderr, 7, "models: %v", err)
		}
		if errors.Is(err, provider.ErrAuth) {
			return failCode(stderr, 5, "models: %v", err)
		}
		if errors.Is(err, provider.ErrProvider) {
			return failCode(stderr, 6, "models: %v", err)
		}
		if errors.Is(err, provider.ErrNetwork) {
			return failCode(stderr, 4, "models: %v", err)
		}
		return failCode(stderr, 1, "models: %v", err)
	}

	resp := events.ModelsResponse{
		Schema: protocol,
		Models: models,
	}
	out, err := json.Marshal(resp)
	if err != nil {
		return failCode(stderr, 1, "models: encode: %v", err)
	}
	fmt.Fprintln(stdout, string(out))
	return 0
}

func runChat(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("chat", stderr)
	urlFlag := fs.String("url", "", "endpoint base URL")
	modelFlag := fs.String("model", "", "model ID")
	fileFlag := fs.String("file", "-", "org file path, - for stdin")
	snapPath := fs.String("snapshot", "", "snapshot file path")
	focusKey := fs.String("focus", "", "focus node key")
	focusLine := fs.Int("focus-line", 0, "focus node line")
	history := fs.Int("history", historyDef, "agent history limit")
	maxTokens := fs.Int("max-tokens", 0, "max tokens to generate")
	timeout := fs.Duration("timeout", 10*time.Second, "connect timeout")
	idleTimeout := fs.Duration("idle-timeout", 60*time.Second, "stream idle timeout")
	allowInsecure := fs.Bool("allow-insecure-http", false, "allow plain http for non-loopback hosts")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	// Positional arguments check: allow single trailing path or "-"
	targetFile := *fileFlag
	if fs.NArg() == 1 {
		targetFile = fs.Arg(0)
	} else if fs.NArg() > 1 {
		return failChatError(stdout, stderr, 1, "bad_request", "chat: too many arguments")
	}

	if *urlFlag == "" || *modelFlag == "" || *snapPath == "" {
		return failChatError(stdout, stderr, 1, "bad_request", "chat: -url, -model, and -snapshot are required")
	}

	apiKey := os.Getenv("OA_API_KEY")
	client, err := provider.NewClient(*urlFlag, apiKey, *allowInsecure, *timeout, *idleTimeout)
	if err != nil {
		if errors.Is(err, provider.ErrInsecureEndpoint) {
			return failChatError(stdout, stderr, 7, "insecure_endpoint", err.Error())
		}
		return failChatError(stdout, stderr, 1, "bad_request", err.Error())
	}

	doc, code := loadDoc(targetFile, "chat", stderr)
	if code != 0 {
		return failChatError(stdout, stderr, 1, "bad_request", fmt.Sprintf("cannot read %s", targetFile))
	}

	snap, err := snapshot.Load(*snapPath)
	if err != nil {
		if errors.Is(err, snapshot.ErrUnsupportedSchema) {
			return failChatError(stdout, stderr, 3, "unsupported_snapshot", err.Error())
		}
		return failChatError(stdout, stderr, 1, "bad_request", err.Error())
	}

	var focus *org.Node
	if *focusKey != "" || *focusLine > 0 {
		f, err := delta.ResolveFocus(doc, *focusKey, *focusLine)
		if err != nil {
			return failChatError(stdout, stderr, 1, "bad_request", err.Error())
		}
		focus = f
	}

	d := delta.Compute(doc, snap)
	deltaCount := len(d.Added) + len(d.Modified) + len(d.Deleted)
	if deltaCount == 0 {
		return failChatError(stdout, stderr, 2, "nothing_to_send", "nothing to send")
	}

	ctxNodes := ctxsel.Select(doc, d, focus, *history)
	promptText := prompt.Build(doc, d, ctxNodes, focus)

	// Context keys list for start event
	ctxKeys := make([]string, len(ctxNodes))
	for i, n := range ctxNodes {
		ctxKeys[i] = n.Key
	}

	focusKeyStr := ""
	if focus != nil {
		focusKeyStr = focus.Key
	}

	tokensEst := (len(promptText) + 3) / 4

	// Count history items included in ctxNodes
	histCount := 0
	for _, n := range ctxNodes {
		if n.Parent != nil && (n.Parent.Key == "agent" || n.Parent.Title == "Agent") {
			histCount++
		}
	}

	// 1. Emit start event
	_ = events.Encode(stdout, events.StartEvent{
		Schema:         protocol,
		Event:          "start",
		Model:          *modelFlag,
		TokensEstimate: tokensEst,
		ContextKeys:    ctxKeys,
		DeltaCount:     deltaCount,
		HistoryCount:   histCount,
		FocusKey:       focusKeyStr,
	})

	// Setup context with cancellation on SIGINT/SIGTERM
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	go func() {
		select {
		case <-sigChan:
			cancel()
		case <-ctx.Done():
		}
	}()

	var maxTokensPtr *int
	if *maxTokens > 0 {
		maxTokensPtr = maxTokens
	}

	cb := provider.StreamCallbacks{
		OnDelta: func(text string) {
			_ = events.Encode(stdout, events.DeltaEvent{
				Schema: protocol,
				Event:  "delta",
				Text:   text,
			})
		},
		OnUsage: func(promptTokens, completionTokens int) {
			_ = events.Encode(stdout, events.UsageEvent{
				Schema:           protocol,
				Event:            "usage",
				PromptTokens:     promptTokens,
				CompletionTokens: completionTokens,
			})
		},
	}

	streamErr := client.StreamChat(ctx, *modelFlag, promptText, maxTokensPtr, cb)
	if streamErr != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return failChatError(stdout, stderr, 130, "cancelled", "request cancelled")
		}
		if errors.Is(streamErr, provider.ErrInsecureEndpoint) {
			return failChatError(stdout, stderr, 7, "insecure_endpoint", streamErr.Error())
		}
		if errors.Is(streamErr, provider.ErrAuth) {
			return failChatError(stdout, stderr, 5, "auth", streamErr.Error())
		}
		if errors.Is(streamErr, provider.ErrNetwork) {
			return failChatError(stdout, stderr, 4, "network", streamErr.Error())
		}
		if errors.Is(streamErr, provider.ErrProvider) {
			return failChatError(stdout, stderr, 6, "provider", streamErr.Error())
		}
		return failChatError(stdout, stderr, 6, "provider", streamErr.Error())
	}

	_ = events.Encode(stdout, events.DoneEvent{
		Schema:       protocol,
		Event:        "done",
		FinishReason: "stop",
	})
	return 0
}

func failChatError(stdout, stderr io.Writer, code int, kind, msg string) int {
	_ = events.Encode(stdout, events.ErrorEvent{
		Schema:  protocol,
		Event:   "error",
		Kind:    kind,
		Message: msg,
	})
	fmt.Fprintln(stderr, msg)
	return code
}
