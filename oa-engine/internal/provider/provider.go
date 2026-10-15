// Package provider implements OpenAI-compatible HTTP client for models and streaming chat.
package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"oa-engine/internal/events"
)

var (
	ErrInsecureEndpoint = errors.New("insecure endpoint: plain http not allowed for non-loopback host")
	ErrAuth             = errors.New("authentication or authorization failure (401/403)")
	ErrProvider         = errors.New("provider error")
	ErrNetwork          = errors.New("network error or timeout")
)

const maxResponseBody = 64 * 1024 * 1024 // 64 MB

// ValidateURL checks whether URL scheme and host are allowed under security policy.
func ValidateURL(rawURL string, allowInsecure bool) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}
	if u.Scheme == "http" && !allowInsecure {
		host := u.Hostname()
		if !isLoopback(host) {
			return nil, ErrInsecureEndpoint
		}
	}
	return u, nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}

// Client handles requests to an OpenAI-compatible endpoint.
type Client struct {
	BaseURL       string
	APIKey        string
	AllowInsecure bool
	Timeout       time.Duration
	IdleTimeout   time.Duration
	HTTPClient    *http.Client
}

func NewClient(baseURL, apiKey string, allowInsecure bool, timeout, idleTimeout time.Duration) (*Client, error) {
	cleanURL := strings.TrimRight(baseURL, "/")
	if _, err := ValidateURL(cleanURL, allowInsecure); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if idleTimeout <= 0 {
		idleTimeout = 60 * time.Second
	}
	c := &Client{
		BaseURL:       cleanURL,
		APIKey:        apiKey,
		AllowInsecure: allowInsecure,
		Timeout:       timeout,
		IdleTimeout:   idleTimeout,
		HTTPClient: &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) > 0 && req.URL.Host != via[0].URL.Host {
					return errors.New("redirect to different host refused")
				}
				return nil
			},
		},
	}
	return c, nil
}

// ListModels calls GET {BaseURL}/models and returns sorted list of models.
func (c *Client) ListModels(ctx context.Context) ([]events.ModelItem, error) {
	reqCtx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	endpoint := c.BaseURL + "/models"
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || osIsTimeout(err) {
			return nil, fmt.Errorf("%w: request timed out", ErrNetwork)
		}
		return nil, fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("%w: status %d", ErrAuth, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errMsg := parseErrorResponse(resp.Body)
		return nil, fmt.Errorf("%w: status %d: %s", ErrProvider, resp.StatusCode, errMsg)
	}

	lr := io.LimitReader(resp.Body, maxResponseBody)
	var raw struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(lr).Decode(&raw); err != nil {
		return nil, fmt.Errorf("%w: malformed JSON: %v", ErrProvider, err)
	}

	models := make([]events.ModelItem, len(raw.Data))
	for i, m := range raw.Data {
		models[i] = events.ModelItem{
			ID:      m.ID,
			OwnedBy: m.OwnedBy,
		}
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})
	return models, nil
}

// ChatRequest payload sent to /chat/completions.
type chatPayload struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	Stream    bool          `json:"stream"`
	MaxTokens *int          `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type StreamCallbacks struct {
	OnDelta func(text string)
	OnUsage func(promptTokens, completionTokens int)
}

// StreamChat calls POST {BaseURL}/chat/completions and parses SSE events.
func (c *Client) StreamChat(ctx context.Context, model, promptText string, maxTokens *int, cb StreamCallbacks) error {
	payload := chatPayload{
		Model: model,
		Messages: []chatMessage{
			{Role: "user", Content: promptText},
		},
		Stream:    true,
		MaxTokens: maxTokens,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	endpoint := c.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) || osIsTimeout(err) {
			return fmt.Errorf("%w: connect timeout", ErrNetwork)
		}
		return fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: status %d", ErrAuth, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errMsg := parseErrorResponse(resp.Body)
		return fmt.Errorf("%w: status %d: %s", ErrProvider, resp.StatusCode, errMsg)
	}

	// Read SSE stream with idle timeout detection
	reader := bufio.NewReader(io.LimitReader(resp.Body, maxResponseBody))
	return c.readSSE(ctx, reader, cb)
}

func (c *Client) readSSE(ctx context.Context, r *bufio.Reader, cb StreamCallbacks) error {
	type lineRes struct {
		line string
		err  error
	}

	lineChan := make(chan lineRes, 1)
	readDone := make(chan struct{})
	defer close(readDone)

	go func() {
		for {
			line, err := r.ReadString('\n')
			select {
			case lineChan <- lineRes{line: line, err: err}:
			case <-readDone:
				return
			}
			if err != nil {
				return
			}
		}
	}()

	timer := time.NewTimer(c.IdleTimeout)
	defer timer.Stop()

	for {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(c.IdleTimeout)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("%w: idle timeout waiting for SSE chunk", ErrNetwork)
		case res := <-lineChan:
			if res.err != nil {
				if errors.Is(res.err, io.EOF) {
					return nil
				}
				return fmt.Errorf("%w: read stream: %v", ErrNetwork, res.err)
			}

			line := strings.TrimSpace(res.line)
			if line == "" || strings.HasPrefix(line, ":") {
				continue // empty line or SSE comment
			}
			if !strings.HasPrefix(line, "data:") {
				continue
			}

			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				return nil
			}

			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
				Usage *struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
				} `json:"usage"`
			}

			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue // ignore unparseable chunk
			}

			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				if cb.OnDelta != nil {
					cb.OnDelta(chunk.Choices[0].Delta.Content)
				}
			}
			if chunk.Usage != nil && cb.OnUsage != nil {
				cb.OnUsage(chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens)
			}
		}
	}
}

func parseErrorResponse(r io.Reader) string {
	var errObj struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	lr := io.LimitReader(r, 4096)
	if err := json.NewDecoder(lr).Decode(&errObj); err == nil && errObj.Error.Message != "" {
		return errObj.Error.Message
	}
	return "unknown error"
}

func osIsTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
