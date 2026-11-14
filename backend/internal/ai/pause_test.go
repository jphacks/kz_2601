package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// countingStore counts how often the limiter charges a request.
type countingStore struct {
	inner ratelimit.Store
	n     atomic.Int32

	refunds     atomic.Int32
	refundUnits atomic.Int32
	waived      atomic.Int32
	unchanged   atomic.Int32
}

func (s *countingStore) Consume(ctx context.Context, c []ratelimit.Counter, w int, exp time.Time) (ratelimit.Usage, error) {
	if len(c) > 0 && c[0].Scope == ratelimit.ScopeWaived {
		s.waived.Add(1) // the hourly count of waived requests, not a charge
	} else if len(c) > 0 && c[0].Scope == ratelimit.ScopeUnchanged {
		s.unchanged.Add(1) // the daily count of unchanged refunds, not a charge
	} else {
		s.n.Add(1)
	}
	return s.inner.Consume(ctx, c, w, exp)
}

func (s *countingStore) Refund(ctx context.Context, c []ratelimit.Counter, w int, exp time.Time) error {
	s.refunds.Add(1)
	s.refundUnits.Add(int32(w))
	return s.inner.Refund(ctx, c, w, exp)
}

// pausedStack mirrors cmd/ai: Pause in front of the rate limiter (#511).
func pausedStack(t *testing.T, paused bool, logs *bytes.Buffer) (http.Handler, *fakeCaller, *countingStore) {
	t.Helper()
	f := &fakeCaller{replies: []string{questionReply}}
	svc, _ := newSvc(f)
	svc.Verifier = auth.DevVerifier{}
	svc.AuthMode = auth.ModeDev
	store := &countingStore{inner: ratelimit.NewMemory()}
	limiter := ratelimit.New(ratelimit.Config{Limits: ratelimit.DefaultLimits, Weights: ratelimit.DefaultWeights}, store, nil)
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	h := Pause(paused, logger)(limiter.Middleware(auth.DevVerifier{})(NewHandler(svc)))
	return h, f, store
}

func serve(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer u-paused")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPause_ClaudeRoutesAnswer503WithoutClaudeOrQuota(t *testing.T) {
	routes := []struct{ path, body string }{
		{"/v1/generate", `{"appId":"a1","transcript":"家計簿を作って"}`},
		{"/v1/generate?stream=1", `{"appId":"a1","transcript":"家計簿を作って","mode":"edit"}`},
		{"/v1/chat", `{"appId":"a1","messages":[{"role":"user","text":"家計簿"}]}`},
		{"/v1/text", `{"instruction":"罰ゲームを 1 つ"}`},
	}
	for _, rt := range routes {
		t.Run(rt.path, func(t *testing.T) {
			var logs bytes.Buffer
			h, f, store := pausedStack(t, true, &logs)
			rec := serve(h, http.MethodPost, rt.path, rt.body)
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status %d, want 503 (%s)", rec.Code, rec.Body.String())
			}
			var body httpx.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error.Code != httpx.CodeAIPaused {
				t.Fatalf("body %s (err %v), want code ai_paused", rec.Body.String(), err)
			}
			if len(f.calls) != 0 {
				t.Fatalf("Claude called %d times while paused", len(f.calls))
			}
			if n := store.refunds.Load(); n != 0 {
				t.Fatalf("paused request refunded %d times; it was never charged", n)
			}
			if n := store.n.Load(); n != 0 {
				t.Fatalf("rate limit charged %d times while paused", n)
			}
			if !strings.Contains(logs.String(), `"msg":"ai.paused"`) || strings.Contains(logs.String(), "家計簿") || strings.Contains(logs.String(), "u-paused") {
				t.Fatalf("log: %s", logs.String())
			}
		})
	}
}

func TestPause_NonAIRoutesKeepWorking(t *testing.T) {
	var logs bytes.Buffer
	h, _, _ := pausedStack(t, true, &logs)
	for _, path := range []string{"/health", "/healthz"} {
		if rec := serve(h, http.MethodGet, path, ""); rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
	if rec := serve(h, http.MethodDelete, "/v1/chat/a1", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /v1/chat: %d %s", rec.Code, rec.Body.String())
	}
	// undo restores a version without Claude: not stopped (the empty store answers its own error)
	if rec := serve(h, http.MethodPost, "/v1/generate", `{"appId":"a1","mode":"undo"}`); rec.Code == http.StatusServiceUnavailable {
		t.Fatalf("undo stopped: %s", rec.Body.String())
	}
}

func TestPause_OffPassesThrough(t *testing.T) {
	var logs bytes.Buffer
	h, f, store := pausedStack(t, false, &logs)
	rec := serve(h, http.MethodPost, "/v1/chat", `{"appId":"a1","messages":[{"role":"user","text":"家計簿"}]}`)
	if rec.Code != http.StatusOK || len(f.calls) != 1 || store.n.Load() != 1 {
		t.Fatalf("status %d calls %d charged %d (%s)", rec.Code, len(f.calls), store.n.Load(), rec.Body.String())
	}
}

func TestPausedFromEnv(t *testing.T) {
	for v, want := range map[string]bool{"": false, "0": false, "off": false, "1": true, "true": true, "on": true} {
		if got := PausedFromEnv(func(string) string { return v }); got != want {
			t.Errorf("AI_PAUSED=%q: %v, want %v", v, got, want)
		}
	}
}
