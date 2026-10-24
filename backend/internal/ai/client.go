package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Anthropic Messages API wire constants (raw HTTP; no SDK dependency).
const (
	defaultBaseURL   = "https://api.anthropic.com"
	anthropicVersion = "2026-10-21"
	// server-side fallback: a safety decline is re-served by another model.
	betaHeader        = "server-side-fallback-2026-07-01"
	defaultMaxTokens  = 16000
	defaultMaxRetries = 2
)

// Message is one conversation turn (text only).
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Completion is one structured-output request to Claude.
type Completion struct {
	System    string          // cached as a single text block
	Messages  []Message       // user/assistant turns
	Schema    json.RawMessage // output_config.format json_schema
	MaxTokens int
}

// Usage is the token accounting of one call.
type Usage struct {
	InputTokens              int `json:"inputTokens"`
	OutputTokens             int `json:"outputTokens"`
	CacheReadInputTokens     int `json:"cacheReadInputTokens,omitempty"`
	CacheCreationInputTokens int `json:"cacheCreationInputTokens,omitempty"`
}

// Add accumulates u2 into u.
func (u *Usage) Add(u2 Usage) {
	u.InputTokens += u2.InputTokens
	u.OutputTokens += u2.OutputTokens
	u.CacheReadInputTokens += u2.CacheReadInputTokens
	u.CacheCreationInputTokens += u2.CacheCreationInputTokens
}

// StopDetails is populated only when StopReason == "refusal".
type StopDetails struct {
	Category    string `json:"category"`
	Explanation string `json:"explanation"`
}

// Response is the parsed Messages API reply.
type Response struct {
	Text        string
	StopReason  string
	StopDetails *StopDetails
	Model       string
	Usage       Usage
}

// Caller abstracts the Claude call so generate.go can be unit-tested.
type Caller interface {
	Complete(ctx context.Context, req Completion) (*Response, error)
}

// APIError is a non-2xx reply from Anthropic.
type APIError struct {
	Status  int
	Type    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("anthropic: %d %s: %s", e.Status, e.Type, e.Message)
}

// Retryable reports whether the request should be retried.
func (e *APIError) Retryable() bool {
	return e.Status == 408 || e.Status == 409 || e.Status == 429 || e.Status >= 500
}

// Client calls POST /v1/messages with structured output.
type Client struct {
	APIKey     string
	Model      string
	Effort     string // low|medium|high|xhigh|max
	BaseURL    string
	HTTP       *http.Client
	MaxRetries int
	// StructuredOutput sends Completion.Schema as output_config.format.
	// Off by default: the API's grammar compiler rejects schemas of DSL size
	// ("compiled grammar is too large" / "too many optional parameters"), so
	// the schema goes into the prompt instead and dsl.Parse stays the gate.
	StructuredOutput bool
}

// NewClient builds a client; apiKey is never logged.
func NewClient(apiKey, model, effort string) *Client {
	return &Client{
		APIKey:     apiKey,
		Model:      model,
		Effort:     effort,
		BaseURL:    defaultBaseURL,
		HTTP:       &http.Client{Timeout: 60 * time.Second},
		MaxRetries: defaultMaxRetries,
	}
}

type wireBlock struct {
	Type         string         `json:"type"`
	Text         string         `json:"text"`
	CacheControl map[string]any `json:"cache_control,omitempty"`
}

type wireRequest struct {
	Model        string         `json:"model"`
	MaxTokens    int            `json:"max_tokens"`
	System       []wireBlock    `json:"system,omitempty"`
	Messages     []Message      `json:"messages"`
	OutputConfig map[string]any `json:"output_config,omitempty"`
	Fallbacks    string         `json:"fallbacks,omitempty"`
}

type wireResponse struct {
	Model   string `json:"model"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason  string       `json:"stop_reason"`
	StopDetails *StopDetails `json:"stop_details"`
	Usage       struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Complete performs one structured-output call with retry on 429/5xx/network.
func (c *Client) Complete(ctx context.Context, req Completion) (*Response, error) {
	if c.APIKey == "" {
		return nil, errors.New("anthropic: ANTHROPIC_API_KEY is not set")
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}
	wr := wireRequest{
		Model:     c.Model,
		MaxTokens: maxTokens,
		Messages:  req.Messages,
		Fallbacks: "default",
	}
	if req.System != "" {
		wr.System = []wireBlock{{Type: "text", Text: req.System, CacheControl: map[string]any{"type": "ephemeral"}}}
	}
	oc := map[string]any{}
	if c.Effort != "" {
		oc["effort"] = c.Effort
	}
	if c.StructuredOutput && len(req.Schema) > 0 {
		oc["format"] = map[string]any{"type": "json_schema", "schema": req.Schema}
	}
	if len(oc) > 0 {
		wr.OutputConfig = oc
	}
	body, err := json.Marshal(wr)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(500*(1<<uint(attempt-1))) * time.Millisecond
			var ae *APIError
			if errors.As(lastErr, &ae) && ae.Status == 429 {
				delay = max(delay, 2*time.Second)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
		resp, err := c.once(ctx, body)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		var ae *APIError
		if errors.As(err, &ae) && !ae.Retryable() {
			return nil, err
		}
		if ctx.Err() != nil {
			return nil, err
		}
		log.Printf("anthropic: attempt %d failed: %v", attempt+1, redact(err))
	}
	return nil, lastErr
}

func (c *Client) once(ctx context.Context, body []byte) (*Response, error) {
	base := c.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("x-api-key", c.APIKey)
	hreq.Header.Set("anthropic-version", anthropicVersion)
	hreq.Header.Set("anthropic-beta", betaHeader)
	started := time.Now()
	hresp, err := c.HTTP.Do(hreq)
	if err != nil {
		return nil, &APIError{Status: 599, Type: "connection_error", Message: err.Error()}
	}
	defer func() { _ = hresp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(hresp.Body, 8<<20))
	if err != nil {
		return nil, &APIError{Status: 599, Type: "read_error", Message: err.Error()}
	}
	var wresp wireResponse
	if hresp.StatusCode >= 300 {
		_ = json.Unmarshal(data, &wresp)
		ae := &APIError{Status: hresp.StatusCode, Type: "http_error", Message: http.StatusText(hresp.StatusCode)}
		if wresp.Error != nil {
			ae.Type, ae.Message = wresp.Error.Type, wresp.Error.Message
		}
		if ra := hresp.Header.Get("retry-after"); ra != "" {
			if s, err := strconv.Atoi(ra); err == nil && s > 0 && s <= 30 {
				time.Sleep(time.Duration(s) * time.Second)
			}
		}
		return nil, ae
	}
	if err := json.Unmarshal(data, &wresp); err != nil {
		return nil, &APIError{Status: 599, Type: "decode_error", Message: err.Error()}
	}
	var sb strings.Builder
	for _, b := range wresp.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	out := &Response{Text: sb.String(), StopReason: wresp.StopReason, StopDetails: wresp.StopDetails, Model: wresp.Model}
	out.Usage = Usage{
		InputTokens:              wresp.Usage.InputTokens,
		OutputTokens:             wresp.Usage.OutputTokens,
		CacheReadInputTokens:     wresp.Usage.CacheReadInputTokens,
		CacheCreationInputTokens: wresp.Usage.CacheCreationInputTokens,
	}
	log.Printf("anthropic: model=%s stop=%s in=%d out=%d cache_read=%d %dms",
		out.Model, out.StopReason, out.Usage.InputTokens, out.Usage.OutputTokens, out.Usage.CacheReadInputTokens, time.Since(started).Milliseconds())
	return out, nil
}

// redact keeps error logs free of anything that could echo request content.
func redact(err error) string {
	var ae *APIError
	if errors.As(err, &ae) {
		return fmt.Sprintf("%d %s", ae.Status, ae.Type)
	}
	return "error"
}
