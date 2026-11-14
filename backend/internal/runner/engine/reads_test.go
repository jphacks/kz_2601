package engine

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

func readsApp(t *testing.T, name string) *dsl.App {
	t.Helper()
	raw, err := os.ReadFile("../../../pkg/dsl/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return parseApp(t, raw)
}

// listReader lists fixed records and counts the lists.
type listReader struct {
	recs  map[string][]expreval.Record
	lists map[string]int
	err   error
}

func (l *listReader) Record(_ context.Context, entity, id string) (expreval.Record, bool, error) {
	for _, r := range l.recs[entity] {
		if r.ID == id {
			return r, true, nil
		}
	}
	return expreval.Record{}, false, nil
}

func (l *listReader) List(_ context.Context, entity string) ([]expreval.Record, error) {
	if l.err != nil {
		return nil, l.err
	}
	if l.lists == nil {
		l.lists = map[string]int{}
	}
	l.lists[entity]++
	return l.recs[entity], nil
}

func rec(id string, data map[string]any) expreval.Record {
	return expreval.Record{ID: id, Data: data, Author: "u-" + id, AuthorName: "人" + id}
}

// at: Sunday 2026-11-08 20:00 JST.
var readsAt = time.Date(2026, 11, 8, 11, 0, 0, 0, time.UTC)

var jst = time.FixedZone("JST", 9*3600)

func expenses() []expreval.Record {
	return []expreval.Record{
		rec("e1", map[string]any{"date": "2026-11-02", "amount": float64(30000)}), // this week (Mon)
		rec("e2", map[string]any{"date": "2026-11-08", "amount": float64(25000)}), // this week (Sun)
		rec("e3", map[string]any{"date": "2026-10-31", "amount": float64(9000)}),  // last month
	}
}

// A condition on a monthly total (agg) and the total inserted with with.
func TestPlanAggCondition(t *testing.T) {
	a := readsApp(t, "server_rules_reads.json")
	r := rule(t, a, "over_budget")
	l := &listReader{recs: map[string][]expreval.Record{"expenses": expenses()}}
	ev := Trigger{AppID: "a", Kind: KindCreated, Entity: "expenses", Record: rec("e2", map[string]any{}), At: readsAt, Location: jst}
	p, err := PlanRule(context.Background(), a, r, ev, l)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Notifies) != 1 || p.Notifies[0].Body != "今月の支出が 55000 円になりました" {
		t.Fatalf("plan %+v", p)
	}
	if l.lists["expenses"] != 1 {
		t.Fatalf("listed %v times; the run reads each entity once", l.lists)
	}
	// under the budget: nothing
	l.recs["expenses"] = expenses()[:1]
	if p, err := PlanRule(context.Background(), a, r, ev, l); err != nil || !p.Empty() {
		t.Fatalf("under budget: %+v %v", p, err)
	}
}

// A weekly summary written to a record and sent to the owner.
func TestPlanWeeklySummary(t *testing.T) {
	a := readsApp(t, "server_rules_reads.json")
	l := &listReader{recs: map[string][]expreval.Record{"expenses": expenses()}}
	ev := Trigger{AppID: "a", Kind: KindScheduled, At: readsAt, Location: jst}
	p, err := PlanRule(context.Background(), a, rule(t, a, "weekly_summary"), ev, l)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Writes) != 1 || p.Writes[0].Values["total"] != float64(55000) || p.Writes[0].Values["count"] != float64(2) || p.Writes[0].Values["week"] != "2026-11-08" {
		t.Fatalf("writes %+v", p.Writes)
	}
	if len(p.Notifies) != 1 || p.Notifies[0].Body != "今週は 2 件・55000 円でした" {
		t.Fatalf("notifies %+v", p.Notifies)
	}
}

// on.each: one plan per record whose condition holds (due in 3 days).
func TestPlanEachDueSoon(t *testing.T) {
	a := readsApp(t, "server_rules_reads.json")
	l := &listReader{recs: map[string][]expreval.Record{"tasks": {
		rec("t1", map[string]any{"title": "レポート", "due": "2026-11-11"}),
		rec("t2", map[string]any{"title": "済んだ", "due": "2026-11-11", "done": true}),
		rec("t3", map[string]any{"title": "まだ先", "due": "2026-11-20"}),
		rec("t4", map[string]any{"title": "日付なし"}),
	}}}
	ev := Trigger{AppID: "a", Kind: KindScheduled, At: readsAt, Location: jst}
	r := rule(t, a, "due_soon")
	rows, err := PlanEach(context.Background(), a, r, ev, l)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Trigger.Record.ID != "t1" || rows[0].Trigger.Entity != "tasks" ||
		rows[0].Plan.Notifies[0].Body != "レポート の締め切りまであと 3 日です" {
		t.Fatalf("rows %+v", rows)
	}
	// PlanRule alone refuses an each rule without its record
	if _, err := PlanRule(context.Background(), a, r, ev, l); err == nil {
		t.Fatal("PlanRule ran an each rule without a record")
	}
	// a reader that cannot list
	if _, err := PlanEach(context.Background(), a, r, ev, &mapRefs{}); !errors.Is(err, errNoLister) {
		t.Fatalf("no lister: %v", err)
	}
}

// Ranking: each player gets its rank; unchanged ranks are not written.
func TestPlanEachRanking(t *testing.T) {
	a := readsApp(t, "server_rules_ranking.json")
	l := &listReader{recs: map[string][]expreval.Record{"players": {
		rec("p1", map[string]any{"name": "A", "score": float64(100), "rank": int64(2)}),
		rec("p2", map[string]any{"name": "B", "score": float64(300), "rank": int64(1)}),
		rec("p3", map[string]any{"name": "C", "score": float64(200), "rank": int64(3)}),
	}}}
	ev := Trigger{AppID: "a", Scope: dsl.ScopeShared, Kind: KindScheduled, At: readsAt, Location: jst}
	rows, err := PlanEach(context.Background(), a, rule(t, a, "ranking"), ev, l)
	if err != nil {
		t.Fatal(err)
	}
	ranks := map[string]any{}
	writes := 0
	for _, r := range rows {
		for _, w := range r.Plan.Writes {
			writes++
			ranks[w.RecordID] = w.Values["rank"]
		}
		if len(r.Plan.Notifies) != 1 {
			t.Fatalf("row %s: %+v", r.Trigger.Record.ID, r.Plan)
		}
	}
	if writes != 2 || ranks["p1"] != float64(3) || ranks["p3"] != float64(2) {
		t.Fatalf("ranks %v (p2 is already 1 and is not written)", ranks)
	}
	if l.lists["players"] != 1 {
		t.Fatalf("players listed %d times", l.lists["players"])
	}
	for _, r := range rows {
		if r.Trigger.Record.ID == "p1" && r.Plan.Notifies[0].Body != "Aさんは 3 位です" {
			t.Fatalf("body %q", r.Plan.Notifies[0].Body)
		}
	}
}

// A read cap error stops the plan (never computed on part of the records).
func TestPlanReadLimit(t *testing.T) {
	a := readsApp(t, "server_rules_reads.json")
	l := &listReader{err: &ReadLimitError{Scope: "run_reads", Limit: 500, Used: 501}}
	ev := Trigger{AppID: "a", Kind: KindScheduled, At: readsAt, Location: jst}
	_, err := PlanRule(context.Background(), a, rule(t, a, "weekly_summary"), ev, l)
	var rl *ReadLimitError
	if !errors.Is(err, ErrReadLimit) || !errors.As(err, &rl) || rl.Scope != "run_reads" {
		t.Fatalf("err %v", err)
	}
	if _, err := PlanEach(context.Background(), a, rule(t, a, "due_soon"), ev, l); !errors.Is(err, ErrReadLimit) {
		t.Fatalf("each err %v", err)
	}
}

// A with number from users' values is masked like an inserted field.
func TestRenderWithMasksLongNumbers(t *testing.T) {
	a := readsApp(t, "server_rules_reads.json")
	big := []expreval.Record{rec("e1", map[string]any{"date": "2026-11-02", "amount": float64(9012345678)})}
	l := &listReader{recs: map[string][]expreval.Record{"expenses": big}}
	ev := Trigger{AppID: "a", Kind: KindScheduled, At: readsAt, Location: jst}
	p, err := PlanRule(context.Background(), a, rule(t, a, "weekly_summary"), ev, l)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Notifies[0].Body; got != "今週は 1 件・"+Mask+" 円でした" {
		t.Fatalf("body %q", got)
	}
}
