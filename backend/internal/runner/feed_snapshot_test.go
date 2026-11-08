package runner

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/feeds"
	"github.com/jphacks/kz_2601/backend/internal/runner/engine"
)

func TestFeedSnapshot(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	p := feedSnapshot("pressure", []feeds.Item{{Min: f(1008.4), Max: f(1012.2)}, {Min: f(1009), Max: f(1015.1)}})
	if p.Summary != "1008〜1016 hPa" || *p.Min != 1008.4 || *p.Max != 1015.1 || p.Drop == nil || *p.Drop {
		t.Fatalf("pressure: %+v", p)
	}
	if e := feedSnapshot("pressure", nil); e.Summary != "" || e.Min != nil || e.Drop != nil {
		t.Fatalf("empty pressure saves nothing: %+v", e)
	}
	w := feedSnapshot("warnings", []feeds.Item{{Title: "大雨警報"}, {Title: "雷注意報"}, {Title: "大雨警報"}})
	if w.Summary != "大雨警報・雷注意報" || w.Min != nil {
		t.Fatalf("warnings: %+v", w)
	}
	if n := feedSnapshot("warnings", nil); n.Summary != "発表なし" {
		t.Fatalf("no warnings: %+v", n)
	}
	long := feedSnapshot("weather", []feeds.Item{{Title: strings.Repeat("晴", 300)}})
	if got := len([]rune(long.Summary)); got != maxFeedSummaryRunes {
		t.Fatalf("summary not cut: %d", got)
	}
	if e := feedSnapshot("weather", nil); e.Summary != "" {
		t.Fatalf("no weather: %+v", e)
	}
}

type errFeeds struct {
	err error
	res *feeds.Result
}

func (f errFeeds) Get(context.Context, feeds.Query, int) (*feeds.Result, error) { return f.res, f.err }

// Busy, recently failed (cached) and stale feeds wait (deferred, not a
// counted failure); a fresh source failure is an ordinary error; a switched
// off provider is skipped.
func TestFeedErrors(t *testing.T) {
	for _, c := range []struct {
		name       string
		f          errFeeds
		wait, skip bool
	}{
		{"busy", errFeeds{err: feeds.ErrBusy}, true, false},
		{"recent failure", errFeeds{err: feeds.ErrRecentFailure}, true, false},
		{"stale", errFeeds{res: &feeds.Result{Stale: true}}, true, false},
		{"upstream", errFeeds{err: feeds.ErrUpstream}, false, false},
		{"disabled", errFeeds{err: feeds.ErrDisabled}, false, true},
	} {
		_, err := (&ruleReader{feeds: c.f}).Feed(t.Context(), "weather", "130000")
		if err == nil || errors.Is(err, errFeedWait) != c.wait || errors.Is(err, engine.ErrFeedSkip) != c.skip {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}
