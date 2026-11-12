package ratelimit

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// ScopeUnchanged names the daily counters of refunded unchanged edits.
const ScopeUnchanged = "unchanged"

// An edit that changed nothing (ReportUnchanged) gets its charge back, so
// the user does not pay for "nothing happened". It still called Claude, so
// the refunds are counted per uid and day (Limits.UnchangedDaily):
//
//	rateLimits/{env}_{yyyy-mm-dd}_unchanged_{uid}  {count, scope, expireAt, updatedAt}
//
// Past the cap the request keeps its charge (asking for changes that cannot
// happen does not make Claude free). When the refund of the daily units fails
// after the cap was counted, the count is given back too.

// unchangedTake is what takeUnchanged counted, so it can be given back.
type unchangedTake struct {
	counter  *Counter
	expireAt time.Time
}

// takeUnchanged counts one unchanged refund for uid. false = no refund
// (cap reached, cap 0, no uid / store or a store failure).
func (l *Limiter) takeUnchanged(ctx context.Context, store Store, uid string) (unchangedTake, bool) {
	limit := l.cfg.Limits.UnchangedDaily
	if limit <= 0 || uid == "" || store == nil {
		return unchangedTake{}, false
	}
	day, reset := l.cfg.Today(l.cfg.Now())
	prefix := l.cfg.Env
	if prefix == "" {
		prefix = "local"
	}
	c := Counter{ID: fmt.Sprintf("%s_%s_unchanged_%s", prefix, day, docSafe(uid)), Scope: ScopeUnchanged, Limit: limit}
	t := unchangedTake{counter: &c, expireAt: reset.Add(24 * time.Hour)}
	u, err := store.Consume(ctx, []Counter{c}, 1, t.expireAt)
	if err != nil {
		l.log.WarnContext(ctx, "ratelimit: unchanged count failed", slog.Any("error", err))
		return unchangedTake{}, false
	}
	if u.Blocked >= 0 {
		l.log.InfoContext(ctx, "ratelimit.unchanged_capped", slog.Int("limit", limit))
		return unchangedTake{}, false
	}
	return t, true
}

// untakeUnchanged gives back what takeUnchanged counted.
func (l *Limiter) untakeUnchanged(ctx context.Context, store Store, t unchangedTake) {
	if t.counter == nil || store == nil {
		return
	}
	if err := store.Refund(ctx, []Counter{*t.counter}, 1, t.expireAt); err != nil {
		l.log.WarnContext(ctx, "ratelimit: unchanged count give-back failed", slog.Any("error", err))
	}
}
