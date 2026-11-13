package engine

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

const saveFeedApp_S7Backend006 = `{
  "schemaVersion": 1,
  "app": {"name": "気圧", "voiceEntry": "floating"},
  "initialScreen": "home",
  "entities": [
    {"id": "plog", "name": "気圧", "fields": [
      {"id": "day", "name": "日", "type": "date", "required": true},
      {"id": "note", "name": "メモ", "type": "string"},
      {"id": "low", "name": "最低", "type": "number"},
      {"id": "high", "name": "最高", "type": "integer"},
      {"id": "falling", "name": "低下", "type": "boolean"}
    ]},
    {"id": "dlog", "name": "日時", "fields": [
      {"id": "at", "name": "時刻", "type": "datetime"},
      {"id": "note", "name": "メモ", "type": "string"}
    ]},
    {"id": "req", "name": "必須", "fields": [
      {"id": "day", "name": "日", "type": "date"},
      {"id": "text", "name": "文", "type": "string", "required": true}
    ]}
  ],
  "screens": [
    {"id": "home", "title": "気圧", "components": [{"type": "list", "entity": "plog", "item": {"title": "note"}}]}
  ],
  "serverRules": [
    {"id": "save", "on": {"every": "daily", "time": "07:00"},
     "do": [{"saveFeed": {"provider": "pressure", "region": "130000", "entity": "plog", "fields": {"date": "day", "summary": "note"}}}]}
  ]
}`

// feedRefs_S7Backend006 is a RefReader that is also a FeedReader.
type feedRefs_S7Backend006 struct {
	snap FeedSnapshot
	err  error
	got  [][2]string
}

func (f *feedRefs_S7Backend006) Record(context.Context, string, string) (expreval.Record, bool, error) {
	return expreval.Record{}, false, nil
}

func (f *feedRefs_S7Backend006) Feed(_ context.Context, provider, region string) (FeedSnapshot, error) {
	f.got = append(f.got, [2]string{provider, region})
	return f.snap, f.err
}

// plainRefs_S7Backend006 reads no feed.
type plainRefs_S7Backend006 struct{}

func (plainRefs_S7Backend006) Record(context.Context, string, string) (expreval.Record, bool, error) {
	return expreval.Record{}, false, nil
}

func f64_S7Backend006(v float64) *float64 { return &v }
func bool_S7Backend006(v bool) *bool      { return &v }

// runSaveFeed_S7Backend006 runs saveFeed once for the action s at ev.
func runSaveFeed_S7Backend006(t *testing.T, refs RefReader, ev Trigger, s *dsl.RuleSaveFeed) (Plan, error) {
	t.Helper()
	a, err := dsl.Parse([]byte(saveFeedApp_S7Backend006))
	if err != nil {
		t.Fatal(err)
	}
	p := planner{app: a, ev: ev, src: &refSource{ctx: context.Background(), refs: refs}}
	var out Plan
	err = p.saveFeed(3, s, &out)
	return out, err
}

func TestPlannerSaveFeed_S7Backend006(t *testing.T) {
	t.Parallel()
	// 2026-11-12 23:30 UTC is already the 13th in Japan.
	at := time.Date(2026, 11, 12, 23, 30, 0, 0, time.UTC)
	ev := Trigger{AppID: "a", Kind: KindScheduled, At: at}
	pressure := &dsl.RuleSaveFeed{Provider: "pressure", Region: "130000", Entity: "plog",
		Fields: dsl.SaveFeedFields{Date: "day", Summary: "note", Min: "low", Max: "high", Drop: "falling"}}

	t.Run("writes one create keyed by the Japan day", func(t *testing.T) {
		t.Parallel()
		refs := &feedRefs_S7Backend006{snap: FeedSnapshot{Summary: "低下", Min: f64_S7Backend006(1004), Max: f64_S7Backend006(1012.6), Drop: bool_S7Backend006(true)}}
		out, err := runSaveFeed_S7Backend006(t, refs, ev, pressure)
		if err != nil {
			t.Fatal(err)
		}
		want := []PlannedWrite{{Action: 3, Kind: WriteCreate, Entity: "plog", Key: "2026-11-13",
			Values: map[string]any{"day": "2026-11-13", "note": "低下", "low": 1004.0, "high": 1013.0, "falling": true}}}
		if !reflect.DeepEqual(out.Writes, want) {
			t.Fatalf("writes %+v", out.Writes)
		}
		if !reflect.DeepEqual(refs.got, [][2]string{{"pressure", "130000"}}) {
			t.Fatalf("feed asked %v", refs.got)
		}
	})

	t.Run("a datetime date field gets the run time", func(t *testing.T) {
		t.Parallel()
		refs := &feedRefs_S7Backend006{snap: FeedSnapshot{Summary: "晴れ"}}
		s := &dsl.RuleSaveFeed{Provider: "weather", Region: "130000", Entity: "dlog", Fields: dsl.SaveFeedFields{Date: "at", Summary: "note"}}
		out, err := runSaveFeed_S7Backend006(t, refs, ev, s)
		if err != nil || len(out.Writes) != 1 {
			t.Fatalf("%+v %v", out, err)
		}
		if got := out.Writes[0].Values["at"]; got != "2026-11-12T23:30:00Z" {
			t.Fatalf("at %v", got)
		}
		if out.Writes[0].Key != "2026-11-13" {
			t.Fatalf("key %q", out.Writes[0].Key)
		}
	})

	t.Run("fields missing from the snapshot or the entity are left out", func(t *testing.T) {
		t.Parallel()
		refs := &feedRefs_S7Backend006{snap: FeedSnapshot{Summary: "x"}}
		s := &dsl.RuleSaveFeed{Provider: "pressure", Region: "130000", Entity: "plog",
			Fields: dsl.SaveFeedFields{Date: "day", Summary: "note", Min: "low", Max: "nope"}}
		out, err := runSaveFeed_S7Backend006(t, refs, ev, s)
		if err != nil || len(out.Writes) != 1 {
			t.Fatalf("%+v %v", out, err)
		}
		want := map[string]any{"day": "2026-11-13", "note": "x"}
		if !reflect.DeepEqual(out.Writes[0].Values, want) {
			t.Fatalf("values %v", out.Writes[0].Values)
		}
	})

	t.Run("an empty snapshot saves nothing", func(t *testing.T) {
		t.Parallel()
		out, err := runSaveFeed_S7Backend006(t, &feedRefs_S7Backend006{}, ev, pressure)
		if err != nil || len(out.Writes) != 0 || len(out.Skipped) != 0 {
			t.Fatalf("%+v %v", out, err)
		}
	})

	t.Run("a required field left empty is skipped with a reason", func(t *testing.T) {
		t.Parallel()
		refs := &feedRefs_S7Backend006{snap: FeedSnapshot{Max: f64_S7Backend006(20)}}
		s := &dsl.RuleSaveFeed{Provider: "weather", Region: "130000", Entity: "req", Fields: dsl.SaveFeedFields{Date: "day", Summary: "text", Max: "nope"}}
		out, err := runSaveFeed_S7Backend006(t, refs, ev, s)
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Writes) != 0 || !reflect.DeepEqual(out.Skipped, []PlannedSkip{{Action: 3, Reason: SkipFeedRequiredEmpty}}) {
			t.Fatalf("%+v", out)
		}
	})

	t.Run("not a time trigger does nothing", func(t *testing.T) {
		t.Parallel()
		refs := &feedRefs_S7Backend006{snap: FeedSnapshot{Summary: "x"}}
		out, err := runSaveFeed_S7Backend006(t, refs, Trigger{AppID: "a", Kind: KindCreated, At: at}, pressure)
		if err != nil || !out.Empty() || len(refs.got) != 0 {
			t.Fatalf("%+v %v %v", out, err, refs.got)
		}
	})

	t.Run("an entity not in the app is an error", func(t *testing.T) {
		t.Parallel()
		s := &dsl.RuleSaveFeed{Provider: "pressure", Region: "130000", Entity: "ghost"}
		if _, err := runSaveFeed_S7Backend006(t, &feedRefs_S7Backend006{}, ev, s); err == nil {
			t.Fatal("want an error")
		}
	})

	t.Run("a reader without Feed saves nothing", func(t *testing.T) {
		t.Parallel()
		out, err := runSaveFeed_S7Backend006(t, plainRefs_S7Backend006{}, ev, pressure)
		if err != nil || !out.Empty() {
			t.Fatalf("%+v %v", out, err)
		}
	})

	t.Run("ErrFeedSkip skips, other errors return", func(t *testing.T) {
		t.Parallel()
		out, err := runSaveFeed_S7Backend006(t, &feedRefs_S7Backend006{err: ErrFeedSkip}, ev, pressure)
		if err != nil || !out.Empty() {
			t.Fatalf("skip: %+v %v", out, err)
		}
		boom := errors.New("feed down")
		out, err = runSaveFeed_S7Backend006(t, &feedRefs_S7Backend006{err: boom}, ev, pressure)
		if !errors.Is(err, boom) || !out.Empty() {
			t.Fatalf("error: %+v %v", out, err)
		}
	})
}

func TestFeedDay_S7Backend006(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		at   time.Time
		want string
	}{
		{time.Date(2026, 11, 12, 14, 59, 0, 0, time.UTC), "2026-11-12"},
		{time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC), "2026-11-13"},
	} {
		if got := FeedDay(c.at); got != c.want {
			t.Errorf("FeedDay(%v) = %s, want %s", c.at, got, c.want)
		}
	}
}
