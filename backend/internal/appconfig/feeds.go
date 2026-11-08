package appconfig

// External feeds (#348): GET /v1/feeds/{provider} reads allowlisted public
// data (weather / warnings / pressure / holidays / allowlisted rss / arxiv
// category / diet speech search) for the DSL feed leaf. The
// server is the only party that talks to the source (the device never does),
// takes nothing from the request but the provider and its typed parameter,
// and sends nothing about the user. internal/feeds holds the allowlist, the
// address check, the timeout, the body cap and the cache.
// docs/backend/features/external-feeds.md

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/feeds"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// DefaultFeedsPerHour is the per-user request cap (FEEDS_USER_PER_HOUR).
const DefaultFeedsPerHour = 120

func init() {
	registerRoutes(func(h *Handler, mux *http.ServeMux) {
		if h.feeds == nil {
			h.feeds = feeds.New(feeds.OptionsFromEnv(os.Getenv))
		}
		if h.feedUsers == nil {
			h.feedUsers = newFeedLimiter(feedsPerHour(os.Getenv), time.Now)
		}
		mux.HandleFunc("GET /v1/feeds/{provider}", h.getFeed)
	})
}

// WithFeeds replaces the feed service and the per-user cap (tests).
func WithFeeds(s *feeds.Service, perHour int, now func() time.Time) Option {
	return func(h *Handler) {
		h.feeds = s
		h.feedUsers = newFeedLimiter(perHour, now)
	}
}

func feedsPerHour(getenv func(string) string) int {
	if n, err := strconv.Atoi(getenv("FEEDS_USER_PER_HOUR")); err == nil && n >= 0 {
		return n
	}
	return DefaultFeedsPerHour
}

var feedParams = map[string]bool{"region": true, "feedId": true, "q": true, "limit": true}

// getFeed: GET /v1/feeds/{provider}?region=&feedId=&q=&limit=
func (h *Handler) getFeed(w http.ResponseWriter, r *http.Request) {
	q := feeds.Query{Provider: r.PathValue("provider")}
	if !dsl.FeedProviders[q.Provider] {
		fail(w, apiErr(http.StatusNotFound, httpx.CodeNotFound, "unknown feed provider"))
		return
	}
	vals := r.URL.Query()
	for k, v := range vals {
		if !feedParams[k] || len(v) != 1 {
			fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "unknown or repeated query parameter").With("field", k))
			return
		}
	}
	q.Region, q.FeedID, q.Q = vals.Get("region"), vals.Get("feedId"), vals.Get("q")
	if _, ok := dsl.NormalizeFeedQuery(q.Q); vals.Has("q") && (q.Provider != dsl.FeedDiet || !ok) {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest,
			fmt.Sprintf("q is for diet only: 1..%d characters, no control characters", dsl.FeedQueryMaxRunes)).With("field", "q"))
		return
	}
	limit := 0
	if raw := vals.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > dsl.FeedMaxLimit {
			fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, fmt.Sprintf("limit must be 1..%d", dsl.FeedMaxLimit)).With("field", "limit"))
			return
		}
		limit = n
	}
	if wait, ok := h.feedUsers.take(uidOf(r)); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(wait))
		httpx.ErrorWithDetails(w, http.StatusTooManyRequests, httpx.CodeRateLimited, "しばらくしてからもう一度読み込んでください。",
			map[string]any{"scope": "user", "limit": h.feedUsers.perHour, "remaining": 0, "retryAfterSec": wait})
		return
	}
	// bounds the wait only: the source fetch itself runs on (internal/feeds)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	q.UID, q.IP = uidOf(r), ratelimit.IPKey(ratelimit.ClientIP(r))
	res, err := h.feeds.Get(ctx, q, limit)
	switch {
	case errors.Is(err, feeds.ErrUserLimit):
		w.Header().Set("Retry-After", "600")
		httpx.ErrorWithDetails(w, http.StatusTooManyRequests, httpx.CodeRateLimited, "しばらくしてからもう一度読み込んでください。",
			map[string]any{"scope": "user", "remaining": 0, "retryAfterSec": 600})
	case errors.Is(err, feeds.ErrUnknown):
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "region / feedId / q is not on the allowlist"))
	case errors.Is(err, feeds.ErrDisabled):
		fail(w, apiErr(http.StatusServiceUnavailable, httpx.CodeUpstreamUnavailable, "この読み込みは今止めています").With("reason", "disabled"))
	case errors.Is(err, feeds.ErrBusy), err != nil:
		w.Header().Set("Retry-After", "60")
		fail(w, apiErr(http.StatusServiceUnavailable, httpx.CodeUpstreamUnavailable, "データの提供元に今つながりません"))
	default:
		w.Header().Set("Cache-Control", "private, max-age=300")
		httpx.WriteJSON(w, http.StatusOK, res)
	}
}

// feedLimiter is a per-uid fixed hourly window on this instance (the feeds
// are cached and cheap; the cap only stops a runaway client).
type feedLimiter struct {
	perHour int // 0 = off
	now     func() time.Time

	mu     sync.Mutex
	window time.Time
	counts map[string]int
}

func newFeedLimiter(perHour int, now func() time.Time) *feedLimiter {
	return &feedLimiter{perHour: perHour, now: now, counts: map[string]int{}}
}

// take counts one request; false + seconds to wait when over the cap.
func (l *feedLimiter) take(uid string) (int, bool) {
	if l.perHour <= 0 {
		return 0, true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Sub(l.window) >= time.Hour {
		l.window, l.counts = now, map[string]int{}
	}
	if l.counts[uid] >= l.perHour {
		return max(1, int(l.window.Add(time.Hour).Sub(now).Seconds()+0.999)), false
	}
	l.counts[uid]++
	return 0, true
}
