package extapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// Call-count limits (#1715): a runaway brake apart from `runs` and coins.
// Every call that reaches a provider counts once against the user's and the
// app's hourly and daily counters of that provider and the provider's
// environment-wide daily counter (all or none, ratelimit.Store). A provider
// that answered 429 is paused for UpstreamPause on this instance: for the
// person whose token it was (NeedsOAuth: the provider counts per token), for
// everyone otherwise (the operator's key). A limited call is 429
// rate_limited with the scope (LimitError).

// The scopes of LimitError (the answer's "scope" and the ext.call log's "limit").
const (
	LimitUserHourly     = "user_hourly"
	LimitUserDaily      = "user_daily"
	LimitAppHourly      = "app_hourly"
	LimitAppDaily       = "app_daily"
	LimitGlobalDaily    = "global_daily"
	LimitProviderPaused = "provider_paused"
)

// UpstreamPause is how long a provider is not called after it answered 429.
const UpstreamPause = time.Minute

// Limits are the calls of one provider per window. 0 = no counter.
type Limits struct {
	UserHourly  int
	UserDaily   int
	AppHourly   int
	AppDaily    int
	GlobalDaily int
}

// DefaultLimits apply when no env is set: low enough to stay under a
// provider's free allocation, high enough for a person pressing buttons.
// The real values are the owner's (env, not written in the docs). A
// NeedsOAuth provider has no GlobalDaily by default (its allocation is per
// person's token): only EXT_GLOBAL_DAILY_<PROVIDER> sets one.
var DefaultLimits = Limits{UserHourly: 30, UserDaily: 100, AppHourly: 100, AppDaily: 300, GlobalDaily: 500}

// limitEnv are the scopes of EXT_LIMIT_<SCOPE> / EXT_LIMIT_<PROVIDER>_<SCOPE>.
var limitEnv = []struct {
	name string
	of   func(*Limits) *int
}{
	{"USER_HOURLY", func(l *Limits) *int { return &l.UserHourly }},
	{"USER_DAILY", func(l *Limits) *int { return &l.UserDaily }},
	{"APP_HOURLY", func(l *Limits) *int { return &l.AppHourly }},
	{"APP_DAILY", func(l *Limits) *int { return &l.AppDaily }},
}

// limitsFromEnv is every provider's Limits:
//
//	EXT_LIMIT_<SCOPE>               every provider (SCOPE = USER_HOURLY / USER_DAILY / APP_HOURLY / APP_DAILY)
//	EXT_LIMIT_<PROVIDER>_<SCOPE>    one provider (wins over the above)
//	EXT_GLOBAL_DAILY                every provider's environment-wide day (not NeedsOAuth ones)
//	EXT_GLOBAL_DAILY_<PROVIDER>     one provider's (wins over the above)
//
// Unset = DefaultLimits (GlobalDaily: 0 for NeedsOAuth, Provider.GlobalDaily
// when the provider has one); 0 = that counter is off.
func limitsFromEnv(getenv func(string) string) (map[string]Limits, error) {
	base := DefaultLimits
	for _, s := range limitEnv {
		if err := envCount(getenv, "EXT_LIMIT_"+s.name, s.of(&base)); err != nil {
			return nil, err
		}
	}
	if err := envCount(getenv, "EXT_GLOBAL_DAILY", &base.GlobalDaily); err != nil {
		return nil, err
	}
	globalSet := strings.TrimSpace(getenv("EXT_GLOBAL_DAILY")) != ""
	out := map[string]Limits{}
	for _, p := range Providers() {
		l, n := base, p.EnvName()
		switch {
		case p.NeedsOAuth:
			l.GlobalDaily = 0
		case p.GlobalDaily > 0 && !globalSet:
			l.GlobalDaily = p.GlobalDaily
		}
		for _, s := range limitEnv {
			if err := envCount(getenv, "EXT_LIMIT_"+n+"_"+s.name, s.of(&l)); err != nil {
				return nil, err
			}
		}
		if err := envCount(getenv, "EXT_GLOBAL_DAILY_"+n, &l.GlobalDaily); err != nil {
			return nil, err
		}
		out[p.ID] = l
	}
	return out, nil
}

// envCount sets *dst from key when it is set (an integer ≥ 0).
func envCount(getenv func(string) string, key string, dst *int) error {
	v := strings.TrimSpace(getenv(key))
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return fmt.Errorf("%s must be an integer ≥ 0, got %q", key, v)
	}
	*dst = n
	return nil
}

// LimitError is a call a limit stopped (429 rate_limited).
type LimitError struct {
	// Scope is one of the Limit* scopes.
	Scope string
	// Limit and Count are the counter's cap and its count (0 for a pause).
	Limit, Count int
	// RetryAfter is until the window resets (or the pause ends).
	RetryAfter time.Duration
}

func (e *LimitError) Error() string { return "extapi: limit: " + e.Scope }

// RetryAfterSec is RetryAfter in whole seconds (at least 1).
func (e *LimitError) RetryAfterSec() int {
	return max(1, int((e.RetryAfter+time.Second-1)/time.Second))
}

// limiter counts calls (ratelimit.Store; the memory fallback when the
// store fails, like ratelimit.Limiter) and keeps the provider pauses.
type limiter struct {
	store    ratelimit.Store
	fallback ratelimit.Store

	mu sync.Mutex
	// paused is until when a pause key (pauseKey) is not called.
	paused map[string]time.Time
}

// pauseKey is what a provider's 429 pauses: the person's own token for a
// NeedsOAuth provider (uid + provider), the provider for everyone otherwise.
func pauseKey(p Provider, uid string) string {
	if p.NeedsOAuth {
		return p.ID + "\x00uid_" + uid
	}
	return p.ID
}

// limitCharge is what consume wrote, for refund.
type limitCharge struct {
	store    ratelimit.Store
	counters []ratelimit.Counter
	expireAt time.Time
}

// Monthly use (#2144): every call counted above also counts once on the
// user's month of the provider, shown by GET /v1/me/connections. It is the
// last counter of the same Consume (all or none with the limits, refunded
// with them, counted in the fallback when the store fails) and never a
// limit (Limit is MaxInt32).
//
//	rateLimits/{env}_{yyyy-mm}_ext_{provider}_uid_{uid}   expires a day after the month
const scopeUserMonthly = "user_monthly"

func (s *Service) monthCounter(provider, uid string, now time.Time) ratelimit.Counter {
	env := s.cfg.Env
	if env == "" {
		env = "local"
	}
	return ratelimit.Counter{ID: fmt.Sprintf("%s_%s_ext_%s_uid_%s", env, now.In(ratelimit.JST).Format("2006-01"), provider, docSafe(uid)),
		Scope: scopeUserMonthly, Limit: math.MaxInt32}
}

// MonthCalls is uid's calls of each provider this month (JST), read
// without writing (one batched read). A store that cannot peek is an error.
func (s *Service) MonthCalls(ctx context.Context, uid string, providers []string) (map[string]int, error) {
	peeker, ok := s.limits.store.(ratelimit.Peeker)
	if !ok {
		return nil, errors.New("extapi: the limit store cannot peek")
	}
	if uid == "" || len(providers) == 0 {
		return map[string]int{}, nil
	}
	now := s.now()
	counters := make([]ratelimit.Counter, len(providers))
	for i, p := range providers {
		counters[i] = s.monthCounter(p, uid, now)
	}
	counts, err := peeker.Peek(ctx, counters)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(providers))
	for i, p := range providers {
		out[p] = counts[i]
	}
	return out, nil
}

// addMonthCalls fills each connection's MonthCalls (left absent when the
// counts cannot be read: the list itself still answers).
func (s *Service) addMonthCalls(ctx context.Context, uid string, conns []ConnectionView) {
	ids := make([]string, len(conns))
	for i, c := range conns {
		ids[i] = c.Provider
	}
	counts, err := s.MonthCalls(ctx, uid, ids)
	if err != nil {
		s.Log.WarnContext(ctx, "extapi: reading the month counts failed", slog.Any("error", err))
		return
	}
	for i := range conns {
		n := counts[conns[i].Provider]
		conns[i].MonthCalls = &n
	}
}

// UseLimitStore counts the call limits in store (Firestore `rateLimits` in
// the runner, shared by every instance). Without it they are per instance.
func (s *Service) UseLimitStore(store ratelimit.Store) {
	if store != nil {
		s.limits.store = store
	}
}

// pausedFor is how long key is still paused (0 = not paused).
func (l *limiter) pausedFor(key string, now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	until, ok := l.paused[key]
	if !ok {
		return 0
	}
	if !now.Before(until) {
		delete(l.paused, key)
		return 0
	}
	return until.Sub(now)
}

// pause stops calls under key until now + d. Ended pauses go at the same
// time (per-person keys would otherwise pile up).
func (l *limiter) pause(key string, now time.Time, d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.paused == nil {
		l.paused = map[string]time.Time{}
	}
	for k, until := range l.paused {
		if !now.Before(until) {
			delete(l.paused, k)
		}
	}
	l.paused[key] = now.Add(d)
}

// consume counts one call of provider for uid and app. A blocked call
// charges nothing and is a *LimitError.
func (s *Service) consumeLimits(ctx context.Context, provider, uid, app string) (limitCharge, error) {
	lim := s.cfg.Limits[provider]
	now := s.now().In(ratelimit.JST)
	y, m, d := now.Date()
	dayEnd := time.Date(y, m, d+1, 0, 0, 0, 0, ratelimit.JST)
	hourEnd := now.Truncate(time.Hour).Add(time.Hour)
	env := s.cfg.Env
	if env == "" {
		env = "local"
	}
	day, hour := now.Format("2006-01-02"), now.Format("2006-01-02T15")
	type ctr struct {
		c     ratelimit.Counter
		reset time.Time
	}
	var cs []ctr
	add := func(scope, window, key string, n int, reset time.Time) {
		if n > 0 {
			cs = append(cs, ctr{ratelimit.Counter{ID: fmt.Sprintf("%s_%s_ext_%s_%s", env, window, provider, key), Scope: scope, Limit: n}, reset})
		}
	}
	if uid != "" {
		k := "uid_" + docSafe(uid)
		add(LimitUserHourly, hour, k, lim.UserHourly, hourEnd)
		add(LimitUserDaily, day, k, lim.UserDaily, dayEnd)
	}
	if app != "" {
		k := "app_" + docSafe(app)
		add(LimitAppHourly, hour, k, lim.AppHourly, hourEnd)
		add(LimitAppDaily, day, k, lim.AppDaily, dayEnd)
	}
	if lim.GlobalDaily > 0 {
		cs = append(cs, ctr{ratelimit.Counter{ID: fmt.Sprintf("%s_%s_ext_%s_global", env, day, provider), Scope: LimitGlobalDaily,
			Limit: lim.GlobalDaily, Shards: ratelimit.GlobalShards}, dayEnd})
	}
	// TTL: a day of slack so the document outlives its own window
	expireAt := dayEnd.Add(24 * time.Hour)
	if uid != "" {
		monthEnd := time.Date(y, m+1, 1, 0, 0, 0, 0, ratelimit.JST)
		cs = append(cs, ctr{s.monthCounter(provider, uid, now), monthEnd})
		// one expireAt per Consume: the month's, so its document outlives the month
		expireAt = monthEnd.Add(24 * time.Hour)
	}
	if len(cs) == 0 {
		return limitCharge{}, nil
	}
	counters := make([]ratelimit.Counter, len(cs))
	for i, c := range cs {
		counters[i] = c.c
	}
	store := s.limits.store
	u, err := store.Consume(ctx, counters, 1, expireAt)
	if err != nil {
		s.Log.WarnContext(ctx, "extapi: limit store failed; counting in memory", slog.String("provider", provider), slog.Any("error", err))
		store = s.limits.fallback
		if u, err = store.Consume(ctx, counters, 1, expireAt); err != nil {
			return limitCharge{}, err
		}
	}
	if u.Blocked >= 0 {
		c := cs[u.Blocked]
		return limitCharge{}, &LimitError{Scope: c.c.Scope, Limit: c.c.Limit, Count: u.Counts[u.Blocked], RetryAfter: c.reset.Sub(now)}
	}
	if u.Charged != nil {
		counters = u.Charged
	}
	return limitCharge{store: store, counters: counters, expireAt: expireAt}, nil
}

// refundLimits gives back a call that did not reach the provider.
func (s *Service) refundLimits(ctx context.Context, provider string, c limitCharge) {
	if c.store == nil {
		return
	}
	if err := c.store.Refund(context.WithoutCancel(ctx), c.counters, 1, c.expireAt); err != nil {
		s.Log.WarnContext(ctx, "extapi: refunding the limit failed", slog.String("provider", provider), slog.Any("error", err))
	}
}

// upstreamLimited reports whether err is the provider's own 429.
func upstreamLimited(err error) bool {
	var pe *ProviderError
	return errors.As(err, &pe) && pe.Status == 429
}

// docSafe keeps a Firestore document id legal ("/" is the only banned rune).
func docSafe(s string) string { return strings.ReplaceAll(s, "/", "_") }
