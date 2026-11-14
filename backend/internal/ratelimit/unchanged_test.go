package ratelimit

import (
	"net/http"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
)

func newUnchangedLimiter(capPerDay int) (*Limiter, *Memory) {
	c := &clock{t: time.Date(2026, 10, 25, 23, 0, 0, 0, JST)}
	m := NewMemory()
	l := New(Config{Limits: Limits{SignedIn: 60, Global: 2000, UnchangedDaily: capPerDay}, Weights: DefaultWeights, Env: "test", Now: c.now}, m, nil)
	return l, m
}

// unchangedAnswer ends with status after reporting mode and, reports times, unchanged.
func unchangedAnswer(status int, mode string, reports int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ReportMode(r.Context(), mode)
		for range reports {
			ReportUnchanged(r.Context())
		}
		w.WriteHeader(status)
	})
}

func unchangedCount(m *Memory, uid string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.entries["test_2026-10-25_unchanged_"+uid].count
}

// An edit that changed nothing pays nothing, and the next response's
// X-RateLimit-Remaining shows it.
func TestRefundUnchangedEdit(t *testing.T) {
	for _, body := range []string{`{"mode":"edit"}`, `{"mode":"auto"}`} {
		l, m := newUnchangedLimiter(5)
		serve(t, l.Middleware(auth.DevVerifier{})(unchangedAnswer(http.StatusOK, "edit", 1)), "POST", "/v1/generate", "u1", body)
		if u, _, g := counts(t, m, "u1", ""); u != 0 || g != 0 {
			t.Fatalf("%s: user=%d global=%d, want 0", body, u, g)
		}
		w := serve(t, l.Middleware(auth.DevVerifier{})(answering(http.StatusOK, "")), "POST", "/v1/chat", "u1", `{}`)
		if got := w.Header().Get("X-RateLimit-Remaining"); got != "59" {
			t.Fatalf("%s: next remaining %q, want 59", body, got)
		}
	}
}

// Reporting unchanged twice in one request refunds once and counts once.
func TestRefundUnchangedOncePerRequest(t *testing.T) {
	l, m := newUnchangedLimiter(5)
	serve(t, l.Middleware(auth.DevVerifier{})(answering(http.StatusOK, "")), "POST", "/v1/chat", "u1", `{}`) // 1 used
	serve(t, l.Middleware(auth.DevVerifier{})(unchangedAnswer(http.StatusOK, "edit", 2)), "POST", "/v1/generate", "u1", `{"mode":"edit"}`)
	if u, _, _ := counts(t, m, "u1", ""); u != 1 {
		t.Fatalf("user=%d, want 1 (the chat only)", u)
	}
	if n := unchangedCount(m, "u1"); n != 1 {
		t.Fatalf("unchanged count %d, want 1", n)
	}
}

// Past UnchangedDaily the edit keeps its charge; other users have their own count.
func TestRefundUnchangedDailyCap(t *testing.T) {
	l, m := newUnchangedLimiter(2)
	h := l.Middleware(auth.DevVerifier{})(unchangedAnswer(http.StatusOK, "edit", 1))
	for range 3 {
		serve(t, h, "POST", "/v1/generate", "u1", `{"mode":"edit"}`)
	}
	if u, _, _ := counts(t, m, "u1", ""); u != DefaultWeights.Edit {
		t.Fatalf("user=%d, want %d (only the 3rd is charged)", u, DefaultWeights.Edit)
	}
	if n := unchangedCount(m, "u1"); n != 2 {
		t.Fatalf("unchanged count %d, want 2", n)
	}
	// past the cap, mode=auto still pays only the edit
	serve(t, h, "POST", "/v1/generate", "u1", `{"mode":"auto"}`)
	if u, _, _ := counts(t, m, "u1", ""); u != 2*DefaultWeights.Edit {
		t.Fatalf("user=%d, want %d", u, 2*DefaultWeights.Edit)
	}
	serve(t, h, "POST", "/v1/generate", "u2", `{"mode":"edit"}`)
	if u, _, _ := counts(t, m, "u2", ""); u != 0 {
		t.Fatalf("u2=%d, want 0", u)
	}
}

// Cap 0 turns the refund off; a failed request reporting unchanged follows the status rules.
func TestRefundUnchangedOffAndFailures(t *testing.T) {
	l, m := newUnchangedLimiter(0)
	serve(t, l.Middleware(auth.DevVerifier{})(unchangedAnswer(http.StatusOK, "edit", 1)), "POST", "/v1/generate", "u1", `{"mode":"edit"}`)
	if u, _, _ := counts(t, m, "u1", ""); u != DefaultWeights.Edit {
		t.Fatalf("off: user=%d, want %d", u, DefaultWeights.Edit)
	}
	l, m = newUnchangedLimiter(5)
	serve(t, l.Middleware(auth.DevVerifier{})(unchangedAnswer(http.StatusBadRequest, "edit", 1)), "POST", "/v1/generate", "u1", `{"mode":"edit"}`)
	if u, _, _ := counts(t, m, "u1", ""); u != DefaultWeights.Edit || unchangedCount(m, "u1") != 0 {
		t.Fatalf("400: user=%d unchanged=%d", u, unchangedCount(m, "u1"))
	}
}

func TestConfigUnchangedDaily(t *testing.T) {
	cfg, err := ConfigFromEnv(func(k string) string {
		if k == "RATE_LIMIT_UNCHANGED_DAILY" {
			return "3"
		}
		return ""
	})
	if err != nil || cfg.Limits.UnchangedDaily != 3 {
		t.Fatalf("cfg %+v err %v", cfg.Limits, err)
	}
	if DefaultLimits.UnchangedDaily != 5 {
		t.Fatalf("default %d", DefaultLimits.UnchangedDaily)
	}
}
