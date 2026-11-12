package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/runner"
)

// scriptTicker returns res/err and signals each call on ticked.
type scriptTicker struct {
	res    runner.TickResult
	err    error
	calls  atomic.Int32
	ticked chan time.Duration
}

func (s *scriptTicker) Tick(_ context.Context, budget time.Duration) (runner.TickResult, error) {
	s.calls.Add(1)
	if s.ticked != nil {
		select {
		case s.ticked <- budget:
		default:
		}
	}
	return s.res, s.err
}

// syncBuffer is a log sink safe for the loop goroutine.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func TestMuxTickOutcomes(t *testing.T) {
	cases := []struct {
		name   string
		ticker *scriptTicker
		busy   bool
		code   int
		status string
	}{
		{"ok", &scriptTicker{res: runner.TickResult{Claimed: 1, Done: 1}}, false, http.StatusOK, "ok"},
		{"error", &scriptTicker{res: runner.TickResult{Claimed: 3, Retried: 3}, err: errors.New("firestore down")}, false, http.StatusInternalServerError, "error"},
		{"busy", &scriptTicker{}, true, http.StatusOK, "busy"},
	}
	for _, c := range cases {
		s := &serial{t: c.ticker}
		if c.busy {
			s.mu.Lock()
		}
		rr := httptest.NewRecorder()
		newMux(s, slog.New(slog.DiscardHandler)).ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/internal/tick", nil))
		if c.busy {
			s.mu.Unlock()
		}
		var body struct {
			Status string            `json:"status"`
			Result runner.TickResult `json:"result"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: %v %s", c.name, err, rr.Body)
		}
		if rr.Code != c.code || body.Status != c.status {
			t.Errorf("%s: %d %s", c.name, rr.Code, rr.Body)
		}
		if c.busy && c.ticker.calls.Load() != 0 {
			t.Errorf("%s: ticked while busy", c.name)
		}
		if c.name == "error" && body.Result.Retried != 3 {
			t.Errorf("error keeps the partial result: %s", rr.Body)
		}
		if strings.Contains(rr.Body.String(), "firestore down") {
			t.Errorf("%s: error text in the response: %s", c.name, rr.Body)
		}
	}
	rr := httptest.NewRecorder()
	newMux(&serial{t: &scriptTicker{}}, slog.New(slog.DiscardHandler)).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("healthz: %d", rr.Code)
	}
}

func TestEnvOr(t *testing.T) {
	t.Setenv("RUNNER_TEST_ENVOR", "")
	if got := envOr("RUNNER_TEST_ENVOR", "8080"); got != "8080" {
		t.Fatalf("empty: %q", got)
	}
	t.Setenv("RUNNER_TEST_ENVOR", "9090")
	if got := envOr("RUNNER_TEST_ENVOR", "8080"); got != "9090" {
		t.Fatalf("set: %q", got)
	}
}

// The local loop ticks with a budget of min(interval, TickBudget), logs a
// failed tick and keeps going, and stops with ctx.
func TestLoop(t *testing.T) {
	ft := &scriptTicker{err: errors.New("boom"), ticked: make(chan time.Duration, 1)}
	var logs syncBuffer
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		loop(ctx, &serial{t: ft}, 5*time.Millisecond, slog.New(slog.NewTextHandler(&logs, nil)))
		close(done)
	}()
	for range 2 { // a second tick after a failed one: the loop kept going
		select {
		case b := <-ft.ticked:
			if b != 5*time.Millisecond {
				t.Fatalf("budget %v, want the interval", b)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("loop did not tick")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not stop with ctx")
	}
	if !strings.Contains(logs.String(), "tick failed") {
		t.Fatalf("logs: %s", logs.String())
	}

}

func TestTickBudget(t *testing.T) {
	if got := tickBudget(2 * TickBudget); got != TickBudget {
		t.Fatalf("2*TickBudget: %v, want TickBudget", got)
	}
	if got := tickBudget(5 * time.Millisecond); got != 5*time.Millisecond {
		t.Fatalf("small interval: %v, want the interval", got)
	}
}

func TestServe(t *testing.T) {
	if err := serve(t.Context(), "127.0.0.1:not-a-port", http.NotFoundHandler(), &serial{t: &scriptTicker{}}, 0, slog.New(slog.DiscardHandler)); err == nil {
		t.Fatal("a bad port was accepted")
	}

	// ctx ends: the server shuts down cleanly; interval > 0 starts the loop.
	// Loopback only, so macOS does not ask to allow incoming connections.
	ft := &scriptTicker{ticked: make(chan time.Duration, 1)}
	ctx, cancel := context.WithCancel(t.Context())
	errc := make(chan error, 1)
	go func() {
		errc <- serve(ctx, "127.0.0.1:0", http.NotFoundHandler(), &serial{t: ft}, 5*time.Millisecond, slog.New(slog.DiscardHandler))
	}()
	select {
	case <-ft.ticked:
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not start the loop")
	}
	cancel()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return after ctx ended")
	}
}
