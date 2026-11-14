package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/runner/engine"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// The pick vocabulary reached the prompt and every line was swapped (a
// renamed line would keep "each・agg・lookup・similar を合わせて").
func TestServerRulesPickPrompt(t *testing.T) {
	for _, l := range serverRulesPickLines {
		if strings.Contains(serverRulesPrompt, l[0]) || !strings.Contains(serverRulesPrompt, l[1]) {
			t.Fatalf("line not swapped: %s", l[0])
		}
	}
	if !strings.Contains(serverRulesPrompt, serverRulesPickSection) {
		t.Fatal("pick section not in the prompt")
	}
	for _, s := range []string{"まだ食べていないメニューから選んで", "日替わりで1つ出して", "今日の献立を決めて"} {
		if !wantsServerRules(s) {
			t.Errorf("%q should run the pass", s)
		}
	}
}

const menusApp = `{"schemaVersion":1,"app":{"name":"メニュー制覇","voiceEntry":"none"},"initialScreen":"home",
"entities":[
 {"id":"menus","name":"メニュー","scope":"shared","titleField":"name","fields":[{"id":"name","name":"名前","type":"string","required":true}]},
 {"id":"eats","name":"食べた記録","scope":"shared","titleField":"menu","fields":[{"id":"menu","name":"メニュー","type":"ref","refEntity":"menus","required":true}]},
 {"id":"picks","name":"今日のおすすめ","scope":"shared","titleField":"name","fields":[
  {"id":"menu","name":"メニュー","type":"ref","refEntity":"menus"},{"id":"name","name":"名前","type":"string"},{"id":"day","name":"日付","type":"date"}]}],
"screens":[{"id":"home","title":"おすすめ","components":[{"type":"list","entity":"picks","item":{"title":"name"}}]}]}`

// The prompt's daily recommendation in the wire form validates and logs a
// dish not eaten yet.
func TestServerRulesPickWire(t *testing.T) {
	reply := `{"unchanged":false,"declined":"","serverRules":[{"id":"today","on":{"every":"daily","time":"08:00"},
"pick":{"entity":"menus","order":"random","exclude":{"entity":"eats","field":"menu"}},
"do":[{"createRecord":{"entity":"picks","values":[{"field":"menu","value":{"ref":"$pick"}},{"field":"name","value":{"ref":"$pick.name"}},{"field":"day","value":{"op":"today","args":[]}}]}},
{"notify":{"to":{"kind":"members"},"title":"今日のおすすめ","body":"今日は {$pick.name} はどうですか"}}]}]}`
	var r serverRulesReply
	if err := json.Unmarshal([]byte(reply), &r); err != nil {
		t.Fatal(err)
	}
	rules, err := serverRulesFromWire(r.ServerRules)
	if err != nil {
		t.Fatal(err)
	}
	out, err := withServerRules([]byte(menusApp), rules)
	if err != nil {
		t.Fatal(err)
	}
	a, err := dsl.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	refs := listMap{
		"menus": {{ID: "m1", Data: map[string]any{"name": "カレー"}}, {ID: "m2", Data: map[string]any{"name": "ラーメン"}}},
		"eats":  {{ID: "e1", Data: map[string]any{"menu": "m2"}}},
	}
	ev := engine.Trigger{AppID: "a", Kind: engine.KindScheduled, RunID: "run-1"}
	p, err := engine.PlanRule(context.Background(), a, &a.ServerRules[0], ev, refs)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Writes) != 1 || p.Writes[0].Values["menu"] != "m1" || len(p.Notifies) != 1 || p.Notifies[0].Body != "今日は カレー はどうですか" {
		t.Fatalf("plan %+v", p)
	}
}
