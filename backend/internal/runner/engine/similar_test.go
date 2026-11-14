package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

func profiles() []expreval.Record {
	return []expreval.Record{
		rec("p1", map[string]any{"name": "青木", "hobbies": "映画、音楽", "level": "middle"}),
		rec("p2", map[string]any{"name": "井上", "hobbies": "映画, 音楽, 旅行", "level": "beginner"}),
		rec("p3", map[string]any{"name": "上田", "hobbies": "釣り", "level": "middle"}),
		rec("p4", map[string]any{"name": "江藤", "hobbies": "音楽", "level": "middle"}),
	}
}

func profileUpdate(data map[string]any) Trigger {
	r := rec("me", data)
	prev := rec("me", map[string]any{})
	return Trigger{AppID: "a", Kind: KindUpdated, Entity: "profiles", Record: r, Prev: &prev, At: readsAt, Location: jst}
}

// The most similar profile is written into the row and its author is told;
// the score counts items over all fields.
func TestPlanSimilarRecordTrigger(t *testing.T) {
	a := readsApp(t, "server_rules_similar.json")
	l := &listReader{recs: map[string][]expreval.Record{"profiles": profiles()}}
	ev := profileUpdate(map[string]any{"name": "私", "hobbies": "ＭＵＳＩＣ・映画、音楽", "level": "middle"})
	p, err := PlanRule(context.Background(), a, rule(t, a, "recommend"), ev, l)
	if err != nil {
		t.Fatal(err)
	}
	// p1: 映画 + 音楽 + middle = 3; p2: 2; p4: 2.
	if len(p.Writes) != 1 || p.Writes[0].RecordID != "me" {
		t.Fatalf("writes %+v", p.Writes)
	}
	v := p.Writes[0].Values
	if v["best_match"] != "p1" || v["best_name"] != "青木" || v["match_score"] != float64(3) {
		t.Fatalf("values %v", v)
	}
	if _, ok := v[dsl.SimilarRef]; ok {
		t.Fatalf("a result is written: %v", v)
	}
	n := p.Notifies
	if len(n) != 1 || n[0].RefEntity != "profiles" || n[0].RefID != "p1" || n[0].Body != "共通点が 3 個ある人が来ました" {
		t.Fatalf("notifies %+v", n)
	}
	if l.lists["profiles"] != 1 {
		t.Fatalf("lists %v", l.lists)
	}
}

// Below min nothing runs; an empty row never searches.
func TestPlanSimilarMin(t *testing.T) {
	a := readsApp(t, "server_rules_similar.json")
	r := rule(t, a, "recommend")
	l := &listReader{recs: map[string][]expreval.Record{"profiles": profiles()}}
	if p, err := PlanRule(context.Background(), a, r, profileUpdate(map[string]any{"hobbies": "釣り", "level": "expert"}), l); err != nil || !p.Empty() {
		t.Fatalf("1 shared < min 2: %+v %v", p, err)
	}
	r.Similar.Min = 1
	if p, err := PlanRule(context.Background(), a, r, profileUpdate(map[string]any{"hobbies": "釣り", "level": "expert"}), l); err != nil || len(p.Writes) != 1 || p.Writes[0].Values["best_match"] != "p3" {
		t.Fatalf("min 1: %+v %v", p, err)
	}
	l.lists = nil
	if p, err := PlanRule(context.Background(), a, r, profileUpdate(map[string]any{"name": "空"}), l); err != nil || !p.Empty() || l.lists["profiles"] != 0 {
		t.Fatalf("empty row: %+v %v lists %v", p, err, l.lists)
	}
}

// The row itself and the row author's other records are never the match;
// ties go to the first listed record.
func TestPlanSimilarSkipsSelf(t *testing.T) {
	a := readsApp(t, "server_rules_similar.json")
	recs := profiles()
	mine := rec("p0", map[string]any{"name": "私の別", "hobbies": "映画、音楽", "level": "middle"})
	mine.Author = "u-me"
	l := &listReader{recs: map[string][]expreval.Record{"profiles": append([]expreval.Record{mine, rec("me", map[string]any{"hobbies": "映画、音楽", "level": "middle"})}, recs...)}}
	p, err := PlanRule(context.Background(), a, rule(t, a, "recommend"), profileUpdate(map[string]any{"hobbies": "映画、音楽", "level": "middle"}), l)
	if err != nil || len(p.Writes) != 1 || p.Writes[0].Values["best_match"] != "p1" {
		t.Fatalf("%+v %v", p, err)
	}
	p, err = PlanRule(context.Background(), a, rule(t, a, "recommend"), profileUpdate(map[string]any{"hobbies": "音楽", "level": "beginner"}), l)
	if err != nil || len(p.Writes) != 1 || p.Writes[0].Values["best_match"] != "p2" {
		t.Fatalf("p2 has 音楽 + beginner: %+v %v", p, err)
	}
	// tie: p2 (旅行) and p3 (釣り) share 1 each; p2 is listed first.
	ev := Trigger{AppID: "a", Kind: KindCreated, Entity: "circles", Record: rec("c1", map[string]any{"name": "外遊び", "hobbies": "釣り、旅行"}), At: readsAt, Location: jst}
	p, err = PlanRule(context.Background(), a, rule(t, a, "scout"), ev, l)
	if err != nil || len(p.Notifies) != 1 || p.Notifies[0].RefID != "p2" {
		t.Fatalf("tie: %+v %v", p, err)
	}
}

// on.each: every profile gets its best match; one list serves all rows and an
// unchanged row is not rewritten.
func TestPlanSimilarEach(t *testing.T) {
	a := readsApp(t, "server_rules_similar.json")
	recs := profiles()
	recs[0].Data["best_match"], recs[0].Data["best_name"] = "p2", "井上"
	l := &listReader{recs: map[string][]expreval.Record{"profiles": recs}}
	ev := Trigger{AppID: "a", Kind: KindScheduled, At: readsAt, Location: jst}
	rows, err := PlanEach(context.Background(), a, rule(t, a, "daily_match"), ev, l)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]any{}
	for _, r := range rows {
		for _, w := range r.Plan.Writes {
			got[w.RecordID] = w.Values["best_match"]
		}
	}
	// p1 already has p2 (not rewritten); p2 -> p1; p3 (釣り) has none; p4 -> p1.
	if len(got) != 2 || got["p2"] != "p1" || got["p4"] != "p1" {
		t.Fatalf("writes %v", got)
	}
	if l.lists["profiles"] != 1 {
		t.Fatalf("lists %v", l.lists)
	}
}

// Another entity (a circle scouting people by hobbies) and the read cap.
func TestPlanSimilarOtherEntity(t *testing.T) {
	a := readsApp(t, "server_rules_similar.json")
	r := rule(t, a, "scout")
	l := &listReader{recs: map[string][]expreval.Record{"profiles": profiles()}}
	ev := Trigger{AppID: "a", Kind: KindCreated, Entity: "circles", Record: rec("c1", map[string]any{"name": "旅研", "hobbies": "旅行\n映画"}), At: readsAt, Location: jst}
	p, err := PlanRule(context.Background(), a, r, ev, l)
	if err != nil || len(p.Notifies) != 1 || p.Notifies[0].RefID != "p2" || p.Notifies[0].Body != "旅研 があなたの趣味と 2 個合います" {
		t.Fatalf("%+v %v", p, err)
	}
	l.err = &ReadLimitError{Scope: "run_reads", Limit: 500, Used: 501}
	if _, err := PlanRule(context.Background(), a, r, ev, l); !errors.Is(err, ErrReadLimit) {
		t.Fatalf("err %v", err)
	}
	if _, err := PlanRule(context.Background(), a, r, ev, refOnly{}); !errors.Is(err, errNoLister) {
		t.Fatalf("no lister: %v", err)
	}
}
