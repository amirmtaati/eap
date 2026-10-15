package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelsCommand(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"data":[{"id":"qwen-32b","owned_by":"9router"}]}`)
	}))
	defer ts.Close()

	// 1. Success
	code, out, errOut := run("models", "-url", ts.URL)
	if code != 0 || errOut != "" {
		t.Fatalf("exit=%d stderr=%q", code, errOut)
	}
	if !strings.Contains(out, `"id":"qwen-32b"`) || !strings.Contains(out, `"schema":1`) {
		t.Errorf("unexpected models output: %s", out)
	}

	// 2. Insecure non-loopback rejection
	code, _, errOut = run("models", "-url", "http://external.site:8080")
	if code != 7 || !strings.Contains(errOut, "insecure endpoint") {
		t.Errorf("expected exit 7 for insecure url, got exit=%d stderr=%q", code, errOut)
	}

	// 3. Allowed with -allow-insecure-http
	// Will fail with network error (exit 4), not 7
	code, _, _ = run("models", "-url", "http://external.site:8080", "-allow-insecure-http", "-timeout", "100ms")
	if code == 7 {
		t.Errorf("did not expect exit 7 with -allow-insecure-http, got %d", code)
	}
}

func TestChatCommand(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("flusher required")
		}
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Chunk 1 \"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Chunk 2\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":10}}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer ts.Close()

	dir := t.TempDir()
	snap := filepath.Join(dir, "snap.json")

	// Create snapshot
	withStdin(t, "* Heading :ctx:\nInitial body\n")
	if code, _, _ := run("snapshot", "-out", snap); code != 0 {
		t.Fatalf("snapshot failed")
	}

	// 1. Chat with delta
	withStdin(t, "* Heading :ctx:\nModified body\n")
	code, out, errOut := run("chat", "-url", ts.URL, "-model", "test-model", "-snapshot", snap)
	if code != 0 || errOut != "" {
		t.Fatalf("chat exit=%d stderr=%q", code, errOut)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 NDJSON lines, got %d: %q", len(lines), out)
	}
	// Verify event sequence: start -> delta -> usage -> done
	if !strings.Contains(lines[0], `"event":"start"`) {
		t.Errorf("line 0 expected start, got %s", lines[0])
	}
	if !strings.Contains(lines[len(lines)-1], `"event":"done"`) {
		t.Errorf("last line expected done, got %s", lines[len(lines)-1])
	}

	// 2. Chat with nothing to send
	withStdin(t, "* Heading :ctx:\nInitial body\n")
	code, out, errOut = run("chat", "-url", ts.URL, "-model", "test-model", "-snapshot", snap)
	if code != 2 {
		t.Errorf("expected exit 2 on nothing to send, got %d", code)
	}
	if !strings.Contains(out, `"kind":"nothing_to_send"`) {
		t.Errorf("expected nothing_to_send error event on stdout, got %s", out)
	}
}
