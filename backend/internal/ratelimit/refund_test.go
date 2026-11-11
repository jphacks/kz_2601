package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
)

// counts reads the user / ip / global counters of the test day from the store.
func counts(t *testing.T, m *Memory, uid, ip string) (user, perIP, global int) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	day := "2026-10-25"
	user = m.entries["test_"+day+"_uid_"+uid].count
	if ip != "" {
		perIP = m.entries["test_"+day+"_ip_"+ipKey(ip)].count
	}
	global = m.entries["test_"+day+"_global"].count
	return
}

func newRefundLimiter() (*Limiter, *Memory) {
	c := &clock{t: time.Date(2026, 10, 25, 23, 0, 0, 0, JST)}
	m := NewMemory()
	l := New(Config{Limits: Limits{Anonymous: 30, SignedIn: 60, PerIP: 300, Global: 2000}, Weights: DefaultWeights, Env: "test", Now: c.now}, m, nil)
	return l, m
}

// answering returns a handler that ends with status after reporting mode.
func answering(status int, mode string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mode != "" {
			ReportMode(r.Context(), mode)
		}
		w.WriteHeader(status)
	})
}

func TestRefundByStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   int // units left charged for a 3-unit create
	}{
		{"502 bad gateway refunds", http.StatusBadGateway, 0},
		{"503 unavailable refunds", http.StatusServiceUnavailable, 0},
		{"504 timeout refunds", http.StatusGatewayTimeout, 0},
		{"200 keeps the charge", http.StatusOK, 3},
		{"400 bad request keeps", http.StatusBadRequest, 3},
		{"422 invalid keeps", http.StatusUnprocessableEntity, 3},
		{"429 Claude rate limit keeps", http.StatusTooManyRequests, 3},
		{"500 internal keeps", http.StatusInternalServerError, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, m := newRefundLimiter()
			h := l.Middleware(auth.DevVerifier{})(answering(c.status, ""))
			if w := serve(t, h, "POST", "/v1/generate", "u1", `{"mode":"create"}`); w.Code != c.status {
				t.Fatalf("status %d", w.Code)
			}
			if u, _, g := counts(t, m, "u1", ""); u != c.want || g != c.want {
				t.Fatalf("user=%d global=%d, want %d", u, g, c.want)
			}
		})
	}
}

// chat / text failures come back the same way (writeErr in the handler).
func TestRefundChatAndText(t *testing.T) {
	l, m := newRefundLimiter()
	h := l.Middleware(auth.DevVerifier{})(answering(http.StatusGatewayTimeout, ""))
	serve(t, h, "POST", "/v1/chat", "u1", `{}`)
	serve(t, h, "POST", "/v1/text", "u1", `{}`)
	if u, _, g := counts(t, m, "u1", ""); u != 0 || g != 0 {
		t.Fatalf("user=%d global=%d", u, g)
	}
}

func TestRefundAutoResolvedMode(t *testing.T) {
	cases := []struct {
		name, body, mode string
		want             int
	}{
		{"auto resolved to edit pays edit", `{"mode":"auto"}`, "edit", DefaultWeights.Edit},
		{"no mode resolved to edit pays edit", `{}`, "edit", DefaultWeights.Edit},
		{"auto resolved to undo pays nothing", `{"mode":"auto"}`, "undo", 0},
		{"auto resolved to create pays create", `{"mode":"auto"}`, "create", DefaultWeights.Create},
		{"edit stays edit", `{"mode":"edit"}`, "edit", DefaultWeights.Edit},
		{"create never gets more back", `{"mode":"create"}`, "create", DefaultWeights.Create},
		{"no report keeps the charge", `{"mode":"auto"}`, "", DefaultWeights.Create},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, m := newRefundLimiter()
			h := l.Middleware(auth.DevVerifier{})(answering(http.StatusOK, c.mode))
			serve(t, h, "POST", "/v1/generate", "u1", c.body)
			if u, _, g := counts(t, m, "u1", ""); u != c.want || g != c.want {
				t.Fatalf("user=%d global=%d, want %d", u, g, c.want)
			}
		})
	}
}

// A mode report on a failed request does not matter: 4xx keeps all, 5xx refunds all.
func TestRefundModeIgnoredOnFailure(t *testing.T) {
	l, m := newRefundLimiter()
	h := l.Middleware(auth.DevVerifier{})(answering(http.StatusBadRequest, "edit"))
	serve(t, h, "POST", "/v1/generate", "u1", `{"mode":"auto"}`)
	if u, _, _ := counts(t, m, "u1", ""); u != 3 {
		t.Fatalf("user=%d, want 3", u)
	}
}

// SSE answers 200 and reports the error event's status through the context.
func TestRefundStreamReportedStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
		mode   string
		want   int
	}{
		{"error event 504 refunds", http.StatusGatewayTimeout, "", 0},
		{"error event 422 keeps", http.StatusUnprocessableEntity, "", 3},
		{"result event auto->edit", 0, "edit", 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, m := newRefundLimiter()
			h := l.Middleware(auth.DevVerifier{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush() // the wrapper keeps SSE flushing
				_, _ = w.Write([]byte("event: app\ndata: {}\n\n"))
				if c.status != 0 {
					ReportStatus(r.Context(), c.status)
				}
				if c.mode != "" {
					ReportMode(r.Context(), c.mode)
				}
			}))
			if w := serve(t, h, "POST", "/v1/generate?stream=1", "u1", `{"mode":"auto"}`); w.Code != 200 || !w.Flushed {
				t.Fatalf("status %d flushed=%v", w.Code, w.Flushed)
			}
			if u, _, g := counts(t, m, "u1", ""); u != c.want || g != c.want {
				t.Fatalf("user=%d global=%d, want %d", u, g, c.want)
			}
		})
	}
}

// A client that disconnects mid-stream is not refunded, even on a 5xx.
func TestNoRefundWhenClientGone(t *testing.T) {
	l, m := newRefundLimiter()
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest("POST", "/v1/generate", strings.NewReader(`{"mode":"create"}`)).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer u1")
	inner := l.Middleware(auth.DevVerifier{})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		cancel() // the client goes away while the handler runs
		w.WriteHeader(http.StatusGatewayTimeout)
	}))
	inner.ServeHTTP(httptest.NewRecorder(), r)
	if u, _, _ := counts(t, m, "u1", ""); u != 3 {
		t.Fatalf("user=%d, want 3", u)
	}
}

// Every counter of the request is refunded: user, ip (anonymous) and global.
func TestRefundAllCounters(t *testing.T) {
	l, m := newRefundLimiter()
	h := l.Middleware(auth.DevVerifier{})(answering(http.StatusOK, ""))
	serve(t, h, "POST", "/v1/generate", "other", `{"mode":"create"}`) // global = 3 from someone else
	anonID := auth.Identity{UID: "anon-1", Anonymous: true}
	dec := mustAllow(t, l, anonID, "203.0.113.7", 3)
	if u, ip, g := counts(t, m, "anon-1", "203.0.113.7"); u != 3 || ip != 3 || g != 6 {
		t.Fatalf("before: user=%d ip=%d global=%d", u, ip, g)
	}
	if err := l.Refund(context.Background(), dec, 3); err != nil {
		t.Fatal(err)
	}
	if u, ip, g := counts(t, m, "anon-1", "203.0.113.7"); u != 0 || ip != 0 || g != 3 {
		t.Fatalf("after: user=%d ip=%d global=%d", u, ip, g)
	}
	// more than charged is capped at the charge
	dec = mustAllow(t, l, anonID, "203.0.113.7", 1)
	if err := l.Refund(context.Background(), dec, 10); err != nil {
		t.Fatal(err)
	}
	if u, _, g := counts(t, m, "anon-1", "203.0.113.7"); u != 0 || g != 3 {
		t.Fatalf("capped: user=%d global=%d", u, g)
	}
}

func TestMemoryRefundNeverBelowZero(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	exp := time.Now().Add(time.Hour)
	cs := []Counter{{ID: "u", Scope: ScopeUser, Limit: 10}, {ID: "g", Scope: ScopeGlobal, Limit: 10}}
	if _, err := m.Consume(ctx, cs, 2, exp); err != nil {
		t.Fatal(err)
	}
	later := exp.Add(time.Hour)
	if err := m.Refund(ctx, cs, 5, later); err != nil {
		t.Fatal(err)
	}
	if e := m.entries["u"]; e.count != 0 || !e.expireAt.Equal(later) {
		t.Fatalf("u: %+v", e)
	}
	// a counter that was never charged (or expired) is not created
	if err := m.Refund(ctx, []Counter{{ID: "missing"}}, 1, later); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.entries["missing"]; ok {
		t.Fatal("refund must not create a counter")
	}
}

// When Firestore failed, the charge went to the memory fallback: refund it there.
func TestRefundGoesToTheStoreThatCharged(t *testing.T) {
	l := New(Config{Limits: Limits{SignedIn: 3}}, failingStore{}, nil)
	ctx := context.Background()
	dec, err := l.Allow(ctx, signed, "", 3)
	if err != nil || !dec.Allowed {
		t.Fatalf("%+v %v", dec, err)
	}
	if err := l.Refund(ctx, dec, 3); err != nil {
		t.Fatal(err)
	}
	if d, _ := l.Allow(ctx, signed, "", 3); !d.Allowed {
		t.Fatalf("after refund: %+v", d)
	}
}

// Undo / not-limited routes are never charged, so nothing is refunded.
func TestNoRefundWhenNothingCharged(t *testing.T) {
	l, m := newRefundLimiter()
	h := l.Middleware(auth.DevVerifier{})(answering(http.StatusBadGateway, ""))
	serve(t, h, "POST", "/v1/generate", "u1", `{"mode":"undo"}`)
	if u, _, g := counts(t, m, "u1", ""); u != 0 || g != 0 {
		t.Fatalf("user=%d global=%d", u, g)
	}
	if err := l.Refund(context.Background(), Decision{}, 3); err != nil {
		t.Fatal(err)
	}
}

// The handler sees the verified identity, anonymous too (#630: an anonymous
// account gets no initial coins).
func TestMiddlewareKeepsAnonymous(t *testing.T) {
	l, _ := newRefundLimiter()
	for uid, want := range map[string]bool{"anon-1": true, "u1": false} {
		var got, seen bool
		h := l.Middleware(auth.DevVerifier{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got, seen = auth.Anonymous(r.Context()), auth.UID(r.Context()) == uid
			w.WriteHeader(http.StatusOK)
		}))
		serve(t, h, "POST", "/v1/chat", uid, `{}`)
		if !seen || got != want {
			t.Errorf("%s: anonymous %v (seen %v), want %v", uid, got, seen, want)
		}
	}
}
