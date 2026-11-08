package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/runner/engine"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// The assignment vocabulary reached the prompt, its lines were swapped once
// and assignment requests run the pass.
func TestServerRulesAssignPrompt(t *testing.T) {
	for _, l := range serverRulesAssignLines {
		if strings.Count(serverRulesPrompt, l[1]) != 1 {
			t.Fatalf("line not swapped: %s", l[0])
		}
	}
	if strings.Contains(serverRulesPrompt, "複数人の割り当て・日程の自動調整") {
		t.Fatal("the prompt still declines assignment")
	}
	if !strings.Contains(serverRulesPrompt, serverRulesAssignSection) {
		t.Fatal("assign section not in the prompt")
	}
	for _, s := range []string{"シフトを自動で割り当てて", "当番を交代で回して", "一番空いている日に入れて", "みんなが集まれる日を決めて"} {
		if !wantsServerRules(s) {
			t.Errorf("%q should run the pass", s)
		}
	}
}

const shiftsApp = `{"schemaVersion":1,"app":{"name":"シフト","voiceEntry":"none"},"initialScreen":"home",
"entities":[
 {"id":"staff","name":"スタッフ","scope":"shared","titleField":"name","fields":[{"id":"name","name":"名前","type":"string","required":true}]},
 {"id":"shifts","name":"シフト","scope":"shared","titleField":"day","fields":[{"id":"day","name":"日付","type":"date","required":true},{"id":"staff","name":"担当","type":"ref","refEntity":"staff"}]},
 {"id":"offs","name":"休み","scope":"shared","titleField":"day","fields":[{"id":"staff","name":"スタッフ","type":"ref","refEntity":"staff","required":true},{"id":"day","name":"日付","type":"date","required":true}]}],
"screens":[{"id":"home","title":"シフト","components":[{"type":"list","entity":"shifts","item":{"title":"day"}}]}]}`

const daysApp = `{"schemaVersion":1,"app":{"name":"作業日","voiceEntry":"none"},"initialScreen":"home",
"entities":[
 {"id":"days","name":"作業日","titleField":"date","fields":[{"id":"date","name":"日付","type":"date","required":true},{"id":"off","name":"作業できない","type":"boolean"}]},
 {"id":"tasks","name":"タスク","titleField":"title","fields":[{"id":"title","name":"名前","type":"string","required":true},{"id":"day","name":"作業日","type":"ref","refEntity":"days"}]}],
"screens":[{"id":"home","title":"タスク","components":[{"type":"list","entity":"tasks","item":{"title":"title"}}]}]}`

func assignPlan(t *testing.T, app, reply string, ev engine.Trigger, refs listMap) engine.Plan {
	t.Helper()
	var r serverRulesReply
	if err := json.Unmarshal([]byte(reply), &r); err != nil {
		t.Fatal(err)
	}
	rules, err := serverRulesFromWire(r.ServerRules)
	if err != nil {
		t.Fatal(err)
	}
	out, err := withServerRules([]byte(app), rules)
	if err != nil {
		t.Fatal(err)
	}
	a, err := dsl.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	p, err := engine.PlanRule(context.Background(), a, &a.ServerRules[0], ev, refs)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// The prompt's shift example in the wire form validates and assigns the
// staff member with the fewest shifts who is not off that day.
func TestServerRulesAssignShiftWire(t *testing.T) {
	reply := `{"unchanged":false,"declined":"","serverRules":[{"id":"assign","on":{"record":"created","entity":"shifts"},
"if":{"op":"eq","args":[{"op":"coalesce","args":[{"ref":"staff"},"-"]},"-"]},
"pick":{"entity":"staff","order":"fewest","count":{"entity":"shifts","field":"staff"},"exclude":{"entity":"offs","field":"staff","match":{"field":"day","equals":"day"}}},
"do":[{"updateRecord":{"values":[{"field":"staff","value":{"ref":"$pick"}}]}},
{"notify":{"to":{"kind":"members"},"title":"シフト","body":"{day} の担当は {$pick.name} さんです"}}]}]}`
	refs := listMap{
		"staff":  {{ID: "s1", Data: map[string]any{"name": "田中"}}, {ID: "s2", Data: map[string]any{"name": "鈴木"}}, {ID: "s3", Data: map[string]any{"name": "佐藤"}}},
		"shifts": {{ID: "x1", Data: map[string]any{"day": "2026-11-09", "staff": "s1"}}},
		"offs":   {{ID: "o1", Data: map[string]any{"staff": "s2", "day": "2026-11-10"}}},
	}
	ev := engine.Trigger{AppID: "a", Kind: engine.KindCreated, Scope: dsl.ScopeShared, Entity: "shifts",
		Record: expreval.Record{ID: "new", Data: map[string]any{"day": "2026-11-10"}}, RunID: "run-1"}
	p := assignPlan(t, shiftsApp, reply, ev, refs)
	if len(p.Writes) != 1 || p.Writes[0].Values["staff"] != "s3" || len(p.Notifies) != 1 || p.Notifies[0].Body != "2026-11-10 の担当は 佐藤 さんです" {
		t.Fatalf("plan %+v", p)
	}
}

// The work-day example: the free day (not off, not past) with the fewest
// tasks.
func TestServerRulesAssignDaysWire(t *testing.T) {
	reply := `{"unchanged":false,"declined":"","serverRules":[{"id":"place","on":{"record":"created","entity":"tasks"},
"pick":{"entity":"days","order":"fewest","filter":[{"field":"off","op":"neq","value":true},{"field":"date","op":"gte","value":{"op":"today","args":[]}}],"count":{"entity":"tasks","field":"day"}},
"do":[{"updateRecord":{"values":[{"field":"day","value":{"ref":"$pick"}}]}}]}]}`
	refs := listMap{
		"days": {
			{ID: "d0", Data: map[string]any{"date": "2026-11-07"}},              // past
			{ID: "d1", Data: map[string]any{"date": "2026-11-09"}},              // 1 task
			{ID: "d2", Data: map[string]any{"date": "2026-11-10", "off": true}}, // off
			{ID: "d3", Data: map[string]any{"date": "2026-11-11"}},              // 0 tasks
		},
		"tasks": {{ID: "t1", Data: map[string]any{"title": "a", "day": "d1"}}},
	}
	ev := engine.Trigger{AppID: "a", Kind: engine.KindCreated, Scope: dsl.ScopePrivate, Entity: "tasks",
		Record: expreval.Record{ID: "t2", Data: map[string]any{"title": "b"}}, RunID: "run-1",
		At: time.Date(2026, 11, 8, 3, 0, 0, 0, time.UTC)}
	p := assignPlan(t, daysApp, reply, ev, refs)
	if len(p.Writes) != 1 || p.Writes[0].Values["day"] != "d3" {
		t.Fatalf("plan %+v", p)
	}
}
