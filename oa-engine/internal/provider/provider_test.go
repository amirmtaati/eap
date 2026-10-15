package provider_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"oa-engine/internal/provider"
)

func TestValidateURL(t *testing.T) {
	cases := []struct {
		url           string
		allowInsecure bool
		wantErr       bool
	}{
		{"http://localhost:8080/v1", false, false},
		{"http://127.0.0.1:8080/v1", false, false},
		{"http://remote.host:8080/v1", false, true},
		{"http://remote.host:8080/v1", true, false},
		{"https://remote.host:8080/v1", false, false},
		{"ftp://remote.host", false, true},
	}

	for _, tc := range cases {
		_, err := provider.ValidateURL(tc.url, tc.allowInsecure)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateURL(%q, %v) err = %v, wantErr = %v", tc.url, tc.allowInsecure, err, tc.wantErr)
		}
	}
}

func TestListModels(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			http.Error(w, `{"error":{"message":"unauthorized"}}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"data":[{"id":"b-model","owned_by":"test"},{"id":"a-model","owned_by":"test"}]}`)
	}))
	defer ts.Close()

	client, err := provider.NewClient(ts.URL+"/v1", "test-key", false, time.Second, time.Second)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 || models[0].ID != "a-model" || models[1].ID != "b-model" {
		t.Fatalf("unexpected models list: %+v", models)
	}

	// Test auth failure
	clientBadAuth, _ := provider.NewClient(ts.URL+"/v1", "wrong-key", false, time.Second, time.Second)
	_, err = clientBadAuth.ListModels(context.Background())
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected auth error, got: %v", err)
	}
}

func TestStreamChat(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}
		fmt.Fprintf(w, ": comment\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello \"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"World\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5}}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer ts.Close()

	client, err := provider.NewClient(ts.URL+"/v1", "", false, time.Second, time.Second)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	var text string
	var pTokens, cTokens int32
	err = client.StreamChat(context.Background(), "test-model", "test prompt", nil, provider.StreamCallbacks{
		OnDelta: func(t string) {
			text += t
		},
		OnUsage: func(pt, ct int) {
			atomic.StoreInt32(&pTokens, int32(pt))
			atomic.StoreInt32(&cTokens, int32(ct))
		},
	})

	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if text != "Hello World" {
		t.Errorf("got text %q, want 'Hello World'", text)
	}
	if pTokens != 10 || cTokens != 5 {
		t.Errorf("unexpected tokens: prompt=%d, completion=%d", pTokens, cTokens)
	}
}
