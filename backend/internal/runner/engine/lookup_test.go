package engine

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

func roster() []expreval.Record {
	return []expreval.Record{
		rec("s1", map[string]any{"student_id": "A001", "name": "青木"}),
		rec("s2", map[string]any{"student_id": " ａ００２ ", "name": "井上"}), // full width, spaces, case
		rec("s3", map[string]any{"name": "空"}),
	}
}

// Roster reconciliation (tk_2413): each registration not on the roster is
// marked; the roster is listed once for all rows.
func TestPlanLookupNotExistsEach(t *testing.T) {
	a := readsApp(t, "server_rules_lookup.json")
	l := &listReader{recs: map[string][]expreval.Record{
		"roster": roster(),
		"registrations": {
			rec("r1", map[string]any{"student_id": "a001"}),
			rec("r2", map[string]any{"student_id": "A002"}),
			rec("r3", map[string]any{"student_id": "B999"}),
			rec("r4", map[string]any{"student_id": ""}), // empty: never fires
			rec("r5", map[string]any{"student_id": "B999", "mismatch": true}),
		},
	}}
	ev := Trigger{AppID: "a", Kind: KindScheduled, At: readsAt, Location: jst}
	rows, err := PlanEach(context.Background(), a, rule(t, a, "roster_check"), ev, l)
	if err != nil {
		t.Fatal(err)
	}
	var marked []string
	for _, r := range rows {
		for _, w := range r.Plan.Writes {
			marked = append(marked, w.RecordID)
		}
	}
	if !slices.Equal(marked, []string{"r3"}) {
		t.Fatalf("marked %v (r5 is already marked and is not rewritten)", marked)
	}
	if l.lists["roster"] != 1 || l.lists["registrations"] != 1 {
		t.Fatalf("lists %v; each entity is read once per run", l.lists)
	}
}

// A record trigger with notExists / exists.
func TestPlanLookupRecordTrigger(t *testing.T) {
	a := readsApp(t, "server_rules_lookup.json")
	r := rule(t, a, "new_registration")
	l := &listReader{recs: map[string][]expreval.Record{"roster": roster()}}
	plan := func(id string) Plan {
		t.Helper()
		ev := Trigger{AppID: "a", Kind: KindCreated, Entity: "registrations", Record: rec("r1", map[string]any{"student_id": id}), At: readsAt, Location: jst}
		p, err := PlanRule(context.Background(), a, r, ev, l)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	if p := plan("Z9"); len(p.Notifies) != 1 || p.Notifies[0].Body != "Z9 は名簿にありません" {
		t.Fatalf("not on roster: %+v", p)
	}
	if p := plan("A001"); !p.Empty() {
		t.Fatalf("on roster: %+v", p)
	}
	r.Lookup.Op = dsl.LookupExists
	if p := plan("A001"); len(p.Notifies) != 1 {
		t.Fatalf("exists: %+v", p)
	}
	if p := plan(""); !p.Empty() {
		t.Fatalf("empty value fires exists: %+v", p)
	}
}

// "Not visited yet" (SP_2303 base): a ref compared with the row's id.
func TestPlanLookupByID(t *testing.T) {
	a := readsApp(t, "server_rules_lookup.json")
	l := &listReader{recs: map[string][]expreval.Record{
		"shops":  {rec("sh1", map[string]any{"name": "麺屋"}), rec("sh2", map[string]any{"name": "喫茶"})},
		"visits": {rec("v1", map[string]any{"shop": "sh1", "date": "2026-11-01"})},
	}}
	ev := Trigger{AppID: "a", Kind: KindScheduled, At: readsAt, Location: jst}
	rows, err := PlanEach(context.Background(), a, rule(t, a, "unvisited"), ev, l)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Trigger.Record.ID != "sh2" || rows[0].Plan.Notifies[0].Body != "喫茶 にまだ行っていません" {
		t.Fatalf("rows %+v", rows)
	}
}

// A lookup on the row's own entity never matches the row itself (duplicate
// detection).
func TestPlanLookupSkipsTheRow(t *testing.T) {
	a := readsApp(t, "server_rules_lookup.json")
	r := rule(t, a, "roster_check")
	r.Lookup = &dsl.RuleLookup{Op: dsl.LookupExists, Entity: "registrations", Field: "student_id", Equals: "student_id"}
	l := &listReader{recs: map[string][]expreval.Record{"registrations": {
		rec("r1", map[string]any{"student_id": "A1"}),
		rec("r2", map[string]any{"student_id": "A1"}),
		rec("r3", map[string]any{"student_id": "B2"}),
	}}}
	ev := Trigger{AppID: "a", Kind: KindScheduled, At: readsAt, Location: jst}
	rows, err := PlanEach(context.Background(), a, r, ev, l)
	if err != nil {
		t.Fatal(err)
	}
	var dup []string
	for _, row := range rows {
		dup = append(dup, row.Trigger.Record.ID)
	}
	if !slices.Equal(dup, []string{"r1", "r2"}) {
		t.Fatalf("duplicates %v", dup)
	}
}

// The lookup is read after if (no read when if is false) and a read cap
// stops the run.
func TestPlanLookupReads(t *testing.T) {
	a := readsApp(t, "server_rules_lookup.json")
	r := rule(t, a, "new_registration")
	r.If = &dsl.Expr{Kind: dsl.ExprLiteral, Lit: false}
	l := &listReader{recs: map[string][]expreval.Record{"roster": roster()}}
	ev := Trigger{AppID: "a", Kind: KindCreated, Entity: "registrations", Record: rec("r1", map[string]any{"student_id": "Z"}), At: readsAt, Location: jst}
	if p, err := PlanRule(context.Background(), a, r, ev, l); err != nil || !p.Empty() || l.lists["roster"] != 0 {
		t.Fatalf("if false: %+v %v lists %v", p, err, l.lists)
	}
	r.If = nil
	l.err = &ReadLimitError{Scope: "run_reads", Limit: 500, Used: 501}
	if _, err := PlanRule(context.Background(), a, r, ev, l); !errors.Is(err, ErrReadLimit) {
		t.Fatalf("err %v", err)
	}
	if _, err := PlanRule(context.Background(), a, r, ev, refOnly{}); !errors.Is(err, errNoLister) {
		t.Fatalf("no lister: %v", err)
	}
}

type refOnly struct{}

func (refOnly) Record(context.Context, string, string) (expreval.Record, bool, error) {
	return expreval.Record{}, false, nil
}
