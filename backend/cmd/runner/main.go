// Command runner runs server rules (#341, M4). It drains the outbox that
// appconfig fills (runner/{APP_ENV}/outbox) on POST /internal/tick (Cloud
// Scheduler in production, about once a minute; each call works for about
// TickBudget) and, locally, on its own every RUNNER_TICK_INTERVAL.
//
// Not deployed yet: adding the service, its SA and the Scheduler job needs a
// human's approval (docs/00_decisions.md「M4 バックエンドの自動構築」).
//
// Environment:
//
//	GOOGLE_CLOUD_PROJECT / FIRESTORE_DATABASE  Firestore (required; internal/fsenv)
//	FIRESTORE_EMULATOR_HOST                    local emulator (make run-runner)
//	APP_ENV                                    names the outbox ("" = local, staging = stg)
//	RUNNER_PAUSED=1                            finish every event as skipped_paused (not run later)
//	RUNNER_TICK_INTERVAL                       e.g. 10s: tick by itself (local); "" = only on /internal/tick
//	RUNNER_LIMIT_* / RUNNER_GLOBAL_DAILY       quota (internal/runner/quota)
//	RATE_LIMIT_COLLECTION                      where quota counters live (ratelimit)
//	PORT                                       default 8080
//
// A stg process on the production database refuses to start even with
// ALLOW_SHARED_PROD_DB=1: until the split (#288) the runner runs in prod
// only, so stg and prod never work on one database (the emulator is fine).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata" // creators' time zones on a distroless image

	"github.com/jphacks/kz_2601/backend/internal/entitlement"
	"github.com/jphacks/kz_2601/backend/internal/fsenv"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
	"github.com/jphacks/kz_2601/backend/internal/runner"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/runner/schedule"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// TickBudget is how long one /internal/tick keeps working (below Cloud
// Scheduler's and Cloud Run's request timeouts, and under the one-minute
// schedule so ticks rarely overlap; leases keep overlapping ticks apart).
const TickBudget = 50 * time.Second

const shutdownTimeout = 15 * time.Second

func main() {
	logger := httpx.NewLogger()
	slog.SetDefault(logger)
	target := fsenv.FromEnv(os.Getenv)
	if err := guard(target, os.Getenv); err != nil {
		logger.Error("runner: refusing to start", slog.Any("error", err))
		os.Exit(2)
	}
	interval, err := tickInterval(os.Getenv)
	if err != nil {
		logger.Error("runner: bad configuration", slog.Any("error", err))
		os.Exit(2)
	}
	maxLate, err := maxLateFromEnv(os.Getenv)
	if err != nil {
		logger.Error("runner: bad configuration", slog.Any("error", err))
		os.Exit(2)
	}
	qcfg, err := quota.ConfigFromEnv(os.Getenv)
	if err != nil {
		logger.Error("runner: bad configuration", slog.Any("error", err))
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.NewFirestore(ctx, target.Project, target.Database)
	if err != nil {
		logger.Error("runner: firestore init failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()
	policy, err := entitlement.ConfigFromEnv(os.Getenv)
	if err != nil {
		logger.Error("runner: appstore config", slog.Any("error", err))
		os.Exit(2)
	}
	q := quota.New(qcfg, ratelimit.NewFirestore(st.Client(), os.Getenv("RATE_LIMIT_COLLECTION")),
		entitlement.NewProChecker(entitlement.NewFirestore(st.Client()), policy.Environments))
	q.Logger = logger
	env := outboxEnv(os.Getenv("APP_ENV"))
	// No device-token store yet (#269) and no APNs key here: notifications
	// are resolved and counted, and LogSender logs their shape only.
	r, err := runner.New(runner.Config{Env: env, Store: st, Quota: q, Paused: pausedFromEnv(os.Getenv), MaxLate: maxLate, Logger: logger})
	if err != nil {
		logger.Error("runner: init failed", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("runner: starting", slog.String("firestore", target.String()), slog.String("outbox_env", env),
		slog.Duration("tick_interval", interval), slog.Bool("paused", pausedFromEnv(os.Getenv)()))
	t := &serial{t: r}
	if err := serve(ctx, envOr("PORT", "8080"), newMux(t, logger), t, interval, logger); err != nil {
		logger.Error("runner: serve failed", slog.Any("error", err))
		os.Exit(1)
	}
}

// guard refuses a stg process on the production database (no override:
// decision「#288 の分離までは prod だけ」), and a process without Firestore.
func guard(t fsenv.Target, getenv func(string) string) error {
	if t.Project == "" {
		return errors.New("GOOGLE_CLOUD_PROJECT is required (the runner has no in-memory mode)")
	}
	if fsenv.IsProd(t, getenv) {
		return fmt.Errorf("APP_ENV=%s would use the production Firestore %s; the runner runs in prod only until the database split (#288)", t.AppEnv, t)
	}
	return nil
}

// outboxEnv names the outbox like appconfig does ("" = local, staging = stg).
func outboxEnv(appEnv string) string {
	switch e := strings.ToLower(strings.TrimSpace(appEnv)); e {
	case "":
		return "local"
	case "staging":
		return "stg"
	default:
		return e
	}
}

// pausedFromEnv reads RUNNER_PAUSED on every call (1 / true / on).
func pausedFromEnv(getenv func(string) string) func() bool {
	return func() bool {
		switch strings.ToLower(strings.TrimSpace(getenv("RUNNER_PAUSED"))) {
		case "1", "true", "on":
			return true
		}
		return false
	}
}

// tickInterval reads RUNNER_TICK_INTERVAL ("" = off).
func tickInterval(getenv func(string) string) (time.Duration, error) {
	raw := strings.TrimSpace(getenv("RUNNER_TICK_INTERVAL"))
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < time.Second {
		return 0, fmt.Errorf("RUNNER_TICK_INTERVAL must be a duration of at least 1s, got %q", raw)
	}
	return d, nil
}

// maxLateFromEnv reads RUNNER_SCHEDULE_MAX_LATE ("" = schedule.DefaultMaxLate).
func maxLateFromEnv(getenv func(string) string) (time.Duration, error) {
	raw := strings.TrimSpace(getenv("RUNNER_SCHEDULE_MAX_LATE"))
	if raw == "" {
		return schedule.DefaultMaxLate, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < time.Minute {
		return 0, fmt.Errorf("RUNNER_SCHEDULE_MAX_LATE must be a duration of at least 1m, got %q", raw)
	}
	return d, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type ticker interface {
	Tick(ctx context.Context, budget time.Duration) (runner.TickResult, error)
}

// serial runs one tick at a time per instance (the HTTP tick and the local
// loop share it); a tick that finds one running is skipped.
type serial struct {
	t  ticker
	mu sync.Mutex
}

func (s *serial) tick(ctx context.Context, budget time.Duration) (res runner.TickResult, busy bool, err error) {
	if !s.mu.TryLock() {
		return res, true, nil
	}
	defer s.mu.Unlock()
	res, err = s.t.Tick(ctx, budget)
	return res, false, err
}

// newMux: GET /health and POST /internal/tick. In production the service
// is private (Cloud Run IAM: only the Scheduler's SA may invoke it).
func newMux(t *serial, logger *slog.Logger) *http.ServeMux {
	mux := http.NewServeMux()
	health := httpx.Health(httpx.HealthInfoFromEnv(os.Getenv))
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("POST /internal/tick", func(w http.ResponseWriter, req *http.Request) {
		res, busy, err := t.tick(req.Context(), TickBudget)
		if busy {
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "busy"})
			return
		}
		if err != nil {
			logger.ErrorContext(req.Context(), "runner: tick failed", slog.Any("error", err))
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "result": res})
			return
		}
		logger.InfoContext(req.Context(), "runner: tick", slog.Int("claimed", res.Claimed), slog.Int("done", res.Done),
			slog.Int("retried", res.Retried), slog.Int("gave_up", res.GaveUp), slog.Bool("more", res.More))
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "result": res})
	})
	return mux
}

// serve runs the HTTP server and, when interval > 0, the local tick loop.
func serve(ctx context.Context, port string, h http.Handler, t *serial, interval time.Duration, logger *slog.Logger) error {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- server.Serve(ln) }()
	if interval > 0 {
		go loop(ctx, t, interval, logger)
	}
	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return server.Shutdown(sctx)
	}
}

// loop ticks every interval until ctx ends (local development).
func loop(ctx context.Context, t *serial, interval time.Duration, logger *slog.Logger) {
	tk := time.NewTicker(interval)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			res, _, err := t.tick(ctx, min(interval, TickBudget))
			if err != nil {
				logger.ErrorContext(ctx, "runner: tick failed", slog.Any("error", err))
				continue
			}
			if res.Claimed > 0 {
				logger.InfoContext(ctx, "runner: tick", slog.Int("claimed", res.Claimed), slog.Int("done", res.Done),
					slog.Int("retried", res.Retried), slog.Int("gave_up", res.GaveUp))
			}
		}
	}
}
