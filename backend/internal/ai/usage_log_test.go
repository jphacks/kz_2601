package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
)

const (
	secretPrompt = "私の家計簿 SECRET-PROMPT 口座番号 1234"
	secretReply  = "SECRET-REPLY 家計簿アプリ"
	secretSystem = "SECRET-SYSTEM rules"
	rawUID       = "uid-RAW-abc123"
)

// fakeAnthropic answers each request with the next reply (status, body) and
// repeats the last one.
func fakeAnthropic(t *testing.T, replies ...[2]string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		i := int(n.Add(1)) - 1
		rep := replies[min(i, len(replies)-1)]
		if strings.HasPrefix(rep[1], "event:") {
			w.Header().Set("Content-Type", "text/event-stream")
		}
		switch rep[0] {
		case "500":
			w.WriteHeader(http.StatusInternalServerError)
		case "400":
			w.WriteHeader(http.StatusBadRequest)
		}
		_, _ = io.WriteString(w, rep[1])
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func okBody(model string) string {
	b, _ := json.Marshal(map[string]any{
		"model":       model,
		"content":     []map[string]any{{"type": "text", "text": secretReply}},
		"stop_reason": "end_turn",
		"usage": map[string]any{
			"input_tokens": 1000, "output_tokens": 500,
			"cache_creation_input_tokens": 2000, "cache_read_input_tokens": 4000,
		},
	})
	return string(b)
}

func testClient(srv *httptest.Server, model string) (*Client, *bytes.Buffer) {
	var buf bytes.Buffer
	c := NewClient("test-key", model, "low")
	c.BaseURL = srv.URL
	c.Logger = slog.New(slog.NewJSONHandler(&buf, nil))
	return c, &buf
}

func usageLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("not JSON: %s", line)
		}
		if m["msg"] == UsageEvent {
			out = append(out, m)
		}
	}
	return out
}

func assertNoUserText(t *testing.T, buf *bytes.Buffer) {
	t.Helper()
	for _, s := range []string{"SECRET", "家計簿", "口座", rawUID, "test-key"} {
		if strings.Contains(buf.String(), s) {
			t.Fatalf("usage log leaks %q: %s", s, buf.String())
		}
	}
}

func userCompletion() Completion {
	return Completion{System: secretSystem, Messages: []Message{{Role: "user", Content: secretPrompt}}}
}

func TestUsageLog_SuccessFields(t *testing.T) {
	srv, _ := fakeAnthropic(t, [2]string{"200", okBody("claude-sonnet-5-5")})
	c, buf := testClient(srv, "claude-sonnet-5-5")
	ctx := WithOperation(auth.ContextWithUID(context.Background(), rawUID), OpPatch)
	if _, err := c.Complete(ctx, userCompletion()); err != nil {
		t.Fatal(err)
	}
	lines := usageLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("want 1 ai.usage line, got %d: %s", len(lines), buf)
	}
	m := lines[0]
	want := map[string]any{
		"event": "ai.usage", "operation": "patch", "model": "claude-sonnet-5-5",
		"response_model": "claude-sonnet-5-5", "stream": false, "attempt": 1.0,
		"outcome": "ok", "stop_reason": "end_turn",
		"input_tokens": 1000.0, "output_tokens": 500.0,
		"cache_creation_input_tokens": 2000.0, "cache_read_input_tokens": 4000.0,
		"uid_hash": UIDHash(rawUID), "pricing_as_of": PricingAsOf,
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %#v, want %#v", k, m[k], v)
		}
	}
	if _, ok := m["latency_ms"].(float64); !ok {
		t.Errorf("latency_ms missing: %v", m)
	}
	if _, ok := m["error_kind"]; ok {
		t.Errorf("error_kind on success: %v", m)
	}
	// 1000*2 + 500*10 + 2000*2.5 + 4000*0.10 = 12400 USD/MTok-tokens
	if got := m["est_cost_usd"].(float64); math.Abs(got-0.0124) > 1e-12 {
		t.Errorf("est_cost_usd = %v, want 0.0124", got)
	}
	assertNoUserText(t, buf)
}

func TestUsageLog_RetryAttemptsAndErrorKind(t *testing.T) {
	srv, n := fakeAnthropic(t,
		[2]string{"500", `{"type":"error","error":{"type":"api_error","message":"SECRET-REPLY echo"}}`},
		[2]string{"200", okBody("claude-sonnet-5-5")})
	c, buf := testClient(srv, "claude-sonnet-5-5")
	if _, err := c.Complete(context.Background(), userCompletion()); err != nil {
		t.Fatal(err)
	}
	lines := usageLines(t, buf)
	if n.Load() != 2 || len(lines) != 2 {
		t.Fatalf("calls=%d lines=%d", n.Load(), len(lines))
	}
	first, second := lines[0], lines[1]
	if first["attempt"] != 1.0 || first["outcome"] != "error" || first["error_kind"] != "500_api_error" || first["input_tokens"] != 0.0 {
		t.Errorf("first attempt: %v", first)
	}
	if first["operation"] != OpUnknown {
		t.Errorf("unlabeled call operation = %v", first["operation"])
	}
	if _, ok := first["uid_hash"]; ok {
		t.Errorf("anonymous call must not carry uid_hash: %v", first)
	}
	if second["attempt"] != 2.0 || second["outcome"] != "ok" {
		t.Errorf("second attempt: %v", second)
	}
	assertNoUserText(t, buf)
}

func TestUsageLog_UnknownModelHasNoCost(t *testing.T) {
	srv, _ := fakeAnthropic(t, [2]string{"200", okBody("claude-unknown-9")})
	c, buf := testClient(srv, "claude-unknown-9")
	if _, err := c.Complete(context.Background(), userCompletion()); err != nil {
		t.Fatal(err)
	}
	m := usageLines(t, buf)[0]
	if _, ok := m["est_cost_usd"]; ok {
		t.Fatalf("unknown model must log no cost: %v", m)
	}
	if m["input_tokens"] != 1000.0 {
		t.Fatalf("tokens still logged: %v", m)
	}
}

func TestUsageLog_StreamErrorKeepsBilledInput(t *testing.T) {
	stream := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-sonnet-5-5\",\"usage\":{\"input_tokens\":700,\"output_tokens\":1}}}\n\n" +
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\"}}\n\n" +
		"event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"invalid_request_error\",\"message\":\"SECRET-REPLY\"}}\n\n"
	srv, _ := fakeAnthropic(t, [2]string{"200", stream})
	c, buf := testClient(srv, "claude-sonnet-5-5")
	c.MaxRetries = 0
	req := userCompletion()
	req.OnText = func(string) {}
	ctx := WithOperation(context.Background(), OpChat)
	if _, err := c.Complete(ctx, req); err == nil {
		t.Fatal("want stream error")
	}
	m := usageLines(t, buf)[0]
	if m["outcome"] != "error" || m["error_kind"] != "500_stream_invalid_request_error" || m["input_tokens"] != 700.0 || m["stream"] != true || m["operation"] != "chat" {
		t.Fatalf("stream error line: %v", m)
	}
	if _, ok := m["est_cost_usd"]; !ok {
		t.Fatalf("billed partial usage should be priced: %v", m)
	}
	assertNoUserText(t, buf)
}

func TestUsageLog_RefusalOutcome(t *testing.T) {
	body := strings.Replace(okBody("claude-sonnet-5-5"), `"end_turn"`, `"refusal"`, 1)
	srv, _ := fakeAnthropic(t, [2]string{"200", body})
	c, buf := testClient(srv, "claude-sonnet-5-5")
	if _, err := c.Complete(context.Background(), userCompletion()); err != nil {
		t.Fatal(err)
	}
	if m := usageLines(t, buf)[0]; m["outcome"] != "refusal" {
		t.Fatalf("outcome = %v", m["outcome"])
	}
}

func TestUIDHash(t *testing.T) {
	if UIDHash("") != "" {
		t.Fatal("anonymous must hash to empty")
	}
	h := UIDHash(rawUID)
	if len(h) != 16 || strings.Contains(h, rawUID) || h != UIDHash(rawUID) || h == UIDHash(rawUID+"x") {
		t.Fatalf("bad hash %q", h)
	}
}

func TestEstimateCostUSD_TableRowsArePositive(t *testing.T) {
	for model, p := range modelPrices {
		if p.Input <= 0 || p.Output <= 0 || p.CacheWrite5m <= 0 || p.CacheRead <= 0 {
			t.Errorf("%s: incomplete price %+v", model, p)
		}
	}
	// the default models of cmd/ai must be priced
	for _, m := range []string{"claude-sonnet-5-5", DefaultChatModel} {
		if _, ok := EstimateCostUSD(m, Usage{InputTokens: 1}); !ok {
			t.Errorf("default model %s has no price", m)
		}
	}
}
