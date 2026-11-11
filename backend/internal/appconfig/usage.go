package appconfig

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/plan"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Usage (#317 #630, docs/backend/rate-limit.md "今の使用量"): what the
// caller used today and may still use, so the app can say "今日の残り N"
// and "上限に近い" before a request is refused.
//
//	GET /v1/me/usage             plan, AI units of today (ai's counter), apps
//	GET /v1/apps/{appId}/usage   records and photos of one app, against the
//	                             payer's plan (the creator's for a shared app)
//
// Reads are cheap: the AI units are one document read (the counter
// internal/ratelimit writes), the counts are cached per instance for
// usageCountTTL (count aggregations, one per entity and photo field).

// usageCountTTL is how long a count may lag behind (per instance).
const usageCountTTL = 30 * time.Second

// UsageConfig is how appconfig reads the ai service's daily units.
type UsageConfig struct {
	// Rate is ai's RATE_LIMIT_* (ratelimit.ConfigFromEnv): limits and APP_ENV.
	Rate ratelimit.Config
	// Counters reads the rateLimits documents; nil = "ai" is not answered.
	Counters ratelimit.Peeker
	// Now is the clock; nil = time.Now.
	Now func() time.Time
}

// WithUsage answers the AI units of GET /v1/me/usage from c.
func WithUsage(c UsageConfig) Option {
	return func(h *Handler) { h.usage = newUsageState(c) }
}

type usageState struct {
	cfg    UsageConfig
	counts *ttlCache[usageCounts]
}

type usageCounts struct{ a, b int }

func newUsageState(c UsageConfig) *usageState {
	if c.Now == nil {
		c.Now = time.Now
	}
	return &usageState{cfg: c, counts: newTTLCache[usageCounts](usageCountTTL, 4096, c.Now)}
}

// routes
func init() {
	registerRoutes(func(h *Handler, mux *http.ServeMux) {
		if h.usage == nil {
			h.usage = newUsageState(UsageConfig{})
		}
		mux.HandleFunc("GET /v1/me/usage", h.getMyUsage)
		mux.HandleFunc("GET /v1/apps/{appId}/usage", h.getAppUsage)
	})
}

// UsageMeter is one used / limit pair. Limit and Remaining are null when
// the limit is not counted (0 in the env, or PLAN_LIMITS_MODE=off).
type UsageMeter struct {
	Used      int  `json:"used"`
	Limit     *int `json:"limit"`
	Remaining *int `json:"remaining"`
}

func meter(used, limit int, counted bool) *UsageMeter {
	m := &UsageMeter{Used: used}
	if counted && limit > 0 {
		rem := max(0, limit-used)
		m.Limit, m.Remaining = &limit, &rem
	}
	return m
}

// AIUsage is today's units of generate, chat and text together (one pool,
// weighed as ratelimit.Weights). The same numbers as X-RateLimit-*.
type AIUsage struct {
	UsageMeter
	Weights UsageWeights `json:"weights"`
}

// UsageWeights are the units one request costs (RATE_LIMIT_WEIGHT_*).
type UsageWeights struct {
	Create    int `json:"create"`
	Edit      int `json:"edit"`
	Chat      int `json:"chat"`
	Text      int `json:"text"`
	TextTask  int `json:"textTask"`
	TextLong  int `json:"textLong"`
	TextImage int `json:"textImage"`
}

// MyUsage is GET /v1/me/usage.
type MyUsage struct {
	Plan    plan.Plan      `json:"plan"`
	Tier    ratelimit.Tier `json:"tier"`
	ResetAt time.Time      `json:"resetAt"`
	// AI is null when this appconfig cannot read the ai counters.
	AI *AIUsage `json:"ai"`
	// Apps are the apps held (own + joined shared ones) against the plan's
	// base plus the bought slots (#630 app slots).
	Apps *UsageMeter `json:"apps"`
	// AppSlots is how Apps.limit is made up (null when apps are not counted).
	AppSlots *AppSlotsUsage `json:"appSlots"`
	// PerApp are the caller's plan's per-app limits (null = not counted).
	PerApp map[string]*int `json:"perApp"`
}

// AppSlotsUsage is MyUsage.appSlots: limit = base + purchased.
type AppSlotsUsage struct {
	Base      int `json:"base"`
	Purchased int `json:"purchased"`
	Limit     int `json:"limit"`
	// CanBuySlot: one more slot can be bought now for SlotCoins coins
	// (COIN_BILLING_MODE=enforce and COIN_PRICE_APP_SLOT > 0).
	CanBuySlot bool  `json:"canBuySlot"`
	SlotCoins  int64 `json:"slotCoins"`
}

// AppUsage is GET /v1/apps/{appId}/usage.
type AppUsage struct {
	AppID string    `json:"appId"`
	Plan  plan.Plan `json:"plan"`
	// Payer is "self" (the caller's own app) or "creator" (someone else's).
	Payer   string      `json:"payer"`
	Upgrade bool        `json:"upgrade"`
	Records *UsageMeter `json:"records"`
	Photos  *UsageMeter `json:"photos"`
}

func (h *Handler) planTable() (plan.Table, bool) {
	if h.planLimits == nil {
		return plan.Default, false
	}
	return *h.planLimits, h.planLimits.Mode.Checks()
}

func (h *Handler) getMyUsage(w http.ResponseWriter, r *http.Request) {
	ctx, uid := r.Context(), uidOf(r)
	u := h.usage
	p := h.planOf(ctx, uid)
	tier := ratelimit.TierOf(p == plan.Pro, auth.Anonymous(ctx))
	day, reset := u.cfg.Rate.Today(u.cfg.Now())
	t, counted := h.planTable()
	limits := t.For(p)
	out := MyUsage{Plan: p, Tier: tier, ResetAt: reset.UTC(), PerApp: map[string]*int{
		plan.RecordsPerApp: limitOrNil(limits.RecordsPerApp, counted),
		plan.PhotosPerApp:  limitOrNil(limits.PhotosPerApp, counted),
	}}
	if pk := u.cfg.Counters; pk != nil {
		// one document read, not cached: it moves with every AI request
		if n, err := pk.Peek(ctx, []ratelimit.Counter{{ID: u.cfg.Rate.UserCounterID(day, uid)}}); err != nil {
			slog.WarnContext(ctx, "appconfig: usage: ai units read failed", slog.Any("error", err))
		} else {
			lim := u.cfg.Rate.DailyLimit(tier)
			out.AI = &AIUsage{UsageMeter: *meter(n[0], lim, true), Weights: UsageWeights(u.cfg.Rate.Weights)}
		}
	}
	if c, ok := h.store.(store.Counter); ok {
		n, err := u.counts.get("apps/"+uid, func() (usageCounts, error) {
			n, err := c.CountApps(ctx, uid)
			return usageCounts{a: n}, err
		})
		if err != nil {
			slog.WarnContext(ctx, "appconfig: usage: apps count failed", slog.Any("error", err))
		} else {
			limit := limits.Apps
			if counted && limit > 0 {
				l := h.heldLimitOf(ctx, uid)
				limit = l.limit()
				out.AppSlots = &AppSlotsUsage{Base: l.base, Purchased: l.purchased, Limit: limit, CanBuySlot: l.price > 0, SlotCoins: l.price}
			}
			out.Apps = meter(n.a, limit, counted)
		}
	}
	w.Header().Set("Cache-Control", "private, no-store")
	httpx.WriteJSON(w, http.StatusOK, out)
}

// raises reports whether Pro has more of resource than the free plan (the
// upgrade of plan_limit_reached).
func raises(t plan.Table, resource string) bool {
	pro, free := t.Pro.Of(resource), t.Free.Of(resource)
	return free > 0 && (pro <= 0 || pro > free)
}

func limitOrNil(n int, counted bool) *int {
	if !counted || n <= 0 {
		return nil
	}
	return &n
}

// getAppUsage counts what the plan limits count on the caller's next write
// (checkRecordLimits): the caller's own records plus the shared ones it may
// read, against the payer's plan. A participant sees only the numbers of its
// own view, the creator's plan name and limits — never the creator's id.
func (h *Handler) getAppUsage(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	ctx, uid := r.Context(), uidOf(r)
	cur, err := h.store.GetApp(ctx, uid, id) // no copy is made here (unlike ensureApp)
	if err != nil {
		fail(w, mapStoreErr(err, "app"))
		return
	}
	payer, err := h.appOwner(ctx, id)
	if err != nil {
		fail(w, err)
		return
	}
	if payer == "" {
		payer = uid
	}
	p := h.planOf(ctx, payer)
	t, counted := h.planTable()
	limits := t.For(p)
	out := AppUsage{AppID: id, Plan: p, Payer: "self"}
	if payer != uid {
		out.Payer = "creator"
	}
	out.Upgrade = p == plan.Free && (raises(t, plan.RecordsPerApp) || raises(t, plan.PhotosPerApp))
	if c, ok := h.store.(store.Counter); ok {
		key := "app/" + uid + "/" + id + "/" + strconv.Itoa(cur.CurrentVersion)
		n, err := h.usage.counts.get(key, func() (usageCounts, error) { return h.countVisible(r, c, cur) })
		if err != nil {
			slog.WarnContext(ctx, "appconfig: usage: records count failed", slog.Any("error", err))
		} else {
			out.Records = meter(n.a, limits.RecordsPerApp, counted)
			out.Photos = meter(n.b, limits.PhotosPerApp, counted)
		}
	}
	w.Header().Set("Cache-Control", "private, no-store")
	httpx.WriteJSON(w, http.StatusOK, out)
}

// countVisible is countAppRecords over the entities the caller may read:
// sharing as published by the creator, scope:"shared" only for participants
// (a non-member's count would reveal another group's data size).
func (h *Handler) countVisible(r *http.Request, c store.Counter, cur *store.App) (usageCounts, error) {
	var n usageCounts
	app, err := h.parsedDSL(uidOf(r), cur)
	if err != nil {
		return n, err
	}
	rules, err := h.sharingRulesFor(r, cur)
	if err != nil {
		return n, err
	}
	member := memberOnce(func() (bool, error) {
		role, err := h.memberRole(r.Context(), cur.ID, uidOf(r))
		return role != "", err
	})
	for i := range app.Entities {
		e, err := rules.entity(&app.Entities[i])
		if err != nil {
			continue // a shared entity the creator has not published: not counted on write either
		}
		if e.EffectiveScope() == dsl.ScopeShared {
			ok, err := member()
			if err != nil {
				return n, err
			}
			if !ok {
				continue
			}
		}
		records, photos, err := countEntity(r.Context(), c, recordSet(r, cur, e, e.ID), e)
		if err != nil {
			return n, err
		}
		n.a += records
		n.b += photos
	}
	return n, nil
}

func countEntity(ctx context.Context, c store.Counter, rs store.RecordSet, e *dsl.Entity) (records, photos int, err error) {
	if records, err = c.CountRecords(ctx, rs, ""); err != nil || records == 0 {
		return records, 0, err
	}
	for j := range e.Fields {
		if e.Fields[j].Type != dsl.FieldPhoto {
			continue
		}
		p, err := c.CountRecords(ctx, rs, e.Fields[j].ID)
		if err != nil {
			return 0, 0, err
		}
		photos += p
	}
	return records, photos, nil
}

func memberOnce(f func() (bool, error)) func() (bool, error) {
	var (
		done bool
		ok   bool
		err  error
	)
	return func() (bool, error) {
		if !done {
			ok, err = f()
			done = true
		}
		return ok, err
	}
}

// ttlCache keeps values for ttl, at most max keys (all dropped when full).
type ttlCache[V any] struct {
	mu  sync.Mutex
	m   map[string]ttlEntry[V]
	ttl time.Duration
	max int
	now func() time.Time
}

type ttlEntry[V any] struct {
	v     V
	until time.Time
}

func newTTLCache[V any](ttl time.Duration, maxKeys int, now func() time.Time) *ttlCache[V] {
	return &ttlCache[V]{m: map[string]ttlEntry[V]{}, ttl: ttl, max: maxKeys, now: now}
}

// get returns the cached value of key or loads it (errors are not cached).
func (c *ttlCache[V]) get(key string, load func() (V, error)) (V, error) {
	now := c.now()
	c.mu.Lock()
	if e, ok := c.m[key]; ok && now.Before(e.until) {
		c.mu.Unlock()
		return e.v, nil
	}
	c.mu.Unlock()
	v, err := load()
	if err != nil {
		return v, err
	}
	c.mu.Lock()
	if len(c.m) >= c.max {
		c.m = map[string]ttlEntry[V]{}
	}
	c.m[key] = ttlEntry[V]{v: v, until: now.Add(c.ttl)}
	c.mu.Unlock()
	return v, nil
}
