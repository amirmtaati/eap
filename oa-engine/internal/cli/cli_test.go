package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withStdin feeds s to the process stdin for the duration of the test.
func withStdin(t *testing.T, s string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, s); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = old
		r.Close()
	})
}

// run invokes Run and returns exit code, stdout and stderr.
func run(args ...string) (int, string, string) {
	var out, errB bytes.Buffer
	code := Run(args, &out, &errB)
	return code, out.String(), errB.String()
}

func TestParseJSONShape(t *testing.T) {
	withStdin(t, "* One\n** Two :ctx:\nbody\n")
	code, out, errOut := run("parse", "-json")
	if code != 0 || errOut != "" {
		t.Fatalf("exit=%d stderr=%q", code, errOut)
	}
	if n := strings.Count(out, "\n"); n != 1 || !strings.HasSuffix(out, "\n") {
		t.Fatalf("want single compact line, got %d newlines: %q", n, out)
	}
	var got struct {
		Schema int `json:"schema"`
		Nodes  []struct {
			Key         string            `json:"key"`
			Title       string            `json:"title"`
			Level       int               `json:"level"`
			Line        int               `json:"line"`
			Tags        []string          `json:"tags"`
			Properties  map[string]string `json:"properties"`
			Body        []string          `json:"body"`
			ContentHash string            `json:"content_hash"`
			SubtreeHash string            `json:"subtree_hash"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if got.Schema != 1 || len(got.Nodes) != 2 {
		t.Fatalf("schema=%d nodes=%d", got.Schema, len(got.Nodes))
	}
	n0, n1 := got.Nodes[0], got.Nodes[1]
	if n0.Key != "one" || n0.Title != "One" || n0.Level != 1 || n0.Line != 1 ||
		len(n0.Tags) != 0 || len(n0.Properties) != 0 || len(n0.Body) != 0 ||
		n0.ContentHash == "" || n0.SubtreeHash == "" {
		t.Errorf("node0 = %+v", n0)
	}
	if n1.Key != "one/two" || n1.Level != 2 || n1.Line != 2 ||
		strings.Join(n1.Tags, ",") != "ctx" ||
		strings.Join(n1.Body, "|") != "body" {
		t.Errorf("node1 = %+v", n1)
	}
}

func TestParseHumanSummaryAndFileFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.org")
	if err := os.WriteFile(path, []byte("* One\n** Two :ctx:\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := run("parse", "-file", path)
	if code != 0 || errOut != "" {
		t.Fatalf("exit=%d stderr=%q", code, errOut)
	}
	want := "nodes: 2\nlevel 1: one\nlevel 2: one/two\n"
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestSnapshotThenPromptRoundtrip(t *testing.T) {
	dir := t.TempDir()
	snap := filepath.Join(dir, "snap.json")

	withStdin(t, "* One\n** Two :ctx:\nbody\n")
	if code, _, errOut := run("snapshot", "-out", snap); code != 0 || errOut != "" {
		t.Fatalf("snapshot exit=%d stderr=%q", code, errOut)
	}
	if _, err := os.Stat(snap); err != nil {
		t.Fatalf("snapshot file missing: %v", err)
	}

	withStdin(t, "* One\n** Two :ctx:\nbody changed\n")
	code, out, errOut := run("prompt", "-snapshot", snap)
	if code != 0 || errOut != "" {
		t.Fatalf("prompt exit=%d stderr=%q", code, errOut)
	}
	if out == "" || !strings.Contains(out, "# Changes") || !strings.Contains(out, "body changed") {
		t.Errorf("unexpected prompt output %q", out)
	}
}

func TestPromptNothingToSend(t *testing.T) {
	dir := t.TempDir()
	snap := filepath.Join(dir, "snap.json")
	withStdin(t, "* One\n** Two :ctx:\nbody\n")
	if code, _, errOut := run("snapshot", "-out", snap); code != 0 || errOut != "" {
		t.Fatalf("snapshot exit=%d stderr=%q", code, errOut)
	}
	withStdin(t, "* One\n** Two :ctx:\nbody\n")
	code, out, errOut := run("prompt", "-snapshot", snap)
	if code != 2 || out != "" || errOut != "nothing to send\n" {
		t.Errorf("exit=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestPromptUnsupportedSnapshotSchema(t *testing.T) {
	dir := t.TempDir()
	snap := filepath.Join(dir, "snap.json")
	if err := os.WriteFile(snap, []byte(`{"schema":2,"nodes":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	withStdin(t, "* One\n")
	code, out, errOut := run("prompt", "-snapshot", snap)
	if code != 3 || out != "" || !strings.Contains(errOut, "unsupported snapshot schema") {
		t.Errorf("exit=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestPromptUnknownFocusKey(t *testing.T) {
	dir := t.TempDir()
	snap := filepath.Join(dir, "snap.json")
	withStdin(t, "* One\n")
	if code, _, errOut := run("snapshot", "-out", snap); code != 0 || errOut != "" {
		t.Fatalf("snapshot exit=%d stderr=%q", code, errOut)
	}
	withStdin(t, "* One\n** Two :ctx:\nbody\n")
	code, out, errOut := run("prompt", "-snapshot", snap, "-focus", "nope")
	if code != 1 || out != "" || !strings.Contains(errOut, "nope") {
		t.Errorf("exit=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestBadUsage(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"unknown command", []string{"bogus"}},
		{"no command", nil},
		{"bad flag", []string{"parse", "-bogus"}},
		{"extra arg", []string{"version", "-json", "extra"}},
		{"missing snapshot", []string{"prompt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := run(tt.args...)
			if code != 1 || out != "" || strings.Count(errOut, "\n") != 1 {
				t.Errorf("exit=%d stdout=%q stderr=%q", code, out, errOut)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	code, out, errOut := run("version")
	if code != 0 || errOut != "" || out != "oa version 0.1.0 (protocol 1)\n" {
		t.Errorf("exit=%d stdout=%q stderr=%q", code, out, errOut)
	}
	code, out, errOut = run("version", "-json")
	if code != 0 || errOut != "" || out != `{"schema":1,"version":"0.1.0","protocol":1}`+"\n" {
		t.Errorf("exit=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestPromptFocusLine(t *testing.T) {
	dir := t.TempDir()
	snap := filepath.Join(dir, "snap.json")
	withStdin(t, "* One\n** Two :ctx:\nbody\n")
	if code, _, errOut := run("snapshot", "-out", snap); code != 0 || errOut != "" {
		t.Fatalf("snapshot exit=%d stderr=%q", code, errOut)
	}
	withStdin(t, "* One\n** Two :ctx:\nbody changed\n")
	code, out, _ := run("prompt", "-snapshot", snap, "-focus-line", "2")
	if code != 0 || !strings.Contains(out, "Focus on the following subtree:") {
		t.Errorf("exit=%d stdout=%q", code, out)
	}
}
