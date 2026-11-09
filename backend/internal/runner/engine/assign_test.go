package engine

import (
	"context"
	"fmt"
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

// fewest + exclude in an on.each run: pick's once-per-run rule for exclude
// does not apply; the least loaded member gets several rows while still the
// least loaded (earlier rows of the run are counted instead).
func TestPlanAssignEachExcludeRepeats(t *testing.T) {
	a := readsApp(t, "server_rules_assign.json")
	r := rule(t, a, "assign_daily")
	r.Pick.Exclude = &dsl.PickExclude{Entity: "offs", Field: "staff", Match: &dsl.PickMatch{Field: "day", Equals: "day"}}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	var shifts []expreval.Record
	for i, s := range []string{"s2", "s2", "s2", "s3", "s3", "s3"} {
		shifts = append(shifts, rec(fmt.Sprintf("busy%d", i), map[string]any{"day": "2026-11-01", "staff": s}))
	}
	shifts = append(shifts,
		rec("y1", map[string]any{"day": "2026-11-10"}),
		rec("y2", map[string]any{"day": "2026-11-11"}),
		rec("y3", map[string]any{"day": "2026-11-12"}),
		rec("y4", map[string]any{"day": "2026-11-13"}))
	offs := []expreval.Record{rec("o1", map[string]any{"staff": "s1", "day": "2026-11-13"})} // s1 is off on y4's day
	l := &listReader{recs: map[string][]expreval.Record{"staff": staff(), "shifts": shifts, "offs": offs}}
	ev := Trigger{AppID: "a", Kind: KindScheduled, Scope: dsl.ScopeShared, At: readsAt, Location: jst, RunID: "run-x"}
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
	// s1 (0, 1, 2 < 3) takes three rows; it is off on y4, which goes to s2
	// (3, tied with s3 and older).
	want := map[string]any{"y1": "s1", "y2": "s1", "y3": "s1", "y4": "s2"}
	if len(got) != len(want) {
		t.Fatalf("writes %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("writes %v, want %v", got, want)
		}
	}
}

// eachPicks plans an on.each run: row id -> the "$pick" its write carries.
func eachPicks(t *testing.T, a *dsl.App, r *dsl.ServerRule, ev Trigger, l *listReader) map[string]any {
	t.Helper()
	rows, err := PlanEach(context.Background(), a, r, ev, l)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]any{}
	for _, row := range rows {
		for _, w := range row.Plan.Writes {
			got[row.Trigger.Record.ID] = w.Values["staff"]
		}
	}
	return got
}

func samePicks(t *testing.T, got, want map[string]any) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("picks %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("picks %v, want %v", got, want)
		}
	}
}

// A retried on.each + createRecord run whose first attempt created the
// records of some rows picks what the first attempt picked: what it created
// for any row is left out (earlier rows count once, through the run's own
// tally; later rows' did not exist while the first attempt planned).
func TestPlanAssignEachCreateRetry(t *testing.T) {
	a := readsApp(t, "server_rules_assign.json")
	r := rule(t, a, "assign_daily")
	r.If = nil
	r.Pick.Count = &dsl.PickCount{Entity: "offs", Field: "staff"}
	r.Do = []dsl.RuleAction{{CreateRecord: &dsl.RuleCreateRecord{Entity: "offs", Values: map[string]dsl.Expr{
		"staff": {Kind: dsl.ExprRef, Ref: dsl.PickRef}, "day": {Kind: dsl.ExprRef, Ref: "day"}}}}}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	shifts := []expreval.Record{
		rec("y1", map[string]any{"day": "2026-11-10"}),
		rec("y2", map[string]any{"day": "2026-11-11"}),
		rec("y3", map[string]any{"day": "2026-11-12"}),
		rec("y4", map[string]any{"day": "2026-11-13"}),
		rec("y5", map[string]any{"day": "2026-11-14"}),
	}
	created := func(rule string, action int, row string) string { return fmt.Sprintf("c/%s/%d/%s", rule, action, row) }
	ev := Trigger{AppID: "a", Kind: KindScheduled, Scope: dsl.ScopeShared, At: readsAt, Location: jst, RunID: "run-c", CreatedID: created}
	offs := []expreval.Record{rec("o1", map[string]any{"staff": "s2"})}
	l := &listReader{recs: map[string][]expreval.Record{"staff": staff(), "shifts": shifts, "offs": offs}}
	first := eachPicks(t, a, r, ev, l)
	// s2 has 1: s1, s3 (s1 older), then all at 1 -> s1, s2, s3.
	samePicks(t, first, map[string]any{"y1": "s1", "y2": "s3", "y3": "s1", "y4": "s2", "y5": "s3"})
	// The first attempt created y1's, y2's and y4's (writes stop anywhere).
	for _, row := range []string{"y1", "y2", "y4"} {
		l.recs["offs"] = append(l.recs["offs"], rec(created(r.ID, 0, row), map[string]any{"staff": first[row]}))
	}
	samePicks(t, eachPicks(t, a, r, ev, l), first)
}

// updateRecord of the row (no if): a row already holding what it picks
// counts once, so the first attempt and a retry after some writes agree.
func TestPlanAssignEachUpdateRetry(t *testing.T) {
	a := readsApp(t, "server_rules_assign.json")
	r := rule(t, a, "assign_daily")
	r.If = nil
	shifts := []expreval.Record{
		rec("x1", map[string]any{"day": "2026-11-09", "staff": "s1"}),
		rec("x2", map[string]any{"day": "2026-11-10"}),
		rec("x3", map[string]any{"day": "2026-11-11"}),
		rec("x4", map[string]any{"day": "2026-11-12"}),
	}
	l := &listReader{recs: map[string][]expreval.Record{"staff": staff(), "shifts": shifts}}
	ev := Trigger{AppID: "a", Kind: KindScheduled, Scope: dsl.ScopeShared, At: readsAt, Location: jst, RunID: "run-u"}
	first := eachPicks(t, a, r, ev, l)
	// x1 keeps s1 (no write; one shift, not two), then s2, s3, and s1 again.
	samePicks(t, first, map[string]any{"x2": "s2", "x3": "s3", "x4": "s1"})
	// The first attempt wrote x2: it is unchanged now, the rest as before.
	shifts[1] = rec("x2", map[string]any{"day": "2026-11-10", "staff": "s2"})
	samePicks(t, eachPicks(t, a, r, ev, l), map[string]any{"x3": "s3", "x4": "s1"})
}
