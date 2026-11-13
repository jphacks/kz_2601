package triage

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/ai"
)

// fakeMessages is a stand-in for POST /v1/messages that records the request.
type fakeMessages struct {
	status int
	body   string
	got    map[string]any
	calls  int
}

func (f *fakeMessages) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.calls++
	if r.URL.Path != "/v1/messages" {
		http.NotFound(w, r)
		return
	}
	f.got = nil
	_ = json.NewDecoder(r.Body).Decode(&f.got)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(f.status)
	_, _ = io.WriteString(w, f.body)
}

func fakeClient(t *testing.T, f *fakeMessages) *ai.Client {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c := ai.NewClient("test", "claude-test", "")
	c.BaseURL = srv.URL
	c.HTTP = srv.Client()
	c.MaxRetries = 0
	c.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	return c
}

const okReply = `{"model":"claude-test","stop_reason":"end_turn",
"content":[{"type":"text","text":"{\"verdict\":\"keep\"}"}],
"usage":{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":30,"cache_creation_input_tokens":7}}`

func TestAIModelComplete(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"verdict":{"type":"string"}}}`)

	t.Run("reply and usage", func(t *testing.T) {
		f := &fakeMessages{status: http.StatusOK, body: okReply}
		text, u, err := AIModel{C: fakeClient(t, f)}.Complete(context.Background(), "sys", "hello", schema)
		if err != nil {
			t.Fatal(err)
		}
		if text != `{"verdict":"keep"}` {
			t.Errorf("text = %q", text)
		}
		// cache reads and writes count as input
		if u != (Usage{InputTokens: 137, OutputTokens: 20}) {
			t.Errorf("usage = %+v", u)
		}
		if f.got["max_tokens"] != float64(4000) {
			t.Errorf("max_tokens = %v, want 4000 for MaxTokens 0", f.got["max_tokens"])
		}
		msgs, _ := f.got["messages"].([]any)
		if len(msgs) != 1 {
			t.Fatalf("messages = %v", f.got["messages"])
		}
		if m, _ := msgs[0].(map[string]any); m["role"] != "user" || m["content"] != "hello" {
			t.Errorf("message = %v", msgs[0])
		}
		sys, _ := f.got["system"].([]any)
		if len(sys) != 1 || sys[0].(map[string]any)["text"] != "sys" {
			t.Errorf("system = %v", f.got["system"])
		}
		oc, _ := f.got["output_config"].(map[string]any)
		format, _ := oc["format"].(map[string]any)
		if format["type"] != "json_schema" || format["schema"] == nil {
			t.Errorf("output_config = %v, want the schema as structured output", f.got["output_config"])
		}
		if _, ok := f.got["tools"]; ok {
			t.Error("tools were sent")
		}
	})

	t.Run("MaxTokens is sent when set", func(t *testing.T) {
		f := &fakeMessages{status: http.StatusOK, body: okReply}
		if _, _, err := (AIModel{C: fakeClient(t, f), MaxTokens: 1234}).Complete(context.Background(), "s", "u", schema); err != nil {
			t.Fatal(err)
		}
		if f.got["max_tokens"] != float64(1234) {
			t.Errorf("max_tokens = %v, want 1234", f.got["max_tokens"])
		}
	})

	t.Run("negative MaxTokens falls back to 4000", func(t *testing.T) {
		f := &fakeMessages{status: http.StatusOK, body: okReply}
		if _, _, err := (AIModel{C: fakeClient(t, f), MaxTokens: -1}).Complete(context.Background(), "s", "u", schema); err != nil {
			t.Fatal(err)
		}
		if f.got["max_tokens"] != float64(4000) {
			t.Errorf("max_tokens = %v, want 4000", f.got["max_tokens"])
		}
	})

	t.Run("API error is returned as is", func(t *testing.T) {
		f := &fakeMessages{status: http.StatusBadRequest,
			body: `{"type":"error","error":{"type":"invalid_request_error","message":"bad schema"}}`}
		text, u, err := AIModel{C: fakeClient(t, f)}.Complete(context.Background(), "s", "u", schema)
		var ae *ai.APIError
		if !errors.As(err, &ae) || ae.Status != http.StatusBadRequest || ae.Type != "invalid_request_error" || ae.Message != "bad schema" {
			t.Fatalf("err = %#v", err)
		}
		if text != "" || u != (Usage{}) {
			t.Errorf("got %q %+v with an error", text, u)
		}
		if f.calls != 1 {
			t.Errorf("calls = %d, want 1 (a 400 is not retried)", f.calls)
		}
	})
}
