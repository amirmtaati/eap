// Package events defines NDJSON event structures for oa streaming and commands.
package events

import (
	"encoding/json"
	"fmt"
	"io"
)

const Schema = 1

type StartEvent struct {
	Schema         int      `json:"schema"`
	Event          string   `json:"event"`
	Model          string   `json:"model"`
	TokensEstimate int      `json:"tokens_estimate"`
	ContextKeys    []string `json:"context_keys"`
	DeltaCount     int      `json:"delta_count"`
	HistoryCount   int      `json:"history_count"`
	FocusKey       string   `json:"focus_key"`
}

type DeltaEvent struct {
	Schema int    `json:"schema"`
	Event  string `json:"event"`
	Text   string `json:"text"`
}

type UsageEvent struct {
	Schema           int    `json:"schema"`
	Event            string `json:"event"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
}

type DoneEvent struct {
	Schema       int    `json:"schema"`
	Event        string `json:"event"`
	FinishReason string `json:"finish_reason"`
}

type ErrorEvent struct {
	Schema  int    `json:"schema"`
	Event   string `json:"event"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type ModelItem struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by"`
}

type ModelsResponse struct {
	Schema int         `json:"schema"`
	Models []ModelItem `json:"models"`
}

// Encode writes an event as a single line JSON object followed by newline.
func Encode(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("events: marshal: %w", err)
	}
	if _, err := fmt.Fprintf(w, "%s\n", data); err != nil {
		return fmt.Errorf("events: write: %w", err)
	}
	return nil
}
