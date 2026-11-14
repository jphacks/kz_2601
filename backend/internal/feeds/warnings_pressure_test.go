package feeds

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Fixtures are real responses recorded on 2026-11-08 (JST):
//   - jma_warning_r8_016000.json: https://www.jma.go.jp/bosai/warning/data/r8/016000.json
//   - met_compact_170000.json: https://api.met.no/weatherapi/locationforecast/2.0/compact?lat=36.59&lon=136.63
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestWarnings(t *testing.T) {
	f := newFake(t)
	s := newSvc(f, &clock{t: testNow}, Options{})
	res, err := s.Get(context.Background(), Query{Provider: dsl.FeedWarnings, Region: "016000"}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "石狩・空知・後志地方の警報・注意報" || res.Source.Name != "気象庁ホームページ" || res.Source.License != pdl {
		t.Errorf("title/source = %q %+v", res.Title, res.Source)
	}
	var got []string
	for _, it := range res.Items {
		got = append(got, it.Title+" | "+it.Subtitle+" | "+it.Icon)
	}
	// 解除 and 「発表警報・注意報はなし」 are left out; warnings come before advisories.
	want := []string{
		"暴風警報 | 継続 · 石狩地方 | exclamationmark.triangle.fill",
		"波浪警報 | 継続 · 石狩地方・後志地方 | exclamationmark.triangle.fill",
		"強風注意報 | 継続 · 空知地方・後志地方 | exclamationmark.circle",
		"着雪注意報 | 継続 · 石狩地方・空知地方・後志地方 | exclamationmark.circle",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("rows =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if d := res.Items[0].Date; d != "2026-11-07T15:31:00+09:00" {
		t.Errorf("date = %s", d)
	}
}

func TestParseWarnings(t *testing.T) {
	none := `[{"reportDatetime":"2026-11-05T13:45:00+09:00","warning":{"class10Items":[
	  {"areaCode":"130010","kinds":[{"status":"発表警報・注意報はなし"}]},
	  {"areaCode":"130030","kinds":[{"code":"10","status":"解除"},{"code":"99","status":"発表"}]}]}}]`
	items, err := parseWarnings([]byte(none))
	if err != nil || items == nil || len(items) != 0 {
		t.Errorf("no warning in force = %v %v, want an empty list", items, err)
	}
	special := `[{"reportDatetime":"2026-11-05T13:45:00+09:00","warning":{"class10Items":[
	  {"areaCode":"130010","kinds":[{"code":"10","status":"発表"},{"code":"33","status":"発表"}]},
	  {"areaCode":"999999","kinds":[{"code":"43","status":"警報から注意報"}]}]}}]`
	items, err = parseWarnings([]byte(special))
	if err != nil || len(items) != 3 {
		t.Fatalf("items = %+v %v", items, err)
	}
	if items[0].Title != "大雨特別警報" || items[1].Title != "大雨危険警報" || items[1].Subtitle != "警報から注意報" || items[2].Title != "大雨注意報" {
		t.Errorf("order / unknown area = %+v", items)
	}
	for _, bad := range []string{`{}`, `[]`, `<html>`} {
		if _, err := parseWarnings([]byte(bad)); err == nil {
			t.Errorf("%s should fail", bad)
		}
	}
}

// Reports out of time order: per (area, kind) the newest report decides, so
// a warning lifted by a later report is not shown, and an
// older 発表 does not stand beside a newer 継続.
func TestParseWarningsNewestReportWins(t *testing.T) {
	items, err := parseWarnings(fixture(t, "jma_warning_r8_order.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range items {
		got = append(got, it.Title+"|"+it.Subtitle+"|"+it.Date)
	}
	want := []string{
		"大雨警報|継続 · 後志地方|2026-11-05T13:00:00+09:00",
		"大雨注意報|発表 · 石狩地方|2026-11-05T16:00:00+09:00",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("items:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestPressure(t *testing.T) {
	// The fixture starts at 2026-11-07T18:00Z (11-08 03:00 JST).
	now := time.Date(2026, 11, 8, 3, 10, 0, 0, jst)
	c := &clock{t: now}
	f := newFake(t)
	s := newSvc(f, c, Options{})
	res, err := s.Get(context.Background(), Query{Provider: dsl.FeedPressure, Region: "170000"}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "石川県の気圧" || !strings.Contains(res.Source.Name, "MET Norway") || !strings.Contains(res.Source.License, "CC BY 4.0") {
		t.Errorf("title/source = %q %+v", res.Title, res.Source)
	}
	if len(res.Items) != 9 {
		t.Fatalf("48 hours from 03:00 = 9 six-hour rows, got %d: %+v", len(res.Items), res.Items)
	}
	first := res.Items[0]
	if first.Date != "2026-11-08T00:00:00+09:00" || first.Title != "0時〜6時" || len(first.Series) != 3 ||
		*first.Min != 1033.2 || *first.Max != 1033.9 || first.Drop != nil || first.Subtitle != "1033 → 1034 hPa" {
		t.Errorf("first row = %+v", first)
	}
	// 1035.7 (11-08 08時) → 1027.9 (11-09 03時): an 8 hPa fall within 24 hours.
	var drops []string
	for _, it := range res.Items {
		if it.Drop != nil && *it.Drop {
			drops = append(drops, it.Date[5:13]+" "+it.Subtitle)
		}
		if len(it.Series) == 0 || len(it.Series) > 6 || *it.Min > *it.Max {
			t.Errorf("row %+v", it)
		}
	}
	if len(drops) == 0 || drops[0] != "11-09T00 24 時間で 8 hPa 下がります" {
		t.Errorf("drops = %v", drops)
	}
	// Rows that have ended are dropped on read (cache hit within the TTL).
	c.add(20 * time.Minute)
	res, err = s.Get(context.Background(), Query{Provider: dsl.FeedPressure, Region: "170000"}, 20)
	if err != nil || len(res.Items) != 9 {
		t.Errorf("cached = %+v %v", res, err)
	}
	if n := f.count("/met/compact"); n != 1 {
		t.Errorf("source hit %d times, want 1 (cached)", n)
	}
	c.add(7 * time.Hour)
	res, err = s.Get(context.Background(), Query{Provider: dsl.FeedPressure, Region: "170000"}, 20)
	if err != nil || res.Items[0].Date != "2026-11-08T06:00:00+09:00" {
		t.Errorf("after 7h first = %+v %v", res.Items, err)
	}
}

func TestParsePressure(t *testing.T) {
	now := time.Date(2026, 11, 8, 3, 0, 0, 0, jst)
	for _, bad := range []string{`{}`, `{"properties":{"timeseries":[]}}`, `[`} {
		if _, err := parsePressure([]byte(bad), now); err == nil {
			t.Errorf("%s should fail", bad)
		}
	}
	// Out-of-range values and the 6-hourly tail are skipped.
	body := `{"properties":{"timeseries":[
	 {"time":"2026-11-07T18:00:00Z","data":{"instant":{"details":{"air_pressure_at_sea_level":1010}}}},
	 {"time":"2026-11-07T19:00:00Z","data":{"instant":{"details":{"air_pressure_at_sea_level":5}}}},
	 {"time":"2026-11-07T20:00:00Z","data":{"instant":{"details":{"air_pressure_at_sea_level":1011}}}},
	 {"time":"2026-11-08T02:00:00Z","data":{"instant":{"details":{"air_pressure_at_sea_level":1000}}}}]}}`
	items, err := parsePressure([]byte(body), now)
	if err == nil {
		t.Errorf("a gap leaves fewer than 2 hourly values: %+v", items)
	}
}

func TestWarningsPressureAllowlist(t *testing.T) {
	f := newFake(t)
	s := newSvc(f, &clock{t: testNow}, Options{})
	for _, q := range []Query{
		{Provider: dsl.FeedWarnings},
		{Provider: dsl.FeedWarnings, Region: "13000"},
		{Provider: dsl.FeedWarnings, Region: "130000/../x"},
		{Provider: dsl.FeedPressure, Region: "999999"},
		{Provider: dsl.FeedPressure, Region: "130000", FeedID: "digital-news"},
	} {
		if _, err := s.Get(context.Background(), q, 5); !errors.Is(err, ErrUnknown) {
			t.Errorf("%+v: err = %v, want ErrUnknown", q, err)
		}
	}
	if n := f.count("/met/compact"); n != 0 {
		t.Errorf("rejected queries reached the source %d times", n)
	}
}

func TestWarningsPressureEndpoints(t *testing.T) {
	ep := DefaultEndpoints()
	if got := ep.warningURL("170000"); got != "https://www.jma.go.jp/bosai/warning/data/r8/170000.json" {
		t.Errorf("warning url = %s", got)
	}
	got, ok := ep.pressureURL("170000")
	if !ok || got != "https://api.met.no/weatherapi/locationforecast/2.0/compact?lat=36.59&lon=136.63" || !ep.allowedURL(got) {
		t.Errorf("pressure url = %s %v", got, ok)
	}
	if _, ok := ep.pressureURL("999999"); ok {
		t.Error("pressure url for an unknown region")
	}
}

// Every forecast office has a pressure point (inside Japan) and no extra.
func TestPressurePointsCoverRegions(t *testing.T) {
	for code := range dsl.WeatherRegions {
		p, ok := pressurePoints[code]
		if !ok {
			t.Errorf("no pressure point for %s", code)
			continue
		}
		if p[0] < 24 || p[0] > 46 || p[1] < 122 || p[1] > 146 {
			t.Errorf("%s point %v is outside Japan", code, p)
		}
	}
	if len(pressurePoints) != len(dsl.WeatherRegions) {
		t.Errorf("%d points for %d regions", len(pressurePoints), len(dsl.WeatherRegions))
	}
}
