package entitlement

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Reader is the part of Store ProChecker needs.
type Reader interface {
	Get(ctx context.Context, uid string) (*Pro, error)
}

// ProChecker implements ratelimit.ProChecker from the stored entitlement
// (#449). Results are cached per instance: "Pro" until min(expiresAt, PosTTL),
// "not Pro" for NegTTL (a fresh purchase counts within NegTTL). Read errors
// count as "not Pro" (signed-in limit) and are not cached.
type ProChecker struct {
	Store        Reader
	Environments []string // accepted App Store environments (empty = any)
	PosTTL       time.Duration
	NegTTL       time.Duration
	Now          func() time.Time

	mu    sync.Mutex
	cache map[string]proEntry
}

type proEntry struct {
	pro   bool
	until time.Time
}

// NewProChecker uses the default TTLs (5 min / 15 s).
func NewProChecker(r Reader, envs []string) *ProChecker {
	return &ProChecker{Store: r, Environments: envs, PosTTL: 5 * time.Minute, NegTTL: 15 * time.Second, Now: time.Now}
}

// IsPro implements ratelimit.ProChecker.
func (c *ProChecker) IsPro(ctx context.Context, uid string) bool {
	now := c.Now()
	c.mu.Lock()
	if e, ok := c.cache[uid]; ok && now.Before(e.until) {
		c.mu.Unlock()
		return e.pro
	}
	c.mu.Unlock()

	p, err := c.Store.Get(ctx, uid)
	if err != nil {
		slog.WarnContext(ctx, "entitlement: pro check failed; treating as not pro", slog.Any("error", err))
		return false
	}
	e := proEntry{pro: p.Active(now, c.Environments), until: now.Add(c.NegTTL)}
	if e.pro {
		e.until = now.Add(c.PosTTL)
		if p.Until().Before(e.until) {
			e.until = p.Until()
		}
	}
	c.mu.Lock()
	if c.cache == nil || len(c.cache) > 10000 {
		c.cache = map[string]proEntry{}
	}
	c.cache[uid] = e
	c.mu.Unlock()
	return e.pro
}

// Forget drops uid's cached answer (a purchase or restore was just stored).
func (c *ProChecker) Forget(uid string) {
	c.mu.Lock()
	delete(c.cache, uid)
	c.mu.Unlock()
}
