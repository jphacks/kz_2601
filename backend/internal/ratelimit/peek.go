package ratelimit

import (
	"fmt"
	"time"
)

// Reading today's usage without charging it (GET /v1/me/usage on appconfig,
// docs/backend/rate-limit.md "今の使用量"). appconfig reads the same
// counter documents the ai Limiter writes; the ids and the limits below must
// stay what Allow uses (peek_test.go pins them to Allow). Peeker and the
// Memory / Firestore Peek live next to Store (ratelimit.go).

var (
	_ Peeker = (*Firestore)(nil)
	_ Peeker = (*Memory)(nil)
)

// TierOf is the tier Allow picks: a verified Pro wins over the sign-in state.
func TierOf(pro, anonymous bool) Tier {
	switch {
	case pro:
		return TierPro
	case anonymous:
		return TierAnonymous
	default:
		return TierSignedIn
	}
}

// DailyLimit is the per-user units of t (0 = not counted).
func (c Config) DailyLimit(t Tier) int {
	switch t {
	case TierAnonymous:
		return c.Limits.Anonymous
	case TierPro:
		return c.Limits.Pro
	default:
		return c.Limits.SignedIn
	}
}

// Today is the day id and the next 0:00 (JST unless Location is set) of now.
func (c Config) Today(now time.Time) (day string, resetAt time.Time) {
	loc := c.Location
	if loc == nil {
		loc = JST
	}
	now = now.In(loc)
	y, m, d := now.Date()
	return now.Format("2006-01-02"), time.Date(y, m, d+1, 0, 0, 0, 0, loc)
}

// UserCounterID is the per-user counter Allow charges uid to on day.
func (c Config) UserCounterID(day, uid string) string {
	prefix := c.Env
	if prefix == "" {
		prefix = "local"
	}
	return fmt.Sprintf("%s_%s_uid_%s", prefix, day, docSafe(uid))
}
