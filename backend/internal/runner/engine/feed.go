package engine

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// FeedSnapshot is what a saveFeed action stores (nil / "" = not available).
type FeedSnapshot struct {
	Summary string
	Min     *float64
	Max     *float64
	Drop    *bool
}

// FeedReader reads one public feed snapshot (the runner's reader implements
// it through internal/feeds). Only the provider and the DSL's fixed region
// are passed: nothing of the user's records.
type FeedReader interface {
	Feed(ctx context.Context, provider, region string) (FeedSnapshot, error)
}

// ErrFeedSkip: the feed cannot be read for this run (switched off, not
// configured, nothing to save). The action is skipped; other errors retry
// the run.
var ErrFeedSkip = errors.New("engine: feed skipped")

// SkipFeedRequiredEmpty: a required field of the target would be empty
// (e.g. the weather came back without its text), so that day is not saved.
const SkipFeedRequiredEmpty = "feed_required_empty"

// feedZone is the zone of a saveFeed record's day. The providers are Japan
// Meteorological Agency forecasts for a Japanese region, so "today" is the
// day in Japan whatever the creator's time zone (JST has no daylight
// saving: a fixed zone needs no tzdata).
var feedZone = time.FixedZone("JST", 9*60*60)

// FeedDay is the day (YYYY-MM-DD, Japan time) a saveFeed run at `at` saves.
func FeedDay(at time.Time) string { return at.In(feedZone).Format("2006-01-02") }

// saveFeed plans the record of one saveFeed action: at most one per rule,
// action and day (Japan time; PlannedWrite.Key), so another run of the same
// day (a rescheduled or caught-up event) does not add a second record.
func (p *planner) saveFeed(i int, s *dsl.RuleSaveFeed, out *Plan) error {
	if p.ev.Kind != KindScheduled {
		return nil
	}
	target := p.app.EntityByID(s.Entity)
	if target == nil {
		return fmt.Errorf("engine: saveFeed entity %q is not in the app", s.Entity)
	}
	fr, ok := p.src.refs.(FeedReader)
	if !ok {
		return nil
	}
	snap, err := fr.Feed(p.src.ctx, s.Provider, s.Region)
	if errors.Is(err, ErrFeedSkip) {
		return nil
	}
	if err != nil {
		return err
	}
	vals := map[string]any{}
	set := func(id string, v any) {
		if id == "" || v == nil {
			return
		}
		f := target.FieldByID(id)
		if f == nil || f.Computed != nil {
			return
		}
		if n, ok := v.(float64); ok && f.Type == dsl.FieldInteger {
			v = math.Round(n)
		}
		vals[id] = v
	}
	day := FeedDay(p.ev.At)
	if id := s.Fields.Date; id != "" {
		if f := target.FieldByID(id); f != nil && f.Type == dsl.FieldDatetime {
			set(id, p.ev.At.UTC().Format("2006-01-02T15:04:05Z"))
		} else {
			set(id, day)
		}
	}
	if snap.Summary != "" {
		set(s.Fields.Summary, snap.Summary)
	}
	if snap.Min != nil {
		set(s.Fields.Min, *snap.Min)
	}
	if snap.Max != nil {
		set(s.Fields.Max, *snap.Max)
	}
	if snap.Drop != nil {
		set(s.Fields.Drop, *snap.Drop)
	}
	if snap.Summary == "" && snap.Min == nil && snap.Max == nil && snap.Drop == nil {
		return nil // nothing came back: no empty record
	}
	for _, f := range target.Fields {
		if _, ok := vals[f.ID]; f.Required && f.Computed == nil && !ok {
			out.Skipped = append(out.Skipped, PlannedSkip{Action: i, Reason: SkipFeedRequiredEmpty})
			return nil
		}
	}
	out.Writes = append(out.Writes, PlannedWrite{Action: i, Kind: WriteCreate, Entity: target.ID, Values: vals, Key: day})
	return nil
}
