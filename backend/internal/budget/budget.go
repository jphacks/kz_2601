// Package budget pauses the free plan's AI work for the rest of the day once
// the environment's AI cost of the day passes a threshold (#317). Pro callers
// go on (up to their rate limit). Off unless AI_FREE_DAILY_BUDGET_USD is set.
//
// The cost is the est_cost_usd of the ai.usage log lines (#298): Tap sees
// each line on its way to the logger and adds it to the day's total, kept in
// the rate limit collection next to the other daily counters
// (docs/backend/rate-limit.md "全体の遮断").
package budget

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// Env names.
const (
	// EnvDailyUSD is the threshold in USD per day (JST); empty or 0 = off.
	EnvDailyUSD = "AI_FREE_DAILY_BUDGET_USD"
	// EnvResumeDay (YYYY-MM-DD, JST) lifts the pause for that day only: the
	// manual way back that turns itself off at midnight.
	EnvResumeDay = "AI_FREE_BUDGET_RESUME_DAY"
)

// Config configures a Breaker.
type Config struct {
	DailyUSD  float64
	ResumeDay string // YYYY-MM-DD in Location; "" = none
	// Env prefixes the counter id (stg and prod may share a Firestore).
	Env string
	// CacheTTL is how long an instance trusts the day's total it read
	// (its own additions count at once). 0 = 30s.
	CacheTTL time.Duration
	Location *time.Location // nil = JST
	Now      func() time.Time
}

// Enabled reports whether the breaker does anything.
func (c Config) Enabled() bool { return c.DailyUSD > 0 }

// ConfigFromEnv reads AI_FREE_DAILY_BUDGET_USD and AI_FREE_BUDGET_RESUME_DAY.
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	cfg := Config{Env: getenv("APP_ENV")}
	if raw := strings.TrimSpace(getenv(EnvDailyUSD)); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || v < 0 {
			return cfg, fmt.Errorf("%s must be a non-negative number (USD), got %q", EnvDailyUSD, raw)
		}
		cfg.DailyUSD = v
	}
	if raw := strings.TrimSpace(getenv(EnvResumeDay)); raw != "" {
		if _, err := time.Parse("2006-01-02", raw); err != nil {
			return cfg, fmt.Errorf("%s must be YYYY-MM-DD (JST), got %q", EnvResumeDay, raw)
		}
		cfg.ResumeDay = raw
	}
	return cfg, nil
}

// Store keeps the day's total cost.
type Store interface {
	Add(ctx context.Context, id string, usd float64, expireAt time.Time) error
	Total(ctx context.Context, id string) (float64, error)
}

// ProChecker reports whether uid has Tailor It Pro (entitlement.ProChecker).
type ProChecker interface {
	IsPro(ctx context.Context, uid string) bool
}

// Breaker tracks the day's cost and decides whether free callers pause.
type Breaker struct {
	cfg   Config
	store Store
	pro   ProChecker
	log   *slog.Logger

	mu     sync.Mutex
	day    string
	total  float64   // last read from the store + this instance's additions since
	readAt time.Time // zero = not read yet today
}

// New builds a Breaker (store nil = memory).
func New(cfg Config, store Store, pro ProChecker) *Breaker {
	if cfg.Location == nil {
		cfg.Location = ratelimit.JST
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 30 * time.Second
	}
	if store == nil {
		store = NewMemory()
	}
	return &Breaker{cfg: cfg, store: store, pro: pro, log: slog.Default()}
}

// day is today's key and the next midnight.
func (b *Breaker) today() (day string, reset time.Time) {
	now := b.cfg.Now().In(b.cfg.Location)
	y, m, d := now.Date()
	return now.Format("2006-01-02"), time.Date(y, m, d+1, 0, 0, 0, 0, b.cfg.Location)
}

func (b *Breaker) counterID(day string) string {
	prefix := b.cfg.Env
	if prefix == "" {
		prefix = "local"
	}
	return prefix + "_" + day + "_ai_cost"
}

// Record adds usd to today's total. The store write runs in the background
// (it must not slow the Claude call down); this instance counts it at once.
func (b *Breaker) Record(ctx context.Context, usd float64) {
	if usd <= 0 || !b.cfg.Enabled() {
		return
	}
	day, reset := b.today()
	b.mu.Lock()
	if b.day == day {
		b.total += usd
	}
	b.mu.Unlock()
	go func() {
		wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := b.store.Add(wctx, b.counterID(day), usd, reset.Add(24*time.Hour)); err != nil {
			b.log.WarnContext(wctx, "budget: add failed", slog.Any("error", err))
		}
	}()
}

// Spent is today's total (cached for CacheTTL). A failed read keeps the last
// known total: the breaker neither trips nor lifts on a Firestore outage.
func (b *Breaker) Spent(ctx context.Context) float64 {
	day, _ := b.today()
	now := b.cfg.Now()
	b.mu.Lock()
	if b.day == day && !b.readAt.IsZero() && now.Sub(b.readAt) < b.cfg.CacheTTL {
		t := b.total
		b.mu.Unlock()
		return t
	}
	b.mu.Unlock()
	t, err := b.store.Total(ctx, b.counterID(day))
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.day != day {
		b.day, b.total, b.readAt = day, 0, time.Time{}
	}
	if err != nil {
		b.log.WarnContext(ctx, "budget: read failed; keeping the last total", slog.Any("error", err))
		return b.total
	}
	b.total, b.readAt = max(t, b.total), now
	return b.total
}

// Paused reports whether free callers are paused now, and when it lifts.
func (b *Breaker) Paused(ctx context.Context) (bool, time.Time) {
	day, reset := b.today()
	if !b.cfg.Enabled() || day == b.cfg.ResumeDay {
		return false, reset
	}
	return b.Spent(ctx) >= b.cfg.DailyUSD, reset
}
