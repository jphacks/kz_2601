package runner

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/feeds"
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
