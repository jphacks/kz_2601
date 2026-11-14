package engine

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

func menus() []expreval.Record {
	at := func(r expreval.Record, created string) expreval.Record { r.CreatedAt = created; return r }
	// listed newest first, like the store
	return []expreval.Record{
		at(rec("m4", map[string]any{"name": "唐揚げ", "category": "main", "price": float64(800)}), "2026-11-04T00:00:00Z"),
		at(rec("m3", map[string]any{"name": "サラダ", "category": "side", "price": float64(300)}), "2026-11-03T00:00:00Z"),
		at(rec("m2", map[string]any{"name": "カレー", "category": "main", "price": float64(700)}), "2026-11-02T00:00:00Z"),
		at(rec("m1", map[string]any{"name": "ラーメン", "category": "main", "price": float64(900)}), "2026-11-01T00:00:00Z"),
	}
}

func dailyPick(run string) Trigger {
	return Trigger{AppID: "a", Kind: KindScheduled, Scope: dsl.ScopeShared, At: readsAt, Location: jst, RunID: run}
}

// A time trigger without each picks a main dish nobody ate yet, writes it
// into the log and tells the members its name.
func TestPlanPickScheduled(t *testing.T) {
	a := readsApp(t, "server_rules_pick.json")
	r := rule(t, a, "daily_pick")
	eats := []expreval.Record{rec("e1", map[string]any{"menu": "m4"}), rec("e2", map[string]any{"menu": "m1"})}
	l := &listReader{recs: map[string][]expreval.Record{"menus": menus(), "eats": eats}}
	p, err := PlanRule(context.Background(), a, r, dailyPick("run-1"), l)
	if err != nil {
		t.Fatal(err)
	}
	// main and not eaten: only m2 is left.
	if len(p.Writes) != 1 || p.Writes[0].Kind != WriteCreate || p.Writes[0].Entity != "picks" {
		t.Fatalf("writes %+v", p.Writes)
	}
	v := p.Writes[0].Values
	if v["menu"] != "m2" || v["name"] != "カレー" || v["day"] != "2026-11-08" {
		t.Fatalf("values %v", v)
	}
	if _, ok := v[dsl.PickRef]; ok {
		t.Fatalf("a result is written: %v", v)
	}
	if len(p.Notifies) != 1 || p.Notifies[0].Body != "まだ食べていない カレー はどうですか" {
		t.Fatalf("notifies %+v", p.Notifies)
	}
	if l.lists["menus"] != 1 || l.lists["eats"] != 1 {
		t.Fatalf("lists %v", l.lists)
	}
	// Everything eaten: nothing runs.
	l.recs["eats"] = append(l.recs["eats"], rec("e3", map[string]any{"menu": "m2"}))
	if p, err := PlanRule(context.Background(), a, r, dailyPick("run-1"), l); err != nil || !p.Empty() {
		t.Fatalf("nothing left: %+v %v", p, err)
	}
}

// Random is fixed by the run: the same run (a retry) picks the same record
// whatever the list order, other runs spread over the candidates.
func TestPlanPickRandomPerRun(t *testing.T) {
	a := readsApp(t, "server_rules_pick.json")
	r := rule(t, a, "daily_pick")
	r.Pick.Filter, r.Pick.Exclude = nil, nil
	pick := func(run string, recs []expreval.Record) string {
		t.Helper()
		l := &listReader{recs: map[string][]expreval.Record{"menus": recs}}
		p, err := PlanRule(context.Background(), a, r, dailyPick(run), l)
		if err != nil || len(p.Writes) != 1 {
			t.Fatalf("plan %+v %v", p, err)
		}
		return p.Writes[0].Values["menu"].(string)
	}
	reversed := menus()
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	seen := map[string]bool{}
	for i := range 40 {
		run := fmt.Sprintf("run-%d", i)
		got := pick(run, menus())
		if again := pick(run, reversed); again != got {
			t.Fatalf("%s: %s then %s", run, got, again)
		}
		seen[got] = true
	}
	if len(seen) < 3 {
		t.Fatalf("40 runs picked only %v", seen)
	}
}

// A retry after the log record was written still picks the same dish: the
// record this run created does not exclude it.
func TestPlanPickRetryKeepsChoice(t *testing.T) {
	a := readsApp(t, "server_rules_pick.json")
	r := rule(t, a, "daily_pick")
	r.Pick.Exclude = &dsl.PickExclude{Entity: "picks", Field: "menu"}
	r.Pick.Filter = nil
	created := func(rule string, action int, row string) string {
		return "c/" + rule + "/" + fmt.Sprint(action) + "/" + row
	}
	ev := dailyPick("run-7")
	ev.CreatedID = created
	l := &listReader{recs: map[string][]expreval.Record{"menus": menus()}}
	p, err := PlanRule(context.Background(), a, r, ev, l)
	if err != nil {
		t.Fatal(err)
	}
	first := p.Writes[0].Values["menu"]
	// the first attempt's create landed; the retry lists it
	l.recs["picks"] = []expreval.Record{rec(created("daily_pick", 0, ""), map[string]any{"menu": first})}
	l.lists = nil
	p, err = PlanRule(context.Background(), a, r, ev, l)
	if err != nil || len(p.Writes) != 1 || p.Writes[0].Values["menu"] != first {
		t.Fatalf("retry %+v %v (first %v)", p, err, first)
	}
	// an earlier day's log does exclude it
	l.recs["picks"][0].ID = "old"
	p, err = PlanRule(context.Background(), a, r, ev, l)
	if err != nil || len(p.Writes) != 1 || p.Writes[0].Values["menu"] == first {
		t.Fatalf("next day %+v %v (first %v)", p, err, first)
	}
}

// oldest / newest follow the created time; the row itself is never picked.
func TestPlanPickOrder(t *testing.T) {
	a := readsApp(t, "server_rules_pick.json")
	r := rule(t, a, "next_after_eat")
	l := &listReader{recs: map[string][]expreval.Record{"menus": menus(), "eats": {rec("e1", map[string]any{"menu": "m1"})}}}
	ev := Trigger{AppID: "a", Kind: KindCreated, Entity: "eats", Record: rec("e1", map[string]any{"menu": "m1"}), At: readsAt, Location: jst}
	p, err := PlanRule(context.Background(), a, r, ev, l)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Writes) != 1 || p.Writes[0].RecordID != "e1" || p.Writes[0].Values["next"] != "m2" {
		t.Fatalf("oldest not eaten: %+v", p.Writes)
	}
	if len(p.Notifies) != 1 || p.Notifies[0].Body != "次は カレー(700 円)はどうですか" {
		t.Fatalf("notifies %+v", p.Notifies)
	}
	r.Pick.Order = dsl.PickNewest
	if p, err := PlanRule(context.Background(), a, r, ev, l); err != nil || p.Writes[0].Values["next"] != "m4" {
		t.Fatalf("newest: %+v %v", p, err)
	}
	// picking from the trigger's own entity skips the row
	a.Entities[1].Fields = append(a.Entities[1].Fields, dsl.Field{ID: "other", Name: "別", Type: dsl.FieldRef, RefEntity: "eats"})
	r.Pick = &dsl.RulePick{Entity: "eats", Order: dsl.PickNewest}
	r.Do = []dsl.RuleAction{{UpdateRecord: &dsl.RuleUpdateRecord{Values: map[string]dsl.Expr{"other": {Kind: dsl.ExprRef, Ref: "$pick"}}}}}
	l.recs["eats"] = []expreval.Record{rec("e1", nil)}
	if p, err := PlanRule(context.Background(), a, r, ev, l); err != nil || !p.Empty() {
		t.Fatalf("only the row: %+v %v", p, err)
	}
}

// authorOf $pick tells the author of the picked record.
func TestPlanPickAuthorOf(t *testing.T) {
	a := readsApp(t, "server_rules_pick.json")
	r := rule(t, a, "daily_pick")
	r.Do[1].Notify.To = dsl.RuleRecipient{Kind: dsl.RecipientAuthorOf, Field: dsl.PickRef}
	l := &listReader{recs: map[string][]expreval.Record{"menus": menus(), "eats": nil}}
	p, err := PlanRule(context.Background(), a, r, dailyPick("run-1"), l)
	if err != nil {
		t.Fatal(err)
	}
	n := p.Notifies
	if len(n) != 1 || n[0].RefEntity != "menus" || n[0].RefID != p.Writes[0].Values["menu"] {
		t.Fatalf("notifies %+v writes %+v", n, p.Writes)
	}
}

// on.each picks per row (seeded by the row too) and shares one list.
func TestPlanPickEach(t *testing.T) {
	a := readsApp(t, "server_rules_pick.json")
	r := rule(t, a, "next_after_eat")
	r.On = dsl.RuleTrigger{Every: "daily", Time: "08:00", Each: "eats"}
	r.Pick.Exclude = nil
	r.Do = r.Do[:1]
	var eats []expreval.Record
	for i := range 20 {
		eats = append(eats, rec(fmt.Sprintf("e%d", i), map[string]any{"menu": "m1"}))
	}
	r.Pick.Order = dsl.PickRandom
	l := &listReader{recs: map[string][]expreval.Record{"menus": menus(), "eats": eats}}
	rows, err := PlanEach(context.Background(), a, r, dailyPick("run-1"), l)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[any]bool{}
	for _, rp := range rows {
		seen[rp.Plan.Writes[0].Values["next"]] = true
	}
	if len(rows) != 20 || len(seen) < 2 || l.lists["menus"] != 1 {
		t.Fatalf("rows %d picks %v lists %v", len(rows), seen, l.lists)
	}
}

// With exclude, the rows of one on.each run never pick the same record: the
// run plans every row before writing, so the earlier rows' picks are skipped
// (oldest / newest / random alike). Rows past the candidates pick nothing.
func TestPlanPickEachExcludeSpreads(t *testing.T) {
	for _, order := range []string{dsl.PickOldest, dsl.PickNewest, dsl.PickRandom} {
		a := readsApp(t, "server_rules_pick.json")
		r := rule(t, a, "next_after_eat")
		r.On = dsl.RuleTrigger{Every: "daily", Time: "08:00", Each: "eats"}
		r.Pick.Order = order
		r.Do = r.Do[:1]
		var eats []expreval.Record
		for i := range 4 {
			eats = append(eats, rec(fmt.Sprintf("e%d", i), map[string]any{"menu": "m1"}))
		}
		l := &listReader{recs: map[string][]expreval.Record{"menus": menus(), "eats": eats}}
		rows, err := PlanEach(context.Background(), a, r, dailyPick("run-1"), l)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[any]bool{}
		for _, rp := range rows {
			seen[rp.Plan.Writes[0].Values["next"]] = true
		}
		// m1 is eaten: m2, m3, m4 once each, the 4th row has nothing left
		if len(rows) != 3 || len(seen) != 3 || seen["m1"] {
			t.Fatalf("%s: rows %d picks %v", order, len(rows), seen)
		}
		// a retry of the run picks the same records
		again, err := PlanEach(context.Background(), a, r, dailyPick("run-1"), l)
		if err != nil || len(again) != len(rows) {
			t.Fatalf("%s retry: %d %v", order, len(again), err)
		}
		for i := range rows {
			if again[i].Plan.Writes[0].Values["next"] != rows[i].Plan.Writes[0].Values["next"] {
				t.Fatalf("%s retry row %d differs", order, i)
			}
		}
	}
}

// A failed list stops the run (never a pick from part of the records).
func TestPlanPickListError(t *testing.T) {
	a := readsApp(t, "server_rules_pick.json")
	boom := errors.New("boom")
	l := &listReader{err: boom}
	if _, err := PlanRule(context.Background(), a, rule(t, a, "daily_pick"), dailyPick("run-1"), l); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}
