package feeds

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Fixtures are real responses recorded on 2026-11-12 (JST), unchanged (they
// are already small; the only cyclone had just become a 温帯低気圧):
//   - jma_typhoon_targetTc.json: https://www.jma.go.jp/bosai/typhoon/data/targetTc.json
//   - jma_typhoon_TC2638_specifications.json: https://www.jma.go.jp/bosai/typhoon/data/TC2638/specifications.json

const (
	typhoonList = "/typhoon/targetTc.json"
	typhoonSpec = "/typhoon/TC2638/specifications.json"
)

func typhoonFake(t *testing.T) *fake {
	f := newFake(t)
	f.body[typhoonList] = fixture(t, "jma_typhoon_targetTc.json")
	f.body[typhoonSpec] = fixture(t, "jma_typhoon_TC2638_specifications.json")
	return f
}

func TestTyphoon(t *testing.T) {
	f := typhoonFake(t)
	s := newSvc(f, &clock{t: testNow}, Options{})
	res, err := s.Get(context.Background(), Query{Provider: dsl.FeedTyphoon}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "台風の情報" || res.Source.Name != "気象庁ホームページ" || res.Source.License != "公共データ利用規約(第1.0版)・気象庁「台風情報」(https://www.jma.go.jp/bosai/typhoon/)をもとに Tailor It 作成(編集責任は Tailor It)" ||
		res.Source.URL != "https://www.jma.go.jp/bosai/typhoon/" {
		t.Errorf("title/source = %q %+v", res.Title, res.Source)
	}
	want := []Item{{Date: "2026-11-11T21:00:00+09:00", Title: "台風第32号(チャンパカ)", Icon: "cloud",
		Subtitle: "温帯低気圧 · 南鳥島近海(北緯30.0度・東経153.0度) · 中心気圧 990 hPa · 東北東 55 km/h"}}
	if !reflect.DeepEqual(res.Items, want) {
		t.Fatalf("items = %+v\nwant %+v", res.Items, want)
	}
	if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedTyphoon}, 0); err != nil {
		t.Fatal(err)
	}
	if f.count(typhoonList) != 1 || f.count(typhoonSpec) != 1 {
		t.Errorf("hits = %d %d, want 1 1 (cached)", f.count(typhoonList), f.count(typhoonSpec))
	}
}

// No cyclone: an empty list (the leaf shows emptyText), not an error.
func TestTyphoonNone(t *testing.T) {
	f := typhoonFake(t)
	f.body[typhoonList] = []byte("[]")
	s := newSvc(f, &clock{t: testNow}, Options{})
	res, err := s.Get(context.Background(), Query{Provider: dsl.FeedTyphoon}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Items == nil || len(res.Items) != 0 || f.count(typhoonSpec) != 0 {
		t.Errorf("items = %#v, spec hits %d", res.Items, f.count(typhoonSpec))
	}
}

// A cyclone's file failing fails the whole fetch (no partial list).
func TestTyphoonSpecFails(t *testing.T) {
	for _, code := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		f := typhoonFake(t)
		f.code[typhoonSpec] = code
		s := newSvc(f, &clock{t: testNow}, Options{})
		if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedTyphoon}, 0); !errors.Is(err, ErrUpstream) {
			t.Errorf("%d: err = %v", code, err)
		}
	}
	f := typhoonFake(t)
	f.body[typhoonSpec] = []byte(`{"not":"a list"}`)
	s := newSvc(f, &clock{t: testNow}, Options{})
	if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedTyphoon}, 0); !errors.Is(err, ErrUpstream) {
		t.Errorf("bad body: err = %v", err)
	}
}

func TestTyphoonAllowlist(t *testing.T) {
	f := typhoonFake(t)
	s := newSvc(f, &clock{t: testNow}, Options{Disabled: []string{dsl.FeedTyphoon}})
	if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedTyphoon}, 0); !errors.Is(err, ErrDisabled) {
		t.Errorf("disabled: err = %v", err)
	}
	s = newSvc(f, &clock{t: testNow}, Options{})
	for _, q := range []Query{
		{Provider: dsl.FeedTyphoon, Region: "130000"},
		{Provider: dsl.FeedTyphoon, FeedID: "TC2638"},
		{Provider: dsl.FeedTyphoon, Q: "台風"},
	} {
		if _, err := s.Get(context.Background(), q, 0); !errors.Is(err, ErrUnknown) {
			t.Errorf("%+v: err = %v", q, err)
		}
	}
	if f.count(typhoonList) != 0 {
		t.Error("a rejected query reached the source")
	}
	o := OptionsFromEnv(func(k string) string { return map[string]string{"FEEDS_DISABLED": "typhoon"}[k] })
	if !slices.Contains(o.Disabled, dsl.FeedTyphoon) {
		t.Errorf("FEEDS_DISABLED=typhoon: %v", o.Disabled)
	}
}

// Only TC + 4 digits becomes a path segment; duplicates go, at most maxTyphoons.
func TestParseTyphoonTargets(t *testing.T) {
	got, err := parseTyphoonTargets([]byte(`[{"tropicalCyclone":"TC2601"},{"tropicalCyclone":"../x"},
	  {"tropicalCyclone":"TC2601"},{"tropicalCyclone":"TC26011"},{"tropicalCyclone":"tc2602"},{"tropicalCyclone":"TC2602?a=1"},
	  {"tropicalCyclone":"TC2603"},{"tropicalCyclone":"TC2604"},{"tropicalCyclone":"TC2605"},{"tropicalCyclone":"TC2606"},{"tropicalCyclone":"TC2607"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"TC2601", "TC2603", "TC2604", "TC2605", "TC2606"}; !slices.Equal(got, want) {
		t.Errorf("ids = %v, want %v", got, want)
	}
	if _, err := parseTyphoonTargets([]byte(`{}`)); err == nil {
		t.Error("an object should fail")
	}
}

// typhoonTY is written by hand in the shape of a specifications.json for a
// typhoon (the fixture's cyclone had no wind); field names as in JMA's files.
const typhoonTY = `[
 {"part":"title","typhoonNumber":"2610","name":{"jp":"テスト","en":"Test"},"category":{"jp":"台風","en":"TY"}},
 {"part":{"jp":"実況","en":"Analysis"},"advancedHours":0,"category":{"jp":"台風","en":"TY"},"scale":"大型","intensity":"非常に強い",
  "position":{"deg":[24.5,128.3]},"location":"沖縄の南","course":"北北西","speed":{"km/h":"ゆっくり"},"pressure":945,
  "maximumWind":{"sustained":{"m/s":"45","kt":"90"},"gust":{"m/s":"60","kt":"130"}},
  "validtime":{"JST":"2026-09-01T09:00:00+09:00"}},
 {"part":{"jp":"推定","en":"Estimate"},"advancedHours":1,"validtime":{"JST":"2026-09-01T10:00:00+09:00"}},
 {"part":{"jp":"予報","en":"Forecast"},"advancedHours":24,"pressure":"950","validtime":{"JST":"2026-09-02T09:00:00+09:00"}},
 {"part":{"jp":"予報","en":"Forecast"},"advancedHours":120,"validtime":{"JST":"2026-09-06T09:00:00+09:00"}},
 {"part":{"jp":"予報","en":"Forecast"},"advancedHours":72,"validtime":{"JST":"2026-09-04T09:00:00+09:00"}}
]`

func TestParseTyphoonSpec(t *testing.T) {
	it, err := parseTyphoonSpec([]byte(typhoonTY))
	if err != nil {
		t.Fatal(err)
	}
	want := Item{Date: "2026-09-01T09:00:00+09:00", Title: "台風第10号(テスト)", Icon: "tropicalstorm",
		Subtitle: "大型で非常に強い · 沖縄の南(北緯24.5度・東経128.3度) · 中心気圧 945 hPa · 最大風速 45 m/s(瞬間 60 m/s) · 北北西 ゆっくり · 予報 9月6日9時まで"}
	if !reflect.DeepEqual(it, want) {
		t.Errorf("item =\n%+v\nwant\n%+v", it, want)
	}

	// No number yet (熱帯低気圧) and out-of-range values left out.
	td := `[{"part":"title","typhoonNumber":"","category":{"jp":"熱帯低気圧","en":"TD"}},
	 {"part":{"jp":"実況"},"advancedHours":0,"scale":"-","intensity":"-","pressure":"5000","speed":{"km/h":"-"},"course":"-",
	  "maximumWind":{"sustained":{"m/s":"999"}},"position":{"deg":[-10,20]},"validtime":{"JST":"2026-09-01T09:00:00+09:00"}}]`
	it, err = parseTyphoonSpec([]byte(td))
	if err != nil {
		t.Fatal(err)
	}
	if it.Title != "熱帯低気圧" || it.Subtitle != "" {
		t.Errorf("td = %+v", it)
	}

	for _, bad := range []string{`[]`, `[{"part":"title"}]`, `[{"part":"title"},{"advancedHours":0,"validtime":{"JST":"x"}}]`, `nope`} {
		if _, err := parseTyphoonSpec([]byte(bad)); err == nil {
			t.Errorf("%s should fail", bad)
		}
	}
}

func TestTyphoonEndpoints(t *testing.T) {
	ep := DefaultEndpoints()
	for _, u := range []string{ep.typhoonListURL(), ep.typhoonSpecURL("TC2638")} {
		if !ep.allowedURL(u) || !strings.HasPrefix(u, "https://www.jma.go.jp/bosai/typhoon/data/") {
			t.Errorf("%s", u)
		}
	}
}
