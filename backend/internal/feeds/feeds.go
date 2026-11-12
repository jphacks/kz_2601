// Package feeds fetches read-only public data for the DSL feed leaf (#348)
// from a fixed allowlist of sources. The caller (appconfig GET /v1/feeds/...)
// passes a provider and that provider's typed parameter; every outgoing URL is
// built here from constants, so no user input ever becomes a host or a path
// segment outside the allowlist. Requests go through a client that refuses
// non-public addresses, have a timeout and a body cap, and are cached so the
// same question reaches the source at most once per TTL per instance.
//
// docs/backend/features/external-feeds.md
package feeds

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Item is one row of a feed. Fields a provider does not use are omitted.
type Item struct {
	Date     string    `json:"date,omitempty"` // YYYY-MM-DD (weather, holidays, diet) or RFC 3339 (rss, arxiv, warnings, pressure, status)
	Title    string    `json:"title"`
	Subtitle string    `json:"subtitle,omitempty"`
	Icon     string    `json:"icon,omitempty"`   // SF Symbols name (weather, warnings, status)
	URL      string    `json:"url,omitempty"`    // https only (rss, arxiv, diet, status)
	Max      *float64  `json:"max,omitempty"`    // °C (weather) / hPa (pressure)
	Min      *float64  `json:"min,omitempty"`    // °C (weather) / hPa (pressure)
	Pop      *int      `json:"pop,omitempty"`    // precipitation probability %
	Drop     *bool     `json:"drop,omitempty"`   // pressure fell 6 hPa or more within 24 hours (pressure)
	Series   []float64 `json:"series,omitempty"` // hourly hPa in the row (pressure)
}

// Source is the attribution shown under the list (PDL1.0 asks for it).
type Source struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	License string `json:"license"`
}

// Result is the body of GET /v1/feeds/{provider}.
type Result struct {
	Provider  string    `json:"provider"`
	Title     string    `json:"title"`
	Items     []Item    `json:"items"`
	Source    Source    `json:"source"`
	FetchedAt time.Time `json:"fetchedAt"`
	Stale     bool      `json:"stale,omitempty"` // the source failed; this is the last good copy
}

// Query names what to read. Region is for weather / warnings / pressure,
// FeedID for rss / arxiv / status, Q (search words) for diet. UID and IP (a
// hashed client address) are the caller, not part of the cache key: they
// count the source fetches search words cause.
type Query struct {
	Provider string
	Region   string
	FeedID   string
	Q        string
	UID      string
	IP       string
}

func (q Query) key() string { return q.Provider + "|" + q.Region + "|" + q.FeedID + "|" + q.Q }

// Errors returned by Get.
var (
	ErrUnknown  = errors.New("feeds: not on the allowlist")
	ErrDisabled = errors.New("feeds: provider disabled")
	ErrBusy     = errors.New("feeds: outgoing request budget used up")
	ErrUpstream = errors.New("feeds: source unavailable")
	// ErrRecentFailure is ErrUpstream served from the cache: the source
	// failed less than FailTTL ago and was not asked again. A caller that
	// retries should wait FailTTL (the runner's saveFeed defers, feed.go).
	ErrRecentFailure = fmt.Errorf("%w (failed recently, cached)", ErrUpstream)
	// ErrUserLimit: the caller's search words reached the source too often
	// this hour (cached answers are still served).
	ErrUserLimit = errors.New("feeds: per-user source fetches used up")
)

// Per-provider limits.
type limits struct {
	ttl     time.Duration // fresh cache lifetime
	maxBody int64         // bytes read from the source
	// gap > 0: the source asks for one request at a time, this far apart
	// (国会会議録: 数秒). Enforced per instance, so arXiv's 3 s is multiplied
	// by the number of appconfig instances (arxivGap).
	gap time.Duration
}

// appconfigMaxInstances is --max-instances of appconfig in
// .github/workflows/deploy-cloudrun.yml. Change both together.
const appconfigMaxInstances = 3

// arxivGap keeps arXiv's 3 s between requests even when every instance
// fetches at once: each instance waits 3 s × the instance count.
const arxivGap = 3 * time.Second * appconfigMaxInstances

var providerLimits = map[string]limits{
	dsl.FeedWeather:  {ttl: 30 * time.Minute, maxBody: 256 << 10},
	dsl.FeedHolidays: {ttl: 12 * time.Hour, maxBody: 128 << 10},
	dsl.FeedRSS:      {ttl: 15 * time.Minute, maxBody: 1 << 20}, // go-blog carries full posts (~360 KB for 10)
	dsl.FeedWarnings: {ttl: 10 * time.Minute, maxBody: 512 << 10},
	dsl.FeedPressure: {ttl: 30 * time.Minute, maxBody: 256 << 10},
	dsl.FeedArxiv:    {ttl: 3 * time.Hour, maxBody: 1 << 20, gap: arxivGap},
	dsl.FeedDiet:     {ttl: 3 * time.Hour, maxBody: 3 << 20, gap: 3 * time.Second},
	// Cloudflare's summary is ~230 KB on a calm day (480 components) and
	// grows with open incidents.
	dsl.FeedStatus: {ttl: 5 * time.Minute, maxBody: 2 << 20},
}

const (
	// FailTTL keeps a failure for a while so a broken source is not retried
	// by every request (ErrRecentFailure until it passes).
	FailTTL = time.Minute
	// staleFor is how long a last good copy may stand in for a failing source.
	staleFor = 24 * time.Hour
	// maxStoredItems caps what one cached entry keeps.
	maxStoredItems = 60
	// maxCacheEntries caps the cache (diet keys are free search words).
	maxCacheEntries = 2000
	// maxGapWait is how long a request waits for a gap provider's turn
	// (in line behind the others, first come first served) before ErrBusy.
	maxGapWait = 10 * time.Second
	// DefaultUpstreamPerMinute caps fetches to all sources but diet per instance.
	DefaultUpstreamPerMinute = 30
	// DefaultDietPerMinute is diet's own cap: free search words miss the
	// cache more often and must not use up the budget of the other feeds.
	DefaultDietPerMinute = 10
	// DefaultUserMissesPerHour caps, per user and instance, the source
	// fetches of a provider that takes search words (diet).
	DefaultUserMissesPerHour = 20
	// DefaultIPMissesPerHour is the same per client address (new anonymous
	// uids share one address; several users behind one address get room).
	DefaultIPMissesPerHour = 60
)

// Service is safe for concurrent use.
type Service struct {
	client         *http.Client
	ep             Endpoints
	now            func() time.Time
	log            *slog.Logger                              // nil: slog.Default()
	sleep          func(context.Context, time.Duration) bool // sleepCtx; tests move a fake clock
	disabled       map[string]bool
	userMissesHour int
	ipMissesHour   int

	mu       sync.Mutex
	cache    map[string]*entry
	inflight map[string]*call
	budgets  map[string]*budget       // per-minute caps: "" = shared, dsl.FeedDiet = diet's own
	turns    map[string]chan struct{} // one fetch at a time per gap provider, in arrival order
	lastAt   map[string]time.Time     // last fetch start per provider
	misses   hourly                   // source fetches per "u:"uid / "i:"ip (search-word providers)
}

// budget is a fixed per-minute window of upstream fetches.
type budget struct {
	limit int
	start time.Time
	used  int
}

// hourly is a fixed hourly per-uid count.
type hourly struct {
	start  time.Time
	counts map[string]int
}

type entry struct {
	items     []Item
	title     string
	fetchedAt time.Time
	expires   time.Time
	failedAt  time.Time // zero = last fetch succeeded
}

// call is one fetch in flight; waiters read its result after done.
type call struct {
	done  chan struct{}
	e     *entry
	stale bool
	err   error
}

// Options configures New. Zero values mean production defaults.
type Options struct {
	Client    *http.Client     // default: NewClient() (public addresses only)
	Endpoints *Endpoints       // default: DefaultEndpoints()
	Now       func() time.Time // default: time.Now
	Disabled  []string         // providers switched off (kill switch)
	PerMinute int              // upstream fetches per minute; 0 = DefaultUpstreamPerMinute
	// DietPerMinute is diet's own per-minute cap; 0 = DefaultDietPerMinute.
	DietPerMinute int
	// UserMissesPerHour caps a user's search-word fetches; 0 = DefaultUserMissesPerHour.
	UserMissesPerHour int
	// IPMissesPerHour caps a client address's search-word fetches; 0 = DefaultIPMissesPerHour.
	IPMissesPerHour int
	Logger          *slog.Logger // feeds.fetch lines; default: slog.Default()
}

// New builds a Service.
func New(o Options) *Service {
	s := &Service{client: o.Client, now: o.Now, log: o.Logger, disabled: map[string]bool{},
		cache: map[string]*entry{}, inflight: map[string]*call{},
		turns: map[string]chan struct{}{}, lastAt: map[string]time.Time{},
		userMissesHour: cmp.Or(max(o.UserMissesPerHour, 0), DefaultUserMissesPerHour),
		ipMissesHour:   cmp.Or(max(o.IPMissesPerHour, 0), DefaultIPMissesPerHour),
		budgets: map[string]*budget{
			"":           {limit: cmp.Or(max(o.PerMinute, 0), DefaultUpstreamPerMinute)},
			dsl.FeedDiet: {limit: cmp.Or(max(o.DietPerMinute, 0), DefaultDietPerMinute)},
		}}
	for p, l := range providerLimits {
		if l.gap > 0 {
			s.turns[p] = make(chan struct{}, 1)
		}
	}
	if s.client == nil {
		s.client = NewClient()
	}
	if o.Endpoints != nil {
		s.ep = *o.Endpoints
	} else {
		s.ep = DefaultEndpoints()
	}
	// Redirects may only stay on an allowlisted https host.
	c := *s.client
	c.CheckRedirect = s.ep.checkRedirect
	s.client = &c
	if s.now == nil {
		s.now = time.Now
	}
	s.sleep = sleepCtx
	for _, p := range o.Disabled {
		if p = strings.TrimSpace(p); p != "" {
			s.disabled[p] = true
		}
	}
	return s
}

// OptionsFromEnv reads FEEDS_DISABLED (comma separated providers, "all" =
// every one) and FEEDS_UPSTREAM_PER_MINUTE.
func OptionsFromEnv(getenv func(string) string) Options {
	o := Options{}
	for _, p := range strings.Split(getenv("FEEDS_DISABLED"), ",") {
		p = strings.TrimSpace(p)
		if p == "all" {
			for q := range dsl.FeedProviders {
				o.Disabled = append(o.Disabled, q)
			}
			continue
		}
		if p != "" {
			o.Disabled = append(o.Disabled, p)
		}
	}
	if n, err := parsePositive(getenv("FEEDS_UPSTREAM_PER_MINUTE")); err == nil {
		o.PerMinute = n
	}
	return o
}

func parsePositive(s string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n); err != nil || n <= 0 {
		return 0, errors.New("not a positive integer")
	}
	return n, nil
}

// Enabled reports whether a provider is on the allowlist and not switched off.
func (s *Service) Enabled(provider string) bool {
	return dsl.FeedProviders[provider] && !s.disabled[provider]
}

// Get returns up to limit items for q (limit <= 0 = dsl.FeedDefaultLimit).
func (s *Service) Get(ctx context.Context, q Query, limit int) (*Result, error) {
	if q.Provider == dsl.FeedDiet {
		q.Q, _ = dsl.NormalizeFeedQuery(q.Q) // "" (rejected by check) when invalid
	}
	if err := s.check(q); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = dsl.FeedDefaultLimit
	}
	limit = min(limit, dsl.FeedMaxLimit)
	e, stale, err := s.entry(ctx, q)
	if err != nil {
		return nil, err
	}
	items := e.items
	switch q.Provider {
	case dsl.FeedHolidays, dsl.FeedWeather:
		items = fromToday(items, s.now())
	case dsl.FeedPressure:
		items = notEnded(items, s.now(), pressureBlock)
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return &Result{Provider: q.Provider, Title: e.title, Items: append([]Item{}, items...), Source: s.ep.source(q),
		FetchedAt: e.fetchedAt, Stale: stale}, nil
}

// check rejects anything off the allowlist before any network use.
func (s *Service) check(q Query) error {
	if !dsl.FeedProviders[q.Provider] {
		return ErrUnknown
	}
	if s.disabled[q.Provider] {
		return ErrDisabled
	}
	switch q.Provider {
	case dsl.FeedWeather, dsl.FeedWarnings, dsl.FeedPressure:
		if _, ok := dsl.WeatherRegions[q.Region]; !ok || q.FeedID != "" {
			return ErrUnknown
		}
	case dsl.FeedRSS:
		if _, ok := s.ep.RSS[q.FeedID]; !ok || q.Region != "" {
			return ErrUnknown
		}
	case dsl.FeedArxiv:
		if _, ok := dsl.ArxivCategories[q.FeedID]; !ok || q.Region != "" {
			return ErrUnknown
		}
	case dsl.FeedStatus:
		if _, ok := s.ep.Status[q.FeedID]; !ok || q.Region != "" {
			return ErrUnknown
		}
	case dsl.FeedDiet:
		if q.Q == "" || q.Region != "" || q.FeedID != "" {
			return ErrUnknown
		}
	case dsl.FeedHolidays:
		if q.Region != "" || q.FeedID != "" {
			return ErrUnknown
		}
	}
	if q.Provider != dsl.FeedDiet && q.Q != "" {
		return ErrUnknown
	}
	return nil
}

// entry returns a fresh cached entry, fetching once per key at a time. On a
// failed fetch the last good copy (if recent enough) is served as stale.
// The fetch runs apart from ctx (only its own requestTimeout bounds it): a
// caller that gives up after waiting for its turn neither cancels the fetch
// nor leaves a failure in the cache. For a gap provider an expired entry with
// a last good copy is served right away; when the provider's turn is free the
// caller also starts the one refresh in the background (refresh).
func (s *Service) entry(ctx context.Context, q Query) (*entry, bool, error) {
	k := q.key()
	turn := s.turns[q.Provider] // nil = no gap
	held := false
	defer func() {
		if held {
			<-turn
		}
	}()
	var waited time.Duration
	refresh := false // serve the stale copy now, fetch in the background
	for {
		s.mu.Lock()
		now := s.now()
		e := s.cache[k]
		if e != nil && now.Before(e.expires) {
			s.mu.Unlock()
			if !e.failedAt.IsZero() {
				return s.staleOr(e, now, ErrRecentFailure)
			}
			return e, false, nil
		}
		stale := e != nil && e.items != nil && now.Sub(e.fetchedAt) < staleFor
		if c := s.inflight[k]; c != nil {
			s.mu.Unlock()
			if turn != nil && stale { // the refresh is already running
				return e, true, nil
			}
			return await(ctx, c)
		}
		busy := func(err error) (*entry, bool, error) {
			if stale {
				return e, true, nil
			}
			return nil, false, err
		}
		if s.userOver(now, q) {
			s.mu.Unlock()
			return busy(ErrUserLimit)
		}
		if turn != nil && !held {
			s.mu.Unlock()
			if stale { // a last good copy beats waiting in line
				if !tryTurn(turn) {
					return e, true, nil
				}
				held, refresh = true, true
				continue
			}
			if !takeTurn(ctx, turn, maxGapWait-waited) {
				return nil, false, ErrBusy
			}
			held = true
			continue // the caller before may have fetched the same key
		}
		if gap := providerLimits[q.Provider].gap; gap > 0 {
			if last, ok := s.lastAt[q.Provider]; ok && now.Sub(last) < gap {
				s.mu.Unlock()
				if refresh { // a later caller refreshes after the gap
					return e, true, nil
				}
				wait := gap - now.Sub(last)
				if waited+wait > maxGapWait || !s.sleep(ctx, wait) {
					return busy(ErrBusy)
				}
				waited += wait
				continue
			}
		}
		if !s.takeMiss(now, q) { // checked again and counted under the same lock
			s.mu.Unlock()
			return busy(ErrUserLimit)
		}
		if !s.takeBudget(now, q.Provider) {
			s.untakeMiss(q)
			s.mu.Unlock()
			return busy(ErrBusy)
		}
		s.lastAt[q.Provider] = now
		c := &call{done: make(chan struct{})}
		s.inflight[k] = c
		s.mu.Unlock()
		var done func()
		if held { // the turn passes to the fetch and ends with it
			held, done = false, func() { <-turn }
		}
		go s.run(context.WithoutCancel(ctx), k, q, e, c, done)
		if refresh {
			return e, true, nil
		}
		return await(ctx, c)
	}
}

// run fetches q and settles c. A fetch that ended by its context (cancel or
// deadline) is not remembered for FailTTL: the next caller tries again.
func (s *Service) run(ctx context.Context, k string, q Query, e *entry, c *call, done func()) {
	if done != nil {
		defer done()
	}
	start := time.Now()
	title, items, n, err := s.fetch(ctx, q)
	feedAttrs := []slog.Attr{slog.String("region", q.Region), slog.String("feed_id", q.FeedID)}
	if q.Q != "" { // the length only: the words stay out of the logs
		feedAttrs = append(feedAttrs, slog.Int("q_runes", len([]rune(q.Q))))
	}
	LogFetch(ctx, s.log, FetchLine{Provider: q.Provider, Err: err, Bytes: n, Duration: time.Since(start), Attrs: feedAttrs})

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	ne := &entry{}
	if e != nil {
		*ne = *e
	}
	keep := true
	switch {
	case err == nil:
		if len(items) > maxStoredItems {
			items = items[:maxStoredItems]
		}
		*ne = entry{items: items, title: title, fetchedAt: now, expires: now.Add(providerLimits[q.Provider].ttl)}
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		keep = false
	default:
		ne.failedAt, ne.expires = now, now.Add(FailTTL)
	}
	if keep {
		if s.cache[k] == nil && len(s.cache) >= maxCacheEntries {
			s.prune(now)
		}
		s.cache[k] = ne
	}
	if err != nil {
		c.e, c.stale, c.err = s.staleOr(ne, now, ErrUpstream)
	} else {
		c.e = ne
	}
	delete(s.inflight, k)
	close(c.done)
}

func (s *Service) staleOr(e *entry, now time.Time, fail error) (*entry, bool, error) {
	if e.items != nil && now.Sub(e.fetchedAt) < staleFor {
		return e, true, nil
	}
	return nil, false, fail
}

// await waits for c, or gives up when ctx ends (the fetch goes on).
func await(ctx context.Context, c *call) (*entry, bool, error) {
	select {
	case <-c.done:
		return c.e, c.stale, c.err
	case <-ctx.Done():
		return nil, false, ErrUpstream
	}
}

// takeTurn waits in line for a gap provider's single fetch slot, at most
// limit. Senders blocked on a channel are served in arrival order, so the
// line is first come first served.
func takeTurn(ctx context.Context, turn chan struct{}, limit time.Duration) bool {
	if limit <= 0 {
		return false
	}
	t := time.NewTimer(limit)
	defer t.Stop()
	select {
	case turn <- struct{}{}:
		return true
	case <-t.C:
		return false
	case <-ctx.Done():
		return false
	}
}

// tryTurn takes a gap provider's fetch slot only when it is free now.
func tryTurn(turn chan struct{}) bool {
	select {
	case turn <- struct{}{}:
		return true
	default:
		return false
	}
}

// takeBudget counts one upstream fetch against the provider's per-minute
// cap (diet has its own; every other provider shares one). Caller holds mu.
func (s *Service) takeBudget(now time.Time, provider string) bool {
	b := s.budgets[""]
	if own := s.budgets[provider]; own != nil {
		b = own
	}
	if now.Sub(b.start) >= time.Minute {
		b.start, b.used = now, 0
	}
	if b.used >= b.limit {
		return false
	}
	b.used++
	return true
}

// missKeys are q's per-user and per-address counters with their caps; none
// without search words.
type missKey struct {
	id    string
	limit int
}

func (s *Service) missKeys(q Query) []missKey {
	if q.Q == "" {
		return nil
	}
	var out []missKey
	if q.UID != "" {
		out = append(out, missKey{"u:" + q.UID, s.userMissesHour})
	}
	if q.IP != "" {
		out = append(out, missKey{"i:" + q.IP, s.ipMissesHour})
	}
	return out
}

// userOver reports whether q's caller (uid or address) used up this hour's
// search-word fetches. Only queries with search words count. Caller holds mu.
func (s *Service) userOver(now time.Time, q Query) bool {
	keys := s.missKeys(q)
	if len(keys) == 0 {
		return false
	}
	if s.misses.counts == nil || now.Sub(s.misses.start) >= time.Hour {
		s.misses = hourly{start: now, counts: map[string]int{}}
	}
	for _, kv := range keys {
		if s.misses.counts[kv.id] >= kv.limit {
			return true
		}
	}
	return false
}

// takeMiss checks and counts one source fetch for q's caller in one step
// (false = over a cap, nothing counted). Caller holds mu.
func (s *Service) takeMiss(now time.Time, q Query) bool {
	if s.userOver(now, q) {
		return false
	}
	for _, kv := range s.missKeys(q) {
		s.misses.counts[kv.id]++
	}
	return true
}

// untakeMiss gives back what takeMiss counted (the fetch did not start). Caller holds mu.
func (s *Service) untakeMiss(q Query) {
	for _, kv := range s.missKeys(q) {
		if k := kv.id; s.misses.counts[k] > 0 {
			s.misses.counts[k]--
		}
	}
}

// sleepCtx waits d unless ctx ends first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// prune drops entries past their stale window, then (still full) the oldest
// ones, so free search words cannot grow the cache without bound. Caller holds mu.
func (s *Service) prune(now time.Time) {
	for k, e := range s.cache {
		if s.inflight[k] == nil && now.After(e.expires) && now.Sub(e.fetchedAt) >= staleFor {
			delete(s.cache, k)
		}
	}
	for len(s.cache) >= maxCacheEntries {
		oldest, at := "", now
		for k, e := range s.cache {
			if s.inflight[k] == nil && !e.fetchedAt.After(at) {
				oldest, at = k, e.fetchedAt
			}
		}
		if oldest == "" {
			return
		}
		delete(s.cache, oldest)
	}
}

// fetch reads one source and reports the body size. The URL comes from
// Endpoints only.
func (s *Service) fetch(ctx context.Context, q Query) (string, []Item, int, error) {
	var rawURL, title string
	var parse func([]byte) ([]Item, error)
	switch q.Provider {
	case dsl.FeedWeather:
		rawURL, title, parse = s.ep.weatherURL(q.Region), dsl.WeatherRegions[q.Region]+"の天気", parseWeather
	case dsl.FeedHolidays:
		// The file goes back to 1955: keep today and later only.
		rawURL, title = s.ep.Holidays, "国民の祝日"
		parse = func(b []byte) ([]Item, error) {
			items, err := parseHolidays(b)
			return fromToday(items, s.now()), err
		}
	case dsl.FeedRSS:
		f := s.ep.RSS[q.FeedID]
		rawURL, title, parse = f.URL, f.Name, parseRSS
	case dsl.FeedWarnings:
		rawURL, title, parse = s.ep.warningURL(q.Region), dsl.WeatherRegions[q.Region]+"の警報・注意報", parseWarnings
	case dsl.FeedPressure:
		u, ok := s.ep.pressureURL(q.Region)
		if !ok {
			return "", nil, 0, ErrUnknown
		}
		rawURL, title = u, dsl.WeatherRegions[q.Region]+"の気圧"
		parse = func(b []byte) ([]Item, error) { return parsePressure(b, s.now()) }
	case dsl.FeedArxiv:
		rawURL, title, parse = s.ep.arxivURL(q.FeedID), "arXiv "+q.FeedID+"("+dsl.ArxivCategories[q.FeedID]+")の新着", parseArxiv
	case dsl.FeedDiet:
		rawURL, title, parse = s.ep.dietURL(q.Q), "国会会議録「"+q.Q+"」", parseDiet
	case dsl.FeedStatus:
		p := s.ep.Status[q.FeedID]
		rawURL, title = p.URL, p.Name+"の稼働状況"
		parse = func(b []byte) ([]Item, error) { return parseStatus(b, p.Name, p.SiteURL) }
	default:
		return "", nil, 0, ErrUnknown
	}
	body, err := s.get(ctx, rawURL, providerLimits[q.Provider].maxBody)
	if err != nil {
		return "", nil, len(body), err
	}
	items, err := parse(body)
	if err != nil {
		return "", nil, len(body), fmt.Errorf("%w: %w", ErrBadResponse, err)
	}
	return title, items, len(body), nil
}

// get performs one GET with a body cap. Anything but 200 is an error. The
// errors carry no URL (nothing logs them, but keep it that way).
func (s *Service) get(ctx context.Context, rawURL string, maxBody int64) ([]byte, error) {
	if !s.ep.allowedURL(rawURL) {
		return nil, ErrUnknown
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{Code: resp.StatusCode}
	}
	if resp.ContentLength > maxBody {
		return nil, fmt.Errorf("%w (%d bytes)", ErrTooLarge, resp.ContentLength)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return body, err
	}
	if int64(len(body)) > maxBody {
		return body, fmt.Errorf("%w (over %d bytes)", ErrTooLarge, maxBody)
	}
	return body, nil
}

// fromToday drops dated items before today (JST).
func fromToday(items []Item, now time.Time) []Item {
	today := now.In(jst).Format(time.DateOnly)
	for i, it := range items {
		if len(it.Date) >= 10 && it.Date[:10] >= today {
			return items[i:]
		}
	}
	return nil
}

// notEnded drops rows (dated RFC 3339 at their start) that ended before now.
func notEnded(items []Item, now time.Time, span time.Duration) []Item {
	for i, it := range items {
		if t, err := time.Parse(time.RFC3339, it.Date); err == nil && t.Add(span).After(now) {
			return items[i:]
		}
	}
	return nil
}

var jst = time.FixedZone("JST", 9*60*60)
