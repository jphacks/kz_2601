package ratelimit

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
)

// waiving answers status after Waive (a request that is not charged).
func waiving(status int, got *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = Waive(r.Context())
		w.WriteHeader(status)
	})
}

// Waive gives every unit back before the response (no second refund at the
// end, even for a 5xx) and the headers show the count without the request.
func TestWaiveRefundsBeforeTheResponse(t *testing.T) {
	for _, status := range []int{http.StatusPaymentRequired, http.StatusBadRequest, http.StatusServiceUnavailable} {
		l, m := newRefundLimiter()
		serve(t, l.Middleware(auth.DevVerifier{})(answering(http.StatusOK, "")), "POST", "/v1/text", "u1", `{"task":"summarize"}`) // 2 used
		var waived bool
		w := serve(t, l.Middleware(auth.DevVerifier{})(waiving(status, &waived)), "POST", "/v1/text", "u1", `{"task":"summarize"}`)
		if !waived || w.Code != status {
			t.Fatalf("%d: waived %v status %d", status, waived, w.Code)
		}
		if u, _, g := counts(t, m, "u1", ""); u != 2 || g != 2 {
			t.Fatalf("%d: user=%d global=%d, want 2", status, u, g)
		}
		if got := w.Header().Get("X-RateLimit-Remaining"); got != "58" {
			t.Fatalf("%d: remaining %q, want 58", status, got)
		}
		if got := w.Header().Get("X-RateLimit-Limit"); got != "60" {
			t.Fatalf("%d: limit %q", status, got)
		}
	}
}

// Waives are capped per uid and hour: past WaivedHourly the request keeps its
// charge; the next hour waives again, and another uid has its own count.
func TestWaiveHourlyCap(t *testing.T) {
	c := &clock{t: time.Date(2026, 10, 25, 22, 10, 0, 0, JST)}
	m := NewMemory()
	l := New(Config{Limits: Limits{SignedIn: 60, Global: 2000, WaivedHourly: 2}, Weights: DefaultWeights, Env: "test", Now: c.now}, m, nil)
	h := func(got *bool) http.Handler {
		return l.Middleware(auth.DevVerifier{})(waiving(http.StatusPaymentRequired, got))
	}
	for i, want := range []bool{true, true, false} {
		var waived bool
		serve(t, h(&waived), "POST", "/v1/text", "u1", `{"task":"summarize"}`)
		if waived != want {
			t.Fatalf("#%d waived %v, want %v", i, waived, want)
		}
	}
	if u, _, _ := counts(t, m, "u1", ""); u != DefaultWeights.TextTask {
		t.Fatalf("user = %d, want %d (only the capped request is charged)", u, DefaultWeights.TextTask)
	}
	var waived bool
	if serve(t, h(&waived), "POST", "/v1/text", "u2", `{"task":"summarize"}`); !waived {
		t.Fatal("u2 has its own count")
	}
	c.t = c.t.Add(time.Hour)
	if serve(t, h(&waived), "POST", "/v1/text", "u1", `{"task":"summarize"}`); !waived {
		t.Fatal("next hour should waive again")
	}
}

// Outside the limiter and for an uncharged (weight-0) request Waive is a no-op.
func TestWaiveNoop(t *testing.T) {
	if Waive(context.Background()) {
		t.Fatal("waived outside the limiter")
	}
	l, m := newRefundLimiter()
	var waived bool
	w := serve(t, l.Middleware(auth.DevVerifier{})(waiving(http.StatusOK, &waived)), "POST", "/v1/generate", "u1", `{"mode":"undo"}`)
	if waived {
		t.Fatal("waived a weight-0 request")
	}
	if u, _, _ := counts(t, m, "u1", ""); u != 0 || w.Header().Get("X-RateLimit-Remaining") != "60" {
		t.Fatalf("user=%d remaining %q", u, w.Header().Get("X-RateLimit-Remaining"))
	}
}
