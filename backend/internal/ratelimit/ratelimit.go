// Package ratelimit caps how much Claude work one caller (and everyone
// together) can trigger per day (#280). Usage is counted in "units": a chat
// turn, an edit and a new app weigh differently because they cost differently.
//
// Counters live in a Store (Firestore in production, memory in tests and
// local runs). Each counter is one document per day; see firestore.go.
package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
)

// Tier picks the per-user daily limit.
type Tier string

const (
	TierAnonymous Tier = "anonymous"
	TierSignedIn  Tier = "signed_in"
	TierPro       Tier = "pro"
)

// Scope names which counter blocked a request.
const (
	ScopeUser   = "user"
	ScopeIP     = "ip"
	ScopeGlobal = "global"
)

// Limits are units per day. 0 disables that counter.
type Limits struct {
	Anonymous int
	SignedIn  int
	Pro       int
	// PerIP applies to anonymous callers only (re-created anonymous accounts
	// share a network); signed-in users behind the same Wi-Fi are not affected.
	PerIP int
	// Global is the cost breaker across every caller of this environment.
	Global int
}

// Weights are the units one request costs.
type Weights struct {
	Create int // POST /v1/generate, new app (mode create/auto)
	Edit   int // POST /v1/generate, mode edit
	Chat   int // POST /v1/chat, one turn
	Text   int // POST /v1/text, one aiButton generate (#417)
}

// DefaultLimits / DefaultWeights: see docs/backend/rate-limit.md for the reasoning.
var (
	DefaultLimits  = Limits{Anonymous: 30, SignedIn: 60, Pro: 300, PerIP: 300, Global: 2000}
	DefaultWeights = Weights{Create: 3, Edit: 2, Chat: 1, Text: 1}
)

// JST is the day boundary (00:00 Asia/Tokyo). A fixed zone avoids needing
// tzdata in the container image.
var JST = time.FixedZone("JST", 9*60*60)

// ProChecker reports whether uid has Tailor It Pro. There is no server-side
// receipt check yet, so the default is "never"; plug a verifier in here once
// App Store Server API / RevenueCat validation exists.
type ProChecker interface {
	IsPro(ctx context.Context, uid string) bool
}

// Counter is one daily counter a request is charged against.
type Counter struct {
	ID    string
	Scope string
	Limit int
}

// Usage is what a Store reports back from Consume.
type Usage struct {
	// Counts per counter: after the charge when allowed, unchanged when blocked.
	Counts []int
	// Blocked is the index of the first counter that would overflow, or -1.
	Blocked int
}

// Store charges weight to every counter atomically, or to none of them when
// any counter would go over its limit.
type Store interface {
	Consume(ctx context.Context, counters []Counter, weight int, expireAt time.Time) (Usage, error)
}

// evaluate is the shared check-then-add rule of every Store.
func evaluate(counters []Counter, counts []int, weight int) Usage {
	for i, c := range counters {
		if counts[i]+weight > c.Limit {
			return Usage{Counts: counts, Blocked: i}
		}
	}
	out := make([]int, len(counts))
	for i := range counts {
		out[i] = counts[i] + weight
	}
	return Usage{Counts: out, Blocked: -1}
}

// Config configures a Limiter.
type Config struct {
	Limits  Limits
	Weights Weights
	// Env prefixes counter ids: staging and production share one Firestore.
	Env string
	// Location sets the day boundary; nil = JST.
	Location *time.Location
	// Now is the clock; nil = time.Now.
	Now func() time.Time
}

// Decision is the outcome of Allow.
type Decision struct {
	Allowed   bool
	Scope     string // counter that blocked (empty when allowed)
	Tier      Tier
	Limit     int // the user's daily limit (or the blocking counter's)
	Used      int
	Remaining int
	ResetAt   time.Time
}

// Limiter decides whether a request may run.
type Limiter struct {
	cfg      Config
	store    Store
	fallback Store
	pro      ProChecker
	log      *slog.Logger
}

// New builds a Limiter. When store fails (Firestore outage, missing IAM) the
// limiter keeps counting in memory per instance instead of failing open.
func New(cfg Config, store Store, pro ProChecker) *Limiter {
	if cfg.Location == nil {
		cfg.Location = JST
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if store == nil {
		store = NewMemory()
	}
	// Memory expires entries by its own clock; keep it on the limiter's.
	if m, ok := store.(*Memory); ok {
		m.now = cfg.Now
	}
	fallback := NewMemory()
	fallback.now = cfg.Now
	return &Limiter{cfg: cfg, store: store, fallback: fallback, pro: pro, log: slog.Default()}
}

// Config returns the effective configuration.
func (l *Limiter) Config() Config { return l.cfg }

func (l *Limiter) tier(ctx context.Context, id auth.Identity) Tier {
	switch {
	case id.Anonymous:
		return TierAnonymous
	case l.pro != nil && l.pro.IsPro(ctx, id.UID):
		return TierPro
	default:
		return TierSignedIn
	}
}

func (l *Limiter) userLimit(t Tier) int {
	switch t {
	case TierAnonymous:
		return l.cfg.Limits.Anonymous
	case TierPro:
		return l.cfg.Limits.Pro
	default:
		return l.cfg.Limits.SignedIn
	}
}

// Allow charges weight units for id (and its client ip) and reports whether
// the request may run. weight <= 0 is always allowed and charges nothing.
func (l *Limiter) Allow(ctx context.Context, id auth.Identity, ip string, weight int) (Decision, error) {
	now := l.cfg.Now().In(l.cfg.Location)
	day := now.Format("2006-01-02")
	y, m, d := now.Date()
	reset := time.Date(y, m, d+1, 0, 0, 0, 0, l.cfg.Location)
	tier := l.tier(ctx, id)
	limit := l.userLimit(tier)
	dec := Decision{Allowed: true, Tier: tier, Limit: limit, Remaining: limit, ResetAt: reset}
	if weight <= 0 {
		return dec, nil
	}

	prefix := l.cfg.Env
	if prefix == "" {
		prefix = "local"
	}
	counters := make([]Counter, 0, 3)
	if limit > 0 {
		counters = append(counters, Counter{ID: fmt.Sprintf("%s_%s_uid_%s", prefix, day, docSafe(id.UID)), Scope: ScopeUser, Limit: limit})
	}
	if id.Anonymous && l.cfg.Limits.PerIP > 0 && ip != "" {
		counters = append(counters, Counter{ID: fmt.Sprintf("%s_%s_ip_%s", prefix, day, ipKey(ip)), Scope: ScopeIP, Limit: l.cfg.Limits.PerIP})
	}
	if l.cfg.Limits.Global > 0 {
		counters = append(counters, Counter{ID: fmt.Sprintf("%s_%s_global", prefix, day), Scope: ScopeGlobal, Limit: l.cfg.Limits.Global})
	}
	if len(counters) == 0 {
		return dec, nil
	}

	// TTL: keep a day of slack so the document outlives its own day.
	expireAt := reset.Add(24 * time.Hour)
	usage, err := l.store.Consume(ctx, counters, weight, expireAt)
	if err != nil {
		l.log.WarnContext(ctx, "ratelimit: store failed; counting in memory", slog.Any("error", err))
		usage, err = l.fallback.Consume(ctx, counters, weight, expireAt)
		if err != nil {
			return dec, err
		}
	}
	if usage.Blocked >= 0 {
		c := counters[usage.Blocked]
		dec.Allowed = false
		dec.Scope = c.Scope
		dec.Limit = c.Limit
		dec.Used = usage.Counts[usage.Blocked]
		dec.Remaining = max(0, c.Limit-dec.Used)
		return dec, nil
	}
	if counters[0].Scope == ScopeUser {
		dec.Used = usage.Counts[0]
		dec.Remaining = max(0, limit-dec.Used)
	}
	return dec, nil
}

// docSafe keeps a Firestore document id legal ("/" is the only banned rune).
func docSafe(s string) string {
	return strings.ReplaceAll(s, "/", "_")
}

// ipKey hashes the address (no raw IPs at rest). IPv6 is grouped by /64:
// one device/home usually owns a whole /64 and can rotate inside it.
func ipKey(ip string) string {
	if strings.Contains(ip, ":") {
		parts := strings.Split(ip, ":")
		if len(parts) > 4 {
			parts = parts[:4]
		}
		ip = strings.Join(parts, ":") + "::/64"
	}
	sum := sha256.Sum256([]byte(ip))
	return hex.EncodeToString(sum[:8])
}
