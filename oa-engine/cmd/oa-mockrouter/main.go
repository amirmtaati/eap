// Command oa-mockrouter is a test mock OpenAI-compatible server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:20128", "listen address")
	flag.Parse()

	mux := http.NewServeMux()

	handleModels := func(w http.ResponseWriter, r *http.Request) {
		if fault := r.Header.Get("X-Mock-Fault"); fault == "auth" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, `{"error":{"message":"unauthorized mock"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"data":[{"id":"mock/qwen2.5-coder-32b","owned_by":"9router"},{"id":"mock/gpt-4o","owned_by":"openai"}]}`)
	}

	handleChat := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if fault := r.Header.Get("X-Mock-Fault"); fault == "auth" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, `{"error":{"message":"invalid api key"}}`)
			return
		}
		if fault := r.Header.Get("X-Mock-Fault"); fault == "server" {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, `{"error":{"message":"mock internal error"}}`)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		chunks := []string{
			`{"choices":[{"delta":{"content":"** Proposed Solution\n"}}]}`,
			`{"choices":[{"delta":{"content":"Here is the updated implementation plan.\n"}}]}`,
			`{"choices":[{"delta":{"content":"- Complete the tests\n- Ship client\n"}}]}`,
		}

		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
			time.Sleep(20 * time.Millisecond)
		}

		fmt.Fprintf(w, "data: {\"usage\":{\"prompt_tokens\":120,\"completion_tokens\":45}}\n\n")
		flusher.Flush()

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}

	mux.HandleFunc("/models", handleModels)
	mux.HandleFunc("/v1/models", handleModels)
	mux.HandleFunc("/chat/completions", handleChat)
	mux.HandleFunc("/v1/chat/completions", handleChat)

	srv := &http.Server{
		Addr:    *addr,
		Handler: mux,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("oa-mockrouter listening on %s", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-stop
	log.Println("shutting down oa-mockrouter...")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
