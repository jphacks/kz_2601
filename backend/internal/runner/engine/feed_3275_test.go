package engine

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// feedRefs3275 is a RefReader that is also a FeedReader with a fixed answer.
type feedRefs3275 struct {
	snap          FeedSnapshot
	err           error
	gotProv, gotR string
	calls         int
}

func (f *feedRefs3275) Record(context.Context, string, string) (expreval.Record, bool, error) {
	return expreval.Record{}, false, nil
}

func (f *feedRefs3275) Feed(_ context.Context, provider, region string) (FeedSnapshot, error) {
	f.calls++
	f.gotProv, f.gotR = provider, region
	return f.snap, f.err
}

// plainRefs3275 is a RefReader that cannot read feeds.
type plainRefs3275 struct{}

func (plainRefs3275) Record(context.Context, string, string) (expreval.Record, bool, error) {
	return expreval.Record{}, false, nil
}

func fp3275(v float64) *float64 { return &v }
func bp3275(v bool) *bool       { return &v }

func feedApp3275(extra ...dsl.Field) *dsl.App {
	fields := []dsl.Field{
		{ID: "day", Type: dsl.FieldDate},
		{ID: "text", Type: dsl.FieldString},
		{ID: "lo", Type: dsl.FieldInteger},
		{ID: "hi", Type: dsl.FieldNumber},
		{ID: "drop", Type: dsl.FieldBoolean},
	}
	return &dsl.App{Entities: []dsl.Entity{{ID: "weather", Fields: append(fields, extra...)}}}
}

func feedAction3275() *dsl.RuleSaveFeed {
	return &dsl.RuleSaveFeed{Provider: "weather", Region: "130000", Entity: "weather",
		Fields: dsl.SaveFeedFields{Date: "day", Summary: "text", Min: "lo", Max: "hi", Drop: "drop"}}
}

func runSaveFeed3275(app *dsl.App, kind string, at time.Time, refs RefReader, s *dsl.RuleSaveFeed) (Plan, error) {
	p := &planner{app: app, ev: Trigger{Kind: kind, At: at}, src: &refSource{ctx: context.Background(), refs: refs}}
	var out Plan
	err := p.saveFeed(0, s, &out)
	return out, err
}

func TestPlannerSaveFeed_3275(t *testing.T) {
	at := time.Date(2026, 11, 12, 16, 30, 0, 0, time.UTC) // 01:30 on the 13th in Japan
	refs := &feedRefs3275{snap: FeedSnapshot{Summary: "晴れ", Min: fp3275(8.6), Max: fp3275(18.4), Drop: bp3275(true)}}
	out, err := runSaveFeed3275(feedApp3275(), KindScheduled, at, refs, feedAction3275())
	if err != nil {
		t.Fatal(err)
	}
	if refs.gotProv != "weather" || refs.gotR != "130000" {
		t.Fatalf("feed asked for %q/%q", refs.gotProv, refs.gotR)
	}
	want := []PlannedWrite{{Action: 0, Kind: WriteCreate, Entity: "weather", Key: "2026-11-13",
		Values: map[string]any{"day": "2026-11-13", "text": "晴れ", "lo": 9.0, "hi": 18.4, "drop": true}}}
	if !reflect.DeepEqual(out.Writes, want) {
		t.Fatalf("writes = %#v\nwant   %#v", out.Writes, want)
	}
	if len(out.Skipped) != 0 {
		t.Fatalf("skipped = %v", out.Skipped)
	}
}

func TestPlannerSaveFeedDatetimeDate_3275(t *testing.T) {
	at := time.Date(2026, 11, 12, 16, 30, 0, 0, time.UTC)
	app := &dsl.App{Entities: []dsl.Entity{{ID: "weather", Fields: []dsl.Field{{ID: "day", Type: dsl.FieldDatetime}, {ID: "text", Type: dsl.FieldString}}}}}
	s := &dsl.RuleSaveFeed{Provider: "weather", Region: "r", Entity: "weather", Fields: dsl.SaveFeedFields{Date: "day", Summary: "text"}}
	out, err := runSaveFeed3275(app, KindScheduled, at, &feedRefs3275{snap: FeedSnapshot{Summary: "雨"}}, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Writes) != 1 || out.Writes[0].Values["day"] != "2026-11-12T16:30:00Z" || out.Writes[0].Key != "2026-11-13" {
		t.Fatalf("writes = %#v", out.Writes)
	}
}

func TestPlannerSaveFeedNothingToSave_3275(t *testing.T) {
	at := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		kind string
		refs RefReader
		err  error
	}{
		"not scheduled":  {KindCreated, &feedRefs3275{snap: FeedSnapshot{Summary: "x"}}, nil},
		"not a reader":   {KindScheduled, plainRefs3275{}, nil},
		"feed skipped":   {KindScheduled, &feedRefs3275{err: ErrFeedSkip}, nil},
		"empty snapshot": {KindScheduled, &feedRefs3275{}, nil},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := runSaveFeed3275(feedApp3275(), c.kind, at, c.refs, feedAction3275())
			if err != nil {
				t.Fatal(err)
			}
			if len(out.Writes) != 0 || len(out.Skipped) != 0 {
				t.Fatalf("plan = %#v", out)
			}
		})
	}
}

func TestPlannerSaveFeedNotScheduledNeverReads_3275(t *testing.T) {
	refs := &feedRefs3275{snap: FeedSnapshot{Summary: "x"}}
	if _, err := runSaveFeed3275(feedApp3275(), KindCreated, time.Now(), refs, feedAction3275()); err != nil {
		t.Fatal(err)
	}
	if refs.calls != 0 {
		t.Fatalf("feed read %d times on a non-scheduled event", refs.calls)
	}
}

func TestPlannerSaveFeedErrors_3275(t *testing.T) {
	at := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	s := feedAction3275()
	s.Entity = "nope"
	if _, err := runSaveFeed3275(feedApp3275(), KindScheduled, at, &feedRefs3275{}, s); err == nil || !strings.Contains(err.Error(), `"nope"`) {
		t.Fatalf("unknown entity err = %v", err)
	}
	boom := errors.New("feed down")
	if _, err := runSaveFeed3275(feedApp3275(), KindScheduled, at, &feedRefs3275{err: boom}, feedAction3275()); !errors.Is(err, boom) {
		t.Fatalf("feed error err = %v, want it passed on (the run retries)", err)
	}
}

func TestPlannerSaveFeedRequiredEmpty_3275(t *testing.T) {
	at := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	app := feedApp3275(dsl.Field{ID: "note", Type: dsl.FieldString, Required: true})
	out, err := runSaveFeed3275(app, KindScheduled, at, &feedRefs3275{snap: FeedSnapshot{Summary: "晴れ"}}, feedAction3275())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Writes) != 0 || !reflect.DeepEqual(out.Skipped, []PlannedSkip{{Action: 0, Reason: SkipFeedRequiredEmpty}}) {
		t.Fatalf("plan = %#v", out)
	}
}

func TestPlannerSaveFeedIgnoresUnknownAndComputed_3275(t *testing.T) {
	at := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	app := feedApp3275()
	app.Entities[0].Fields[1].Computed = &dsl.Expr{} // text is read-only
	s := feedAction3275()
	s.Fields.Min = "missing"
	out, err := runSaveFeed3275(app, KindScheduled, at, &feedRefs3275{snap: FeedSnapshot{Summary: "晴れ", Min: fp3275(1), Max: fp3275(2)}}, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Writes) != 1 {
		t.Fatalf("writes = %#v", out.Writes)
	}
	v := out.Writes[0].Values
	if _, ok := v["text"]; ok {
		t.Error("computed field written")
	}
	if _, ok := v["missing"]; ok {
		t.Error("unknown field written")
	}
	if v["hi"] != 2.0 {
		t.Errorf("hi = %v", v["hi"])
	}
}
