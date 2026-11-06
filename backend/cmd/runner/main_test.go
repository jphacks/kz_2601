package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/fsenv"
	"github.com/jphacks/kz_2601/backend/internal/runner"
)

func envMap(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

// decision「#288 の分離までは prod だけ」: stg on the production database
// never starts (ALLOW_SHARED_PROD_DB does not help); the emulator does.
func TestGuard(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		ok   bool
	}{
		{"no project", map[string]string{}, false},
		{"prod", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject, "APP_ENV": "prod"}, true},
		{"stg on prod db", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject, "APP_ENV": "stg"}, false},
		{"stg on prod db with override", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject, "APP_ENV": "staging", fsenv.OverrideEnv: "1"}, false},
		{"stg on its own db", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject, "APP_ENV": "stg", "FIRESTORE_DATABASE": "stg"}, true},
		{"emulator", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject, "APP_ENV": "stg", "FIRESTORE_EMULATOR_HOST": "127.0.0.1:8089"}, true},
	}
	for _, c := range cases {
		get := envMap(c.env)
		if err := guard(fsenv.FromEnv(get), get); (err == nil) != c.ok {
			t.Errorf("%s: err=%v, want ok=%v", c.name, err, c.ok)
		}
	}
}

func TestEnvParsing(t *testing.T) {
	for in, want := range map[string]string{"": "local", "staging": "stg", "stg": "stg", "prod": "prod"} {
		if got := outboxEnv(in); got != want {
			t.Errorf("outboxEnv(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]bool{"": false, "0": false, "1": true, "true": true, "ON": true} {
		if got := pausedFromEnv(envMap(map[string]string{"RUNNER_PAUSED": in}))(); got != want {
			t.Errorf("RUNNER_PAUSED=%q: %v", in, got)
		}
	}
	if d, err := tickInterval(envMap(nil)); d != 0 || err != nil {
		t.Errorf("unset interval: %v %v", d, err)
	}
	if d, err := tickInterval(envMap(map[string]string{"RUNNER_TICK_INTERVAL": "10s"})); d != 10*time.Second || err != nil {
		t.Errorf("10s: %v %v", d, err)
	}
	for _, bad := range []string{"x", "10ms", "-1s"} {
		if _, err := tickInterval(envMap(map[string]string{"RUNNER_TICK_INTERVAL": bad})); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if d, err := maxLateFromEnv(envMap(nil)); d != 30*time.Minute || err != nil {
		t.Errorf("unset max late: %v %v", d, err)
	}
	if d, err := maxLateFromEnv(envMap(map[string]string{"RUNNER_SCHEDULE_MAX_LATE": "45m"})); d != 45*time.Minute || err != nil {
		t.Errorf("45m: %v %v", d, err)
	}
	for _, bad := range []string{"x", "10s", "-5m"} {
		if _, err := maxLateFromEnv(envMap(map[string]string{"RUNNER_SCHEDULE_MAX_LATE": bad})); err == nil {
			t.Errorf("max late %q accepted", bad)
		}
	}
	// time/tzdata is linked in: zones load without the OS database
	if _, err := time.LoadLocation("Asia/Tokyo"); err != nil {
		t.Fatal(err)
	}
}

type fakeTicker struct {
	calls   atomic.Int32
	block   chan struct{}
	started chan struct{}
}

func (f *fakeTicker) Tick(ctx context.Context, budget time.Duration) (runner.TickResult, error) {
	f.calls.Add(1)
	if f.block != nil {
		close(f.started)
		<-f.block
	}
	return runner.TickResult{Claimed: 2, Done: 2}, nil
}

func TestMux(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	ft := &fakeTicker{}
	mux := newMux(&serial{t: ft}, slog.New(slog.DiscardHandler))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"env":"prod"`) {
		t.Fatalf("health: %d %s", rr.Code, rr.Body)
	}
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/internal/tick", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET tick: %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/internal/tick", nil))
	var body struct {
		Status string            `json:"status"`
		Result runner.TickResult `json:"result"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil || rr.Code != http.StatusOK || body.Status != "ok" || body.Result.Done != 2 {
		t.Fatalf("tick: %d %s", rr.Code, rr.Body)
	}
}

// a tick while one runs is skipped (the local loop and /internal/tick share one lock)
func TestSerialBusy(t *testing.T) {
	ft := &fakeTicker{block: make(chan struct{}), started: make(chan struct{})}
	s := &serial{t: ft}
	done := make(chan struct{})
	go func() { _, _, _ = s.tick(t.Context(), time.Second); close(done) }()
	<-ft.started
	if _, busy, _ := s.tick(t.Context(), time.Second); !busy {
		t.Fatal("second tick ran while the first was running")
	}
	close(ft.block)
	<-done
	if ft.calls.Load() != 1 {
		t.Fatalf("calls: %d", ft.calls.Load())
	}
}

func TestNotifyPausedAndAutoPauseFromEnv(t *testing.T) {
	if !flagFromEnv(envMap(map[string]string{"RUNNER_NOTIFY_PAUSED": "on"}), "RUNNER_NOTIFY_PAUSED")() {
		t.Fatal("RUNNER_NOTIFY_PAUSED=on")
	}
	if flagFromEnv(envMap(nil), "RUNNER_NOTIFY_PAUSED")() {
		t.Fatal("unset is not paused")
	}
	if a, err := autoPauseFromEnv(envMap(nil)); err != nil || a.Reporters != 0 || a.For != 0 {
		t.Fatalf("defaults: %+v %v", a, err)
	}
	a, err := autoPauseFromEnv(envMap(map[string]string{"RUNNER_AUTO_NOTIFY_PAUSE_REPORTERS": "5", "RUNNER_AUTO_NOTIFY_PAUSE_FOR": "24h"}))
	if err != nil || a.Reporters != 5 || a.For != 24*time.Hour {
		t.Fatalf("set: %+v %v", a, err)
	}
	if a, err := autoPauseFromEnv(envMap(map[string]string{"RUNNER_AUTO_NOTIFY_PAUSE_REPORTERS": "0"})); err != nil || a.Reporters >= 0 {
		t.Fatalf("0 = off: %+v %v", a, err)
	}
	for _, bad := range []map[string]string{
		{"RUNNER_AUTO_NOTIFY_PAUSE_REPORTERS": "-1"}, {"RUNNER_AUTO_NOTIFY_PAUSE_REPORTERS": "x"},
		{"RUNNER_AUTO_NOTIFY_PAUSE_FOR": "10m"}, {"RUNNER_AUTO_NOTIFY_PAUSE_FOR": "x"},
	} {
		if _, err := autoPauseFromEnv(envMap(bad)); err == nil {
			t.Fatalf("%v accepted", bad)
		}
	}
}
