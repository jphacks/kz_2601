package budget

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestConfigFromEnv(t *testing.T) {
	c, err := ConfigFromEnv(envOf(nil))
	if err != nil || c.Enabled() {
		t.Fatalf("unset = off: %+v %v", c, err)
	}
	c, err = ConfigFromEnv(envOf(map[string]string{EnvDailyUSD: "25.5", EnvResumeDay: "2026-11-07", "APP_ENV": "stg"}))
	if err != nil || c.DailyUSD != 25.5 || c.ResumeDay != "2026-11-07" || c.Env != "stg" || !c.Enabled() {
		t.Fatalf("set: %+v %v", c, err)
	}
	for k, v := range map[string]string{EnvDailyUSD: "-1", EnvResumeDay: "11/07"} {
		if _, err := ConfigFromEnv(envOf(map[string]string{k: v})); err == nil {
			t.Errorf("%s=%q must fail", k, v)
		}
	}
	if _, err := ConfigFromEnv(envOf(map[string]string{EnvDailyUSD: "abc"})); err == nil {
		t.Error("not a number must fail")
	}
}

// clock is a settable time.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

// syncStore is a Memory whose Add signals, so tests wait for the background write.
type syncStore struct {
	*Memory
	added chan struct{}
}

func (s *syncStore) Add(ctx context.Context, id string, usd float64, exp time.Time) error {
	defer func() { s.added <- struct{}{} }()
	return s.Memory.Add(ctx, id, usd, exp)
}

func newTestBreaker(daily float64, resume string) (*Breaker, *syncStore, *clock) {
	clk := &clock{t: time.Date(2026, 11, 7, 3, 0, 0, 0, time.UTC)} // 12:00 JST
	st := &syncStore{Memory: NewMemory(), added: make(chan struct{}, 16)}
	b := New(Config{DailyUSD: daily, ResumeDay: resume, Env: "test", Now: clk.now}, st, proSet{"pro": true})
	return b, st, clk
}

type proSet map[string]bool

func (p proSet) IsPro(_ context.Context, uid string) bool { return p[uid] }

func TestBreakerPaused(t *testing.T) {
	ctx := context.Background()
	b, st, clk := newTestBreaker(10, "")
	if p, _ := b.Paused(ctx); p {
		t.Fatal("nothing spent")
	}
	b.Record(ctx, 6)
	<-st.added
	if p, _ := b.Paused(ctx); p {
		t.Fatal("6 < 10")
	}
	b.Record(ctx, 4)
	<-st.added
	p, reset := b.Paused(ctx)
	if !p {
		t.Fatal("10 >= 10 pauses (this instance's own additions count at once)")
	}
	if want := time.Date(2026, 11, 8, 0, 0, 0, 0, time.FixedZone("JST", 9*3600)); !reset.Equal(want) {
		t.Fatalf("reset = %v, want %v", reset, want)
	}
	if got, _ := st.Total(ctx, "test_2026-11-07_ai_cost"); got != 10 {
		t.Fatalf("stored total = %v", got)
	}

	// another instance's spend arrives after the cache expires
	other, _, _ := newTestBreaker(10, "")
	other.store = st
	other.cfg.Now = clk.now
	if p, _ := other.Paused(ctx); !p {
		t.Fatal("other instance reads the shared total")
	}

	// the next day (JST) starts from 0
	clk.t = clk.t.Add(13 * time.Hour) // 01:00 JST next day
	if p, _ := b.Paused(ctx); p {
		t.Fatal("new day")
	}
}

func TestBreakerResumeDayAndOff(t *testing.T) {
	ctx := context.Background()
	b, st, _ := newTestBreaker(1, "2026-11-07")
	b.Spent(ctx)
	b.Record(ctx, 5)
	<-st.added
	if p, _ := b.Paused(ctx); p {
		t.Fatal("resumed for today")
	}
	off, _, _ := newTestBreaker(0, "")
	off.Record(ctx, 100)
	if p, _ := off.Paused(ctx); p {
		t.Fatal("0 = off")
	}
}

func TestTap(t *testing.T) {
	ctx := context.Background()
	b, st, _ := newTestBreaker(100, "")
	var buf bytes.Buffer
	lg := slog.New(b.Tap(slog.NewJSONHandler(&buf, nil), "ai.usage")).With("svc", "ai")
	lg.LogAttrs(ctx, slog.LevelInfo, "ai.usage", slog.Float64("est_cost_usd", 0.25))
	<-st.added
	lg.LogAttrs(ctx, slog.LevelInfo, "ai.usage", slog.String("model", "unknown"))    // unpriced: nothing
	lg.LogAttrs(ctx, slog.LevelInfo, "ai.generate", slog.Float64("est_cost_usd", 9)) // the request sum: not again
	if got := b.Spent(ctx); got != 0.25 {
		t.Fatalf("spent = %v", got)
	}
	if n := strings.Count(buf.String(), "\n"); n != 3 {
		t.Fatalf("every line still logged: %d", n)
	}
	if off := New(Config{}, nil, nil); off.Tap(slog.Default().Handler(), "ai.usage") != slog.Default().Handler() {
		t.Fatal("off: the handler is unchanged")
	}
}

func TestMiddleware(t *testing.T) {
	ctx := context.Background()
	b, st, _ := newTestBreaker(1, "")
	b.Record(ctx, 0) // no-op
	b.Spent(ctx)
	b.Record(ctx, 2)
	<-st.added

	reached := false
	app := b.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/v1/generate" && !bytes.Contains(body, []byte("mode")) {
			t.Errorf("body must reach the handler unchanged: %q", body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	do := func(uid, method, path, body string) *httptest.ResponseRecorder {
		reached = false
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if uid != "" {
			r = r.WithContext(auth.ContextWithUID(r.Context(), uid))
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		return w
	}
	for _, c := range []struct {
		name, uid, method, path, body string
		paused                        bool
	}{
		{"free generate", "u", http.MethodPost, "/v1/generate", `{"mode":"create"}`, true},
		{"free generate, multipart / no mode", "u", http.MethodPost, "/v1/generate", `--x`, true},
		{"free chat", "u", http.MethodPost, "/v1/chat", `{}`, true},
		{"no uid (rate limit off)", "", http.MethodPost, "/v1/chat", `{}`, true},
		{"free undo", "u", http.MethodPost, "/v1/generate", `{"mode":"undo"}`, false},
		{"text (Pro feature)", "u", http.MethodPost, "/v1/text", `{}`, false},
		{"delete chat", "u", http.MethodDelete, "/v1/chat/app", ``, false},
		{"pro generate", "pro", http.MethodPost, "/v1/generate", `{"mode":"create"}`, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := do(c.uid, c.method, c.path, c.body)
			if !c.paused {
				if !reached || w.Code != http.StatusOK {
					t.Fatalf("want pass, got %d %s", w.Code, w.Body)
				}
				return
			}
			if reached || w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") == "" {
				t.Fatalf("want 503, got %d reached=%v", w.Code, reached)
			}
			var out struct {
				Error struct {
					Code    string         `json:"code"`
					Message string         `json:"message"`
					Details map[string]any `json:"details"`
				} `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if out.Error.Code != httpx.CodeFreePlanPaused || out.Error.Details["reason"] != "daily_budget" ||
				out.Error.Details["resumeAt"] != "2026-11-07T15:00:00Z" || !strings.Contains(out.Error.Message, "11月8日 0:00") {
				t.Fatalf("body: %s", w.Body)
			}
		})
	}

	// disabled: everything passes
	off := New(Config{}, nil, nil).Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	w := httptest.NewRecorder()
	off.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/generate", strings.NewReader(`{}`)))
	if w.Code != http.StatusTeapot {
		t.Fatalf("off: %d", w.Code)
	}
}
