package runner

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/internal/feeds"
	"github.com/jphacks/kz_2601/backend/internal/runner/engine"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// FeedGetter is what saveFeed reads public feeds through: *feeds.Service
// (the feed leaf's allowlist, outgoing-address checks, body caps, cache,
// per-minute budget and FEEDS_DISABLED kill switch).
type FeedGetter interface {
	Get(ctx context.Context, q feeds.Query, limit int) (*feeds.Result, error)
}

// errFeedWait: the feed cannot answer right now for a reason the next
// attempt 30 s later would hit again: the per-minute fetch budget is used up
// (a 07:00 burst), the source failed less than feeds.FailTTL ago (cached), or
// only a stale copy is left. The run is deferred without counting as a
// failure (Runner.deferRun), past feeds.FailTTL with jitter.
var errFeedWait = errors.New("runner: feed not ready")

// errFeedStale: the source failed and only an older copy is cached; the run
// waits rather than saving an old value as today's.
var errFeedStale = fmt.Errorf("%w: source unavailable (stale copy)", errFeedWait)

// maxFeedSummaryRunes caps the saved summary.
const maxFeedSummaryRunes = 200

// pressureRows: 4 rows of 6 hours = the next 24 hours.
const pressureRows = 4

// Feed implements engine.FeedReader. Only the provider and the DSL's fixed
// region reach the source.
func (rr *ruleReader) Feed(ctx context.Context, provider, region string) (engine.FeedSnapshot, error) {
	if rr.feeds == nil || !slices.Contains(dsl.SaveFeedProviders, provider) {
		return engine.FeedSnapshot{}, engine.ErrFeedSkip
	}
	limit := 1
	switch provider {
	case dsl.FeedPressure:
		limit = pressureRows
	case dsl.FeedWarnings:
		limit = dsl.FeedMaxLimit
	}
	res, err := rr.feeds.Get(ctx, feeds.Query{Provider: provider, Region: region}, limit)
	switch {
	case errors.Is(err, feeds.ErrDisabled), errors.Is(err, feeds.ErrUnknown):
		return engine.FeedSnapshot{}, engine.ErrFeedSkip
	case errors.Is(err, feeds.ErrBusy), errors.Is(err, feeds.ErrRecentFailure):
		return engine.FeedSnapshot{}, fmt.Errorf("runner: feed %s: %w: %w", provider, errFeedWait, err)
	case err != nil:
		return engine.FeedSnapshot{}, fmt.Errorf("runner: feed %s: %w", provider, err)
	case res.Stale:
		return engine.FeedSnapshot{}, errFeedStale
	}
	return feedSnapshot(provider, res.Items), nil
}

// feedSnapshot reduces a feed's rows to one record's values.
func feedSnapshot(provider string, items []feeds.Item) engine.FeedSnapshot {
	var s engine.FeedSnapshot
	switch provider {
	case dsl.FeedWeather:
		if len(items) == 0 {
			return s
		}
		it := items[0] // today (the service drops past days)
		s.Summary, s.Min, s.Max = it.Title, it.Min, it.Max
	case dsl.FeedPressure:
		drop := false
		for _, it := range items {
			if it.Min != nil && (s.Min == nil || *it.Min < *s.Min) {
				s.Min = ptrOf(*it.Min)
			}
			if it.Max != nil && (s.Max == nil || *it.Max > *s.Max) {
				s.Max = ptrOf(*it.Max)
			}
			if it.Drop != nil && *it.Drop && !drop {
				drop, s.Summary = true, it.Subtitle
			}
		}
		if s.Min == nil || s.Max == nil {
			return engine.FeedSnapshot{}
		}
		s.Drop = &drop
		if s.Summary == "" {
			s.Summary = fmt.Sprintf("%.0f〜%.0f hPa", math.Floor(*s.Min), math.Ceil(*s.Max))
		}
	case dsl.FeedWarnings:
		var names []string
		for _, it := range items {
			if it.Title != "" && !slices.Contains(names, it.Title) {
				names = append(names, it.Title)
			}
		}
		s.Summary = "発表なし"
		if len(names) > 0 {
			s.Summary = strings.Join(names, "・")
		}
	}
	s.Summary = cutRunes(strings.TrimSpace(s.Summary), maxFeedSummaryRunes)
	return s
}

func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func ptrOf[T any](v T) *T { return &v }
