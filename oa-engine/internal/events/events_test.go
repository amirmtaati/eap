package events_test

import (
	"bytes"
	"strings"
	"testing"

	"oa-engine/internal/events"
)

func TestEncode(t *testing.T) {
	var buf bytes.Buffer
	ev := events.StartEvent{
		Schema:         events.Schema,
		Event:          "start",
		Model:          "gpt-4",
		TokensEstimate: 100,
		ContextKeys:    []string{"k1", "k2"},
		DeltaCount:     1,
		HistoryCount:   0,
		FocusKey:       "focus/1",
	}

	if err := events.Encode(&buf, ev); err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}

	out := buf.String()
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("expected trailing newline")
	}
	if !strings.Contains(out, `"event":"start"`) || !strings.Contains(out, `"schema":1`) {
		t.Errorf("unexpected json output: %s", out)
	}
}
