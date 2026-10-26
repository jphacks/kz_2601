package ratelimit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newTestLimiter(lim Limits) (*Limiter, *clock) {
	c := &clock{t: time.Date(2026, 10, 25, 23, 0, 0, 0, JST)}
	return New(Config{Limits: lim, Weights: DefaultWeights, Env: "test", Now: c.now}, NewMemory(), nil), c
}

var (
	anon   = auth.Identity{UID: "anon-1", Anonymous: true}
	signed = auth.Identity{UID: "apple-1"}
)

func mustAllow(t *testing.T, l *Limiter, id auth.Identity, ip string, w int) Decision {
	t.Helper()
	d, err := l.Allow(context.Background(), id, ip, w)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestAllowCountsUpToLimit(t *testing.T) {
	l, _ := newTestLimiter(Limits{SignedIn: 5})
	for i, want := range []int{2, 4} {
		d := mustAllow(t, l, signed, "", 2)
		if !d.Allowed || d.Used != want || d.Remaining != 5-want {
			t.Fatalf("call %d: %+v", i, d)
		}
	}
	// 4 + 2 > 5: blocked and nothing is charged; a 1-unit request still fits.
	if d := mustAllow(t, l, signed, "", 2); d.Allowed || d.Scope != ScopeUser || d.Used != 4 {
		t.Fatalf("over limit: %+v", d)
	}
	if d := mustAllow(t, l, signed, "", 1); !d.Allowed || d.Remaining != 0 {
		t.Fatalf("exact fit: %+v", d)
	}
	if d := mustAllow(t, l, signed, "", 1); d.Allowed {
		t.Fatalf("after exact fit: %+v", d)
	}
}

func TestZeroWeightIsFree(t *testing.T) {
	l, _ := newTestLimiter(Limits{SignedIn: 1})
	mustAllow(t, l, signed, "", 1)
	if d := mustAllow(t, l, signed, "", 0); !d.Allowed {
		t.Fatalf("undo must not be limited: %+v", d)
	}
}

func TestDayRollsOverAtMidnightJST(t *testing.T) {
	l, c := newTestLimiter(Limits{SignedIn: 3})
	mustAllow(t, l, signed, "", 3)
	d := mustAllow(t, l, signed, "", 1)
	if d.Allowed {
		t.Fatal("expected blocked")
	}
	if want := time.Date(2026, 10, 26, 0, 0, 0, 0, JST); !d.ResetAt.Equal(want) {
		t.Fatalf("resetAt %v, want %v", d.ResetAt, want)
	}
	c.t = c.t.Add(59 * time.Minute) // 23:59 JST, same day
	if mustAllow(t, l, signed, "", 1).Allowed {
		t.Fatal("still the same JST day")
	}
	c.t = c.t.Add(time.Minute) // 00:00 JST
	if d := mustAllow(t, l, signed, "", 1); !d.Allowed || d.Used != 1 {
		t.Fatalf("new day: %+v", d)
	}
}

func TestAnonymousIsStricterThanSignedIn(t *testing.T) {
	l, _ := newTestLimiter(Limits{Anonymous: 3, SignedIn: 9})
	if d := mustAllow(t, l, anon, "", 3); !d.Allowed || d.Tier != TierAnonymous || d.Limit != 3 {
		t.Fatalf("anon: %+v", d)
	}
	if d := mustAllow(t, l, anon, "", 1); d.Allowed {
		t.Fatalf("anon over: %+v", d)
	}
	if d := mustAllow(t, l, signed, "", 9); !d.Allowed || d.Tier != TierSignedIn || d.Limit != 9 {
		t.Fatalf("signed in: %+v", d)
	}
}

type proAll struct{}

func (proAll) IsPro(context.Context, string) bool { return true }

func TestProTierHook(t *testing.T) {
	l := New(Config{Limits: Limits{SignedIn: 1, Pro: 10}}, NewMemory(), proAll{})
	d, _ := l.Allow(context.Background(), signed, "", 5)
	if !d.Allowed || d.Tier != TierPro || d.Remaining != 5 {
		t.Fatalf("pro: %+v", d)
	}
	// anonymous never gets Pro limits (no receipt can belong to a throwaway account)
	if d, _ := l.Allow(context.Background(), anon, "", 1); d.Tier != TierAnonymous {
		t.Fatalf("anon pro: %+v", d)
	}
}

func TestPerIPCatchesRecreatedAnonymousAccounts(t *testing.T) {
	l, _ := newTestLimiter(Limits{Anonymous: 3, SignedIn: 30, PerIP: 5})
	for i := range 2 {
		id := auth.Identity{UID: "anon-" + string(rune('a'+i)), Anonymous: true}
		mustAllow(t, l, id, "203.0.113.7", 2)
	}
	fresh := auth.Identity{UID: "anon-z", Anonymous: true}
	if d := mustAllow(t, l, fresh, "203.0.113.7", 2); d.Allowed || d.Scope != ScopeIP {
		t.Fatalf("fresh anon on same IP: %+v", d)
	}
	if d := mustAllow(t, l, fresh, "198.51.100.1", 2); !d.Allowed {
		t.Fatalf("other IP: %+v", d)
	}
	// signed-in users on the same venue Wi-Fi are not counted per IP
	if d := mustAllow(t, l, signed, "203.0.113.7", 2); !d.Allowed {
		t.Fatalf("signed in on same IP: %+v", d)
	}
}

func TestIPv6GroupedBy64(t *testing.T) {
	if ipKey("2001:db8:1:2:aaaa::1") != ipKey("2001:db8:1:2:bbbb::9") {
		t.Fatal("same /64 must share a key")
	}
	if ipKey("2001:db8:1:2::1") == ipKey("2001:db8:1:3::1") {
		t.Fatal("different /64 must differ")
	}
}

func TestGlobalBreaker(t *testing.T) {
	l, _ := newTestLimiter(Limits{SignedIn: 100, Global: 5})
	mustAllow(t, l, signed, "", 3)
	other := auth.Identity{UID: "apple-2"}
	if d := mustAllow(t, l, other, "", 3); d.Allowed || d.Scope != ScopeGlobal || d.Limit != 5 {
		t.Fatalf("global: %+v", d)
	}
	// blocked requests must not leak into the user's own counter
	if d := mustAllow(t, l, other, "", 2); !d.Allowed || d.Used != 2 {
		t.Fatalf("after global block: %+v", d)
	}
}

type failingStore struct{}

func (failingStore) Consume(context.Context, []Counter, int, time.Time) (Usage, error) {
	return Usage{}, errors.New("firestore down")
}

func TestStoreFailureFallsBackToMemory(t *testing.T) {
	l := New(Config{Limits: Limits{SignedIn: 2}}, failingStore{}, nil)
	ctx := context.Background()
	if d, err := l.Allow(ctx, signed, "", 2); err != nil || !d.Allowed {
		t.Fatalf("first: %+v %v", d, err)
	}
	if d, _ := l.Allow(ctx, signed, "", 1); d.Allowed {
		t.Fatal("fallback must still limit")
	}
}

func TestConfigFromEnv(t *testing.T) {
	env := map[string]string{"RATE_LIMIT_ANON_DAILY": "7", "RATE_LIMIT_GLOBAL_DAILY": "0", "APP_ENV": "stg"}
	cfg, err := ConfigFromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Limits.Anonymous != 7 || cfg.Limits.Global != 0 || cfg.Limits.SignedIn != DefaultLimits.SignedIn || cfg.Env != "stg" {
		t.Fatalf("cfg: %+v", cfg)
	}
	env["RATE_LIMIT_USER_DAILY"] = "lots"
	if _, err := ConfigFromEnv(func(k string) string { return env[k] }); err == nil {
		t.Fatal("expected error for non-integer")
	}
}

// --- middleware ---

func serve(t *testing.T, h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	r.Header.Set("X-Forwarded-For", "10.0.0.1, 203.0.113.7")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestMiddleware429Shape(t *testing.T) {
	l, _ := newTestLimiter(Limits{Anonymous: 3, SignedIn: 10})
	var gotBody string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	})
	h := l.Middleware(auth.DevVerifier{})(next)

	body := `{"appId":"a1","transcript":"家計簿","mode":"create"}`
	if w := serve(t, h, "POST", "/v1/generate", "anon-1", body); w.Code != 200 || gotBody != body {
		t.Fatalf("first: %d body=%q", w.Code, gotBody)
	} else if w.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("remaining header: %q", w.Header().Get("X-RateLimit-Remaining"))
	}
	w := serve(t, h, "POST", "/v1/chat", "anon-1", `{"appId":"a1","messages":[]}`)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", w.Code)
	}
	if ra := w.Header().Get("Retry-After"); ra != "3600" {
		t.Fatalf("Retry-After %q", ra)
	}
	var env struct {
		Error struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	e := env.Error
	if e.Code != "rate_limited" || !strings.Contains(e.Message, "上限") || !strings.Contains(e.Message, "サインイン") {
		t.Fatalf("error: %+v", e)
	}
	if e.Details["scope"] != "user" || e.Details["tier"] != "anonymous" || e.Details["limit"] != float64(3) ||
		e.Details["remaining"] != float64(0) || e.Details["resetAt"] != "2026-10-25T15:00:00Z" || e.Details["retryAfterSec"] != float64(3600) {
		t.Fatalf("details: %+v", e.Details)
	}
}

func TestMiddlewareAuthAndPassThrough(t *testing.T) {
	l, _ := newTestLimiter(Limits{SignedIn: 1})
	hits := 0
	h := l.Middleware(auth.DevVerifier{})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits++ }))
	if w := serve(t, h, "POST", "/v1/chat", "", `{}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", w.Code)
	}
	if w := serve(t, h, "POST", "/v1/chat", "bad token!", `{}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("bad token: %d", w.Code)
	}
	for _, p := range [][2]string{{"GET", "/health"}, {"DELETE", "/v1/chat/a1"}} {
		if w := serve(t, h, p[0], p[1], "", ""); w.Code != 200 {
			t.Fatalf("%s %s: %d", p[0], p[1], w.Code)
		}
	}
	// undo costs nothing, so it runs even after the limit is used up
	serve(t, h, "POST", "/v1/chat", "u1", `{}`)
	if w := serve(t, h, "POST", "/v1/generate", "u1", `{"mode":"undo"}`); w.Code != 200 {
		t.Fatalf("undo: %d", w.Code)
	}
	if hits != 4 {
		t.Fatalf("hits %d", hits)
	}
}

func TestWeights(t *testing.T) {
	w := DefaultWeights
	cases := []struct {
		method, path, body string
		want               int
	}{
		{"POST", "/v1/generate", `{"mode":"create"}`, 3},
		{"POST", "/v1/generate", `{"mode":"auto"}`, 3},
		{"POST", "/v1/generate", `{}`, 3},
		{"POST", "/v1/generate", `{"mode":"edit"}`, 2},
		{"POST", "/v1/generate", `{"mode":"undo"}`, 0},
		{"POST", "/v1/chat", `{}`, 1},
		{"DELETE", "/v1/chat/x", ``, -1},
		{"GET", "/health", ``, -1},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, c.path, nil)
		if got := w.Weight(r, []byte(c.body)); got != c.want {
			t.Errorf("%s %s %s = %d, want %d", c.method, c.path, c.body, got, c.want)
		}
	}
}

func TestClientIPUsesLastForwardedHop(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/chat", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.7")
	if got := ClientIP(r); got != "203.0.113.7" {
		t.Fatalf("got %q", got)
	}
	r.Header.Del("X-Forwarded-For")
	r.RemoteAddr = "198.51.100.2:5555"
	if got := ClientIP(r); got != "198.51.100.2" {
		t.Fatalf("got %q", got)
	}
}
