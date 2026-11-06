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
	Date     string   `json:"date,omitempty"` // YYYY-MM-DD (weather, holidays) or RFC 3339 (rss)
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle,omitempty"`
	Icon     string   `json:"icon,omitempty"` // SF Symbols name (weather)
	URL      string   `json:"url,omitempty"`  // https only (rss)
	Max      *float64 `json:"max,omitempty"`  // °C
	Min      *float64 `json:"min,omitempty"`  // °C
	Pop      *int     `json:"pop,omitempty"`  // precipitation probability %
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

// Query names what to read. Region is for weather, FeedID for rss.
type Query struct {
	Provider string
	Region   string
	FeedID   string
}

func (q Query) key() string { return q.Provider + "|" + q.Region + "|" + q.FeedID }

// Errors returned by Get.
var (
	ErrUnknown  = errors.New("feeds: not on the allowlist")
	ErrDisabled = errors.New("feeds: provider disabled")
	ErrBusy     = errors.New("feeds: outgoing request budget used up")
	ErrUpstream = errors.New("feeds: source unavailable")
)

// Per-provider limits.
type limits struct {
	ttl     time.Duration // fresh cache lifetime
	maxBody int64         // bytes read from the source
}

var providerLimits = map[string]limits{
	dsl.FeedWeather:  {ttl: 30 * time.Minute, maxBody: 256 << 10},
	dsl.FeedHolidays: {ttl: 12 * time.Hour, maxBody: 128 << 10},
	dsl.FeedRSS:      {ttl: 15 * time.Minute, maxBody: 512 << 10},
}

const (
	// failTTL keeps a failure for a while so a broken source is not retried
	// by every request.
	failTTL = time.Minute
	// staleFor is how long a last good copy may stand in for a failing source.
	staleFor = 24 * time.Hour
	// maxStoredItems caps what one cached entry keeps.
	maxStoredItems = 60
	// DefaultUpstreamPerMinute caps fetches to all sources per instance.
	DefaultUpstreamPerMinute = 30
)

// Service is safe for concurrent use.
type Service struct {
	client    *http.Client
	ep        Endpoints
	now       func() time.Time
	disabled  map[string]bool
	perMinute int

	mu       sync.Mutex
	cache    map[string]*entry
	inflight map[string]*call
	window   time.Time // start of the current upstream minute
	used     int       // fetches in window
}

type entry struct {
	items     []Item
	title     string
	fetchedAt time.Time
	expires   time.Time
	failedAt  time.Time // zero = last fetch succeeded
}

type call struct {
	done chan struct{}
}

// Options configures New. Zero values mean production defaults.
type Options struct {
	Client    *http.Client     // default: NewClient() (public addresses only)
	Endpoints *Endpoints       // default: DefaultEndpoints()
	Now       func() time.Time // default: time.Now
	Disabled  []string         // providers switched off (kill switch)
	PerMinute int              // upstream fetches per minute; 0 = DefaultUpstreamPerMinute
}

// New builds a Service.
func New(o Options) *Service {
	s := &Service{client: o.Client, now: o.Now, perMinute: o.PerMinute, disabled: map[string]bool{},
		cache: map[string]*entry{}, inflight: map[string]*call{}}
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
	if s.perMinute <= 0 {
		s.perMinute = DefaultUpstreamPerMinute
	}
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
	if q.Provider == dsl.FeedHolidays || q.Provider == dsl.FeedWeather {
		items = fromToday(items, s.now())
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
	case dsl.FeedWeather:
		if _, ok := dsl.WeatherRegions[q.Region]; !ok || q.FeedID != "" {
			return ErrUnknown
		}
	case dsl.FeedRSS:
		if _, ok := s.ep.RSS[q.FeedID]; !ok || q.Region != "" {
			return ErrUnknown
		}
	case dsl.FeedHolidays:
		if q.Region != "" || q.FeedID != "" {
			return ErrUnknown
		}
	}
	return nil
}

// entry returns a fresh cached entry, fetching once per key at a time. On a
// failed fetch the last good copy (if recent enough) is served as stale.
func (s *Service) entry(ctx context.Context, q Query) (*entry, bool, error) {
	k := q.key()
	for {
		s.mu.Lock()
		now := s.now()
		e := s.cache[k]
		if e != nil && now.Before(e.expires) {
			s.mu.Unlock()
			if !e.failedAt.IsZero() {
				return s.staleOr(e, now)
			}
			return e, false, nil
		}
		if c := s.inflight[k]; c != nil {
			s.mu.Unlock()
			select {
			case <-c.done:
				continue // read the cache again
			case <-ctx.Done():
				return nil, false, ErrUpstream
			}
		}
		if !s.takeBudget(now) {
			s.mu.Unlock()
			if e != nil && e.items != nil && now.Sub(e.fetchedAt) < staleFor {
				return e, true, nil
			}
			return nil, false, ErrBusy
		}
		c := &call{done: make(chan struct{})}
		s.inflight[k] = c
		s.mu.Unlock()

		title, items, err := s.fetch(ctx, q)

		s.mu.Lock()
		now = s.now()
		ne := &entry{}
		if e != nil {
			*ne = *e
		}
		if err != nil {
			ne.failedAt, ne.expires = now, now.Add(failTTL)
			slog.Warn("feeds: fetch failed", slog.String("provider", q.Provider), slog.String("region", q.Region),
				slog.String("feedId", q.FeedID), slog.Any("error", err))
		} else {
			if len(items) > maxStoredItems {
				items = items[:maxStoredItems]
			}
			*ne = entry{items: items, title: title, fetchedAt: now, expires: now.Add(providerLimits[q.Provider].ttl)}
		}
		s.cache[k] = ne
		delete(s.inflight, k)
		close(c.done)
		s.mu.Unlock()
		if err != nil {
			return s.staleOr(ne, now)
		}
		return ne, false, nil
	}
}

func (s *Service) staleOr(e *entry, now time.Time) (*entry, bool, error) {
	if e.items != nil && now.Sub(e.fetchedAt) < staleFor {
		return e, true, nil
	}
	return nil, false, ErrUpstream
}

// takeBudget counts one upstream fetch against the per-minute cap. Caller holds mu.
func (s *Service) takeBudget(now time.Time) bool {
	if now.Sub(s.window) >= time.Minute {
		s.window, s.used = now, 0
	}
	if s.used >= s.perMinute {
		return false
	}
	s.used++
	return true
}

// fetch reads one source. The URL comes from Endpoints only.
func (s *Service) fetch(ctx context.Context, q Query) (string, []Item, error) {
	lim := providerLimits[q.Provider]
	switch q.Provider {
	case dsl.FeedWeather:
		body, err := s.get(ctx, s.ep.weatherURL(q.Region), lim.maxBody)
		if err != nil {
			return "", nil, err
		}
		items, err := parseWeather(body)
		return dsl.WeatherRegions[q.Region] + "の天気", items, err
	case dsl.FeedHolidays:
		body, err := s.get(ctx, s.ep.Holidays, lim.maxBody)
		if err != nil {
			return "", nil, err
		}
		items, err := parseHolidays(body)
		// The file goes back to 1955: keep today and later only.
		return "国民の祝日", fromToday(items, s.now()), err
	case dsl.FeedRSS:
		f := s.ep.RSS[q.FeedID]
		body, err := s.get(ctx, f.URL, lim.maxBody)
		if err != nil {
			return "", nil, err
		}
		items, err := parseRSS(body)
		return f.Name, items, err
	}
	return "", nil, ErrUnknown
}

// get performs one GET with a body cap. Anything but 200 is an error.
func (s *Service) get(ctx context.Context, rawURL string, maxBody int64) ([]byte, error) {
	if !s.ep.allowedURL(rawURL) {
		return nil, fmt.Errorf("%w: %s", ErrUnknown, rawURL)
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
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBody {
		return nil, fmt.Errorf("body too large (%d bytes)", resp.ContentLength)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBody {
		return nil, fmt.Errorf("body over %d bytes", maxBody)
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

var jst = time.FixedZone("JST", 9*60*60)
