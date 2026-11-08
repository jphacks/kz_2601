package ratelimit

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// ScopeWaived names the hourly counter of waived requests.
const ScopeWaived = "waived"

// A waived request (/v1/text 402) is not charged against the daily limits,
// but it still reads Firestore on the way. Waives are counted per uid and
// hour on their own counter (Limits.WaivedHourly):
//
//	rateLimits/{env}_{yyyy-mm-ddTHH}_waived_{uid}   {count, scope, expireAt, updatedAt}
//
// Within the cap a waived request costs nothing of the daily limit. Past it
// Waive does not refund, so further waived requests use up the daily limit
// and then end at 429 before the handler.

// takeWaive counts one waived request for uid. false = the cap is reached
// (keep the charge). Off (0), no uid or a store failure lets the waive through.
func (l *Limiter) takeWaive(ctx context.Context, store Store, uid string) bool {
	limit := l.cfg.Limits.WaivedHourly
	if limit <= 0 || uid == "" {
		return true
	}
	now := l.cfg.Now().In(l.cfg.Location)
	hour := now.Truncate(time.Hour)
	prefix := l.cfg.Env
	if prefix == "" {
		prefix = "local"
	}
	c := Counter{ID: fmt.Sprintf("%s_%s_waived_%s", prefix, hour.Format("2006-01-02T15"), docSafe(uid)), Scope: ScopeWaived, Limit: limit}
	u, err := store.Consume(ctx, []Counter{c}, 1, hour.Add(25*time.Hour))
	if err != nil {
		l.log.WarnContext(ctx, "ratelimit: waive count failed", slog.Any("error", err))
		return true
	}
	if u.Blocked >= 0 {
		l.log.InfoContext(ctx, "ratelimit.waive_capped", slog.Int("limit", limit))
		return false
	}
	return true
}
