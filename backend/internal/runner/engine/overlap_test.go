package engine

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

func bookings() []expreval.Record {
	return []expreval.Record{
		rec("b1", map[string]any{"title": "会議", "start": "2026-11-10T10:00:00+09:00", "end": "2026-11-10T11:00:00+09:00"}),
		rec("b2", map[string]any{"title": "面談", "start": "2026-11-10T11:00:00+09:00", "end": "2026-11-10T12:00:00+09:00"}),  // right after b1
		rec("b3", map[string]any{"title": "打合せ", "start": "2026-11-10T11:30:00+09:00", "end": "2026-11-10T13:00:00+09:00"}), // overlaps b2
		rec("b4", map[string]any{"title": "壊れ", "start": "2026-11-10T15:00:00+09:00", "end": "2026-11-10T14:00:00+09:00"}),  // end before start
		rec("b5", map[string]any{"title": "印あり", "start": "2026-11-10T10:30:00+09:00", "clash": true}),                      // instant inside b1
	}
}

// Double booking: a new booking that clashes with another one notifies; one
// that only touches the end of another does not.
func TestPlanOverlapRecordTrigger(t *testing.T) {
	a := readsApp(t, "server_rules_overlap.json")
	r := rule(t, a, "double_booking")
	l := &listReader{recs: map[string][]expreval.Record{"bookings": bookings()}}
	plan := func(id, start, end string) Plan {
		t.Helper()
		data := map[string]any{"title": "新規", "start": start}
		if end != "" {
			data["end"] = end
		}
		ev := Trigger{AppID: "a", Kind: KindCreated, Entity: "bookings", Record: rec(id, data), At: readsAt, Location: jst}
		p, err := PlanRule(context.Background(), a, r, ev, l)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	if p := plan("n1", "2026-11-10T10:30:00+09:00", "2026-11-10T10:45:00+09:00"); len(p.Notifies) != 1 || p.Notifies[0].Body != "新規 がほかの予約と重なっています" {
		t.Fatalf("clash: %+v", p)
	}
	if p := plan("n2", "2026-11-10T13:00:00+09:00", "2026-11-10T14:30:00+09:00"); !p.Empty() {
		t.Fatalf("starts when b3 ends (b4 is not a period): %+v", p)
	}
	if p := plan("n3", "2026-11-10T09:00:00+09:00", "2026-11-10T08:00:00+09:00"); !p.Empty() {
		t.Fatalf("end before start fires: %+v", p)
	}
	r.Lookup.Op = dsl.LookupNotOverlaps
	if p := plan("n4", "2026-11-11T09:00:00+09:00", ""); len(p.Notifies) != 1 {
		t.Fatalf("notOverlaps: %+v", p)
	}
	if p := plan("n5", "", ""); !p.Empty() {
		t.Fatalf("empty start fires notOverlaps: %+v", p)
	}
}

// Marking clashes on every booking: the row is never its own match and the
// entity is listed once for all rows.
func TestPlanOverlapEach(t *testing.T) {
	a := readsApp(t, "server_rules_overlap.json")
	l := &listReader{recs: map[string][]expreval.Record{"bookings": bookings()}}
	ev := Trigger{AppID: "a", Kind: KindScheduled, At: readsAt, Location: jst}
	rows, err := PlanEach(context.Background(), a, rule(t, a, "clash_mark"), ev, l)
	if err != nil {
		t.Fatal(err)
	}
	var marked []string
	for _, r := range rows {
		for _, w := range r.Plan.Writes {
			marked = append(marked, w.RecordID)
		}
	}
	if !slices.Equal(marked, []string{"b1", "b2", "b3"}) {
		t.Fatalf("marked %v (b5 is already marked, b4 is not a period)", marked)
	}
	if l.lists["bookings"] != 1 {
		t.Fatalf("lists %v; the entity is read once per run", l.lists)
	}
}

// Work plans that cover a day off (ng_2412): date ranges include the end day.
func TestPlanOverlapDaysOff(t *testing.T) {
	a := readsApp(t, "server_rules_overlap.json")
	l := &listReader{recs: map[string][]expreval.Record{
		"days_off": {rec("d1", map[string]any{"day": "2026-11-14"})},
		"tasks": {
			rec("t1", map[string]any{"name": "レポート", "from": "2026-11-10", "due": "2026-11-14"}), // due on the day off
			rec("t2", map[string]any{"name": "発表", "from": "2026-11-10", "due": "2026-11-13"}),
			rec("t3", map[string]any{"name": "当日", "from": "2026-11-14"}), // no due: that day only
			rec("t4", map[string]any{"name": "未定"}),
		},
	}}
	ev := Trigger{AppID: "a", Kind: KindScheduled, At: readsAt, Location: jst}
	rows, err := PlanEach(context.Background(), a, rule(t, a, "day_off_check"), ev, l)
	if err != nil {
		t.Fatal(err)
	}
	var hit []string
	for _, r := range rows {
		hit = append(hit, r.Trigger.Record.ID)
	}
	if !slices.Equal(hit, []string{"t1", "t3"}) {
		t.Fatalf("tasks over a day off %v", hit)
	}
}

// A date field compares whole days by its type, not by the form of the
// stored value: a value kept in RFC 3339 (the field was a datetime before an
// app edit) still includes its end day.
func TestPlanOverlapDateFieldByType(t *testing.T) {
	a := readsApp(t, "server_rules_overlap.json")
	l := &listReader{recs: map[string][]expreval.Record{
		"days_off": {rec("d1", map[string]any{"day": "2026-11-14T00:00:00+09:00"})},
		"tasks": {
			rec("t1", map[string]any{"name": "混在", "from": "2026-11-10", "due": "2026-11-14T00:00:00+09:00"}),
			rec("t2", map[string]any{"name": "日時の形", "from": "2026-11-10T00:00:00+09:00", "due": "2026-11-14T00:00:00+09:00"}),
			rec("t3", map[string]any{"name": "前日まで", "from": "2026-11-10", "due": "2026-11-13T23:00:00+09:00"}),
		},
	}}
	ev := Trigger{AppID: "a", Kind: KindScheduled, At: readsAt, Location: jst}
	rows, err := PlanEach(context.Background(), a, rule(t, a, "day_off_check"), ev, l)
	if err != nil {
		t.Fatal(err)
	}
	var hit []string
	for _, r := range rows {
		hit = append(hit, r.Trigger.Record.ID)
	}
	if !slices.Equal(hit, []string{"t1", "t2"}) {
		t.Fatalf("tasks over a day off %v", hit)
	}
}

// notOverlaps never fires on part of the records: a read cap stops the run.
func TestPlanNotOverlapsReadLimit(t *testing.T) {
	a := readsApp(t, "server_rules_overlap.json")
	r := rule(t, a, "double_booking")
	r.Lookup.Op = dsl.LookupNotOverlaps
	l := &listReader{err: &ReadLimitError{Scope: "run_reads", Limit: 1, Used: 2}}
	ev := Trigger{AppID: "a", Kind: KindCreated, Entity: "bookings", Record: rec("n1", map[string]any{"title": "新規", "start": "2026-11-11T09:00:00+09:00"}), At: readsAt, Location: jst}
	p, err := PlanRule(context.Background(), a, r, ev, l)
	if !errors.Is(err, ErrReadLimit) || !p.Empty() {
		t.Fatalf("plan %+v err %v", p, err)
	}
}
