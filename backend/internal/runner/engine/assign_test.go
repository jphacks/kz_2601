package engine

import (
	"context"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

func staff() []expreval.Record {
	at := func(r expreval.Record, created string) expreval.Record { r.CreatedAt = created; return r }
	// listed newest first, like the store
	return []expreval.Record{
		at(rec("s3", map[string]any{"name": "佐藤"}), "2026-11-03T00:00:00Z"),
		at(rec("s2", map[string]any{"name": "鈴木"}), "2026-11-02T00:00:00Z"),
		at(rec("s1", map[string]any{"name": "田中"}), "2026-11-01T00:00:00Z"),
	}
}

func newShift(id, day string) Trigger {
	return Trigger{AppID: "a", Kind: KindCreated, Scope: dsl.ScopeShared, Entity: "shifts",
		Record: rec(id, map[string]any{"day": day}), At: readsAt, Location: jst, RunID: "run-" + id}
}

// A new shift goes to the staff member with the fewest shifts who is not
// off that day; the row itself is not counted; ties go to the one added
// first.
func TestPlanAssignCreated(t *testing.T) {
	a := readsApp(t, "server_rules_assign.json")
	r := rule(t, a, "assign_new")
	shifts := []expreval.Record{
		rec("x1", map[string]any{"day": "2026-11-01", "staff": "s1"}),
		rec("x2", map[string]any{"day": "2026-11-02", "staff": "s1"}),
		rec("x3", map[string]any{"day": "2026-11-03", "staff": "s2"}),
		rec("new", map[string]any{"day": "2026-11-10"}), // the row (already listed)
	}
	offs := []expreval.Record{
		rec("o1", map[string]any{"staff": "s3", "day": "2026-11-10"}), // s3 is off that day
		rec("o2", map[string]any{"staff": "s2", "day": "2026-11-11"}), // another day: no effect
	}
	l := &listReader{recs: map[string][]expreval.Record{"staff": staff(), "shifts": shifts, "offs": offs}}
	p, err := PlanRule(context.Background(), a, r, newShift("new", "2026-11-10"), l)
	if err != nil {
		t.Fatal(err)
	}
	// s3 (0 shifts) is off, s2 has 1, s1 has 2.
	if len(p.Writes) != 1 || p.Writes[0].Kind != WriteUpdate || p.Writes[0].RecordID != "new" || p.Writes[0].Values["staff"] != "s2" {
		t.Fatalf("writes %+v", p.Writes)
	}
	if len(p.Notifies) != 1 || p.Notifies[0].RefEntity != "staff" || p.Notifies[0].RefID != "s2" || p.Notifies[0].Body != "2026-11-10 は 鈴木 さんの担当です" {
		t.Fatalf("notifies %+v", p.Notifies)
	}
	// Nobody off: s3 (0) wins. s3 and a new s4 tie at 0: the one added first.
	l.recs["offs"] = nil
	l.recs["staff"] = append([]expreval.Record{{ID: "s4", Data: map[string]any{"name": "高橋"}, CreatedAt: "2026-11-04T00:00:00Z"}}, staff()...)
	p, err = PlanRule(context.Background(), a, r, newShift("new", "2026-11-10"), l)
	if err != nil || p.Writes[0].Values["staff"] != "s3" {
		t.Fatalf("tie: %+v %v", p.Writes, err)
	}
	// Everyone off: nothing runs.
	l.recs["staff"] = staff()
	for _, s := range []string{"s1", "s2", "s3"} {
		l.recs["offs"] = append(l.recs["offs"], rec("o-"+s, map[string]any{"staff": s, "day": "2026-11-10"}))
	}
	if p, err := PlanRule(context.Background(), a, r, newShift("new", "2026-11-10"), l); err != nil || !p.Empty() {
		t.Fatalf("all off: %+v %v", p, err)
	}
}

// A retry after the write counts the same: the row is never counted, even
// when it already holds the assignment.
func TestPlanAssignRetry(t *testing.T) {
	a := readsApp(t, "server_rules_assign.json")
	r := rule(t, a, "assign_new")
	shifts := []expreval.Record{
		rec("x1", map[string]any{"day": "2026-11-01", "staff": "s1"}),
		rec("new", map[string]any{"day": "2026-11-10", "staff": "s2"}), // written by the first attempt
	}
	l := &listReader{recs: map[string][]expreval.Record{"staff": staff(), "shifts": shifts}}
	p, err := PlanRule(context.Background(), a, r, newShift("new", "2026-11-10"), l)
	if err != nil {
		t.Fatal(err)
	}
	// s2 and s3 tie at 0 (the row is not counted): s2 is older.
	if len(p.Writes) != 1 || p.Writes[0].Values["staff"] != "s2" {
		t.Fatalf("writes %+v", p.Writes)
	}
}

// A daily run fills the unassigned shifts in turn: each row counts the
// rows assigned before it in the run; assigned rows are skipped by if.
func TestPlanAssignEachRoundRobin(t *testing.T) {
	a := readsApp(t, "server_rules_assign.json")
	r := rule(t, a, "assign_daily")
	shifts := []expreval.Record{
		rec("x1", map[string]any{"day": "2026-11-09", "staff": "s1"}),
		rec("x2", map[string]any{"day": "2026-11-10"}),
		rec("x3", map[string]any{"day": "2026-11-11"}),
		rec("x4", map[string]any{"day": "2026-11-12"}),
		rec("x5", map[string]any{"day": "2026-11-13"}),
	}
	l := &listReader{recs: map[string][]expreval.Record{"staff": staff(), "shifts": shifts}}
	ev := Trigger{AppID: "a", Kind: KindScheduled, Scope: dsl.ScopeShared, At: readsAt, Location: jst, RunID: "run-d"}
	rows, err := PlanEach(context.Background(), a, r, ev, l)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]any{}
	for _, row := range rows {
		for _, w := range row.Plan.Writes {
			got[w.RecordID] = w.Values["staff"]
		}
	}
	// s1 has 1: s2, s3 first (s2 older), then s1 and s2 tie at 1 (s1 older).
	want := map[string]any{"x2": "s2", "x3": "s3", "x4": "s1", "x5": "s2"}
	if len(got) != len(want) {
		t.Fatalf("writes %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("writes %v, want %v", got, want)
		}
	}
	if l.lists["shifts"] != 1 || l.lists["staff"] != 1 {
		t.Fatalf("lists %v", l.lists)
	}
}

// most picks the candidate the most records point to (the day most can
// attend) and nothing when none does; a record this run creates is not
// counted.
func TestPlanAssignMost(t *testing.T) {
	a := readsApp(t, "server_rules_assign.json")
	r := rule(t, a, "assign_daily")
	r.On.Each, r.If = "", nil
	r.Pick.Order = dsl.PickMost
	r.Pick.Count = &dsl.PickCount{Entity: "offs", Field: "staff"}
	r.Do = []dsl.RuleAction{{CreateRecord: &dsl.RuleCreateRecord{Entity: "offs", Values: map[string]dsl.Expr{
		"staff": {Kind: dsl.ExprRef, Ref: dsl.PickRef}, "day": {Kind: dsl.ExprOp, Op: "today"}}}}}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	ev := Trigger{AppID: "a", Kind: KindScheduled, Scope: dsl.ScopeShared, At: readsAt, Location: jst, RunID: "run-m",
		CreatedID: func(rule string, action int, row string) string { return "created-" + rule }}
	l := &listReader{recs: map[string][]expreval.Record{"staff": staff(), "offs": nil}}
	if p, err := PlanRule(context.Background(), a, r, ev, l); err != nil || !p.Empty() {
		t.Fatalf("none: %+v %v", p, err)
	}
	l.recs["offs"] = []expreval.Record{
		rec("o1", map[string]any{"staff": "s1"}),
		rec("o2", map[string]any{"staff": "s3"}),
		rec("o3", map[string]any{"staff": "s3"}),
		rec("created-assign_daily", map[string]any{"staff": "s1"}), // this run's own (a retry)
		rec("created-assign_daily-2", map[string]any{"staff": "s1"}),
	}
	p, err := PlanRule(context.Background(), a, r, ev, l)
	if err != nil {
		t.Fatal(err)
	}
	// s1: 2 (own create not counted), s3: 2 -> tie, s1 is older.
	if len(p.Writes) != 1 || p.Writes[0].Values["staff"] != "s1" {
		t.Fatalf("writes %+v", p.Writes)
	}
}
