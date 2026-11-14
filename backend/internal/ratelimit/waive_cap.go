package ratelimit

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// ScopeWaived names the hourly counters of waived requests.
const ScopeWaived = "waived"

// A waived request (/v1/text 402) is not charged against the daily limits,
// but it still reads Firestore on the way. Waives are counted per uid and
// per client address (an anonymous uid is cheap to make again) and hour on
// their own counters (Limits.WaivedHourly / Limits.WaivedIPHourly), both
// taken in one step:
//
//	rateLimits/{env}_{yyyy-mm-ddTHH}_waived_{uid}      {count, scope, expireAt, updatedAt}
//	rateLimits/{env}_{yyyy-mm-ddTHH}_waived_ip_{hash}  {count, scope, expireAt, updatedAt}
//
// Within the caps a waived request costs nothing of the daily limit. Past
// either one Waive does not refund, so further waived requests use up the
// daily limit and then end at 429 before the handler. When the refund of
// the daily units fails after the waive was counted, the waive count is
// given back too (the request keeps its charge and is not a waive).

// waiveTake is what takeWaive counted, so it can be given back.
type waiveTake struct {
	counters []Counter
	expireAt time.Time
}

// takeWaive counts one waived request for uid and ip. false = a cap is
// reached (keep the charge; nothing counted). Off (0), no uid / ip or a
// store failure lets the waive through.
func (l *Limiter) takeWaive(ctx context.Context, store Store, uid, ip string) (waiveTake, bool) {
	now := l.cfg.Now().In(l.cfg.Location)
	hour := now.Truncate(time.Hour)
	prefix := l.cfg.Env
	if prefix == "" {
		prefix = "local"
	}
	base := fmt.Sprintf("%s_%s_waived_", prefix, hour.Format("2006-01-02T15"))
	var counters []Counter
	if limit := l.cfg.Limits.WaivedHourly; limit > 0 && uid != "" {
		counters = append(counters, Counter{ID: base + docSafe(uid), Scope: ScopeWaived, Limit: limit})
	}
	if limit := l.cfg.Limits.WaivedIPHourly; limit > 0 && ip != "" {
		counters = append(counters, Counter{ID: base + "ip_" + ipKey(ip), Scope: ScopeWaived, Limit: limit})
	}
	if len(counters) == 0 {
		return waiveTake{}, true
	}
	t := waiveTake{counters: counters, expireAt: hour.Add(25 * time.Hour)}
	u, err := store.Consume(ctx, counters, 1, t.expireAt)
	if err != nil {
		l.log.WarnContext(ctx, "ratelimit: waive count failed", slog.Any("error", err))
		return waiveTake{}, true
	}
	if u.Blocked >= 0 {
		l.log.InfoContext(ctx, "ratelimit.waive_capped", slog.Int("limit", counters[u.Blocked].Limit), slog.Bool("ip", u.Blocked > 0 || uid == ""))
		return waiveTake{}, false
	}
	return t, true
}

// untakeWaive gives back what takeWaive counted.
func (l *Limiter) untakeWaive(ctx context.Context, store Store, t waiveTake) {
	if len(t.counters) == 0 {
		return
	}
	if err := store.Refund(ctx, t.counters, 1, t.expireAt); err != nil {
		l.log.WarnContext(ctx, "ratelimit: waive count give-back failed", slog.Any("error", err))
	}
}
