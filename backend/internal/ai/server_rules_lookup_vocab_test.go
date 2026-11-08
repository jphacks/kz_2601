package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/runner/engine"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// The lookup vocabulary reached the prompt and every reads-section line was
// swapped (a renamed line would keep "each と agg を合わせて 2 つまで").
func TestServerRulesLookupPrompt(t *testing.T) {
	for _, l := range serverRulesLookupLines {
		want := l[1]
		for _, later := range serverRulesSimilarLines { // swapped again by server_rules_similar_vocab.go
			if later[0] == want {
				want = later[1]
			}
		}
		for _, later := range serverRulesPickLines { // and by server_rules_vocab_pick.go
			want = strings.Replace(want, later[0], later[1], 1)
		}
		if strings.Contains(serverRulesPrompt, l[0]) || !strings.Contains(serverRulesPrompt, want) {
			t.Fatalf("line not swapped: %s", l[0])
		}
	}
	if !strings.Contains(serverRulesPrompt, serverRulesLookupSection) {
		t.Fatal("lookup section not in the prompt")
	}
	for _, s := range []string{"名簿の CSV と履修登録を突合して", "名簿に載っていない人", "まだ行っていない店を教えて"} {
		if !wantsServerRules(s) {
			t.Errorf("%q should run the pass", s)
		}
	}
}

const rosterApp = `{"schemaVersion":1,"app":{"name":"履修チェック","voiceEntry":"none"},"initialScreen":"home",
"entities":[
 {"id":"roster","name":"名簿","fields":[{"id":"student_id","name":"学籍番号","type":"string","required":true}]},
 {"id":"registrations","name":"履修登録","titleField":"student_id","fields":[
  {"id":"student_id","name":"学籍番号","type":"string","required":true},
  {"id":"mismatch","name":"名簿に無い","type":"boolean"}]}],
"screens":[{"id":"home","title":"履修","components":[{"type":"list","entity":"registrations","item":{"title":"student_id"}}]}]}`

// The prompt's roster example in the wire form validates and marks only the
// registration missing from the roster.
func TestServerRulesLookupWire(t *testing.T) {
	reply := `{"unchanged":false,"declined":"","serverRules":[{"id":"roster_check","on":{"every":"daily","time":"07:00","each":"registrations"},
"lookup":{"op":"notExists","entity":"roster","field":"student_id","equals":"student_id"},
"do":[{"updateRecord":{"values":[{"field":"mismatch","value":true}]}}]}]}`
	var r serverRulesReply
	if err := json.Unmarshal([]byte(reply), &r); err != nil {
		t.Fatal(err)
	}
	rules, err := serverRulesFromWire(r.ServerRules)
	if err != nil {
		t.Fatal(err)
	}
	out, err := withServerRules([]byte(rosterApp), rules)
	if err != nil {
		t.Fatal(err)
	}
	a, err := dsl.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	refs := listMap{
		"roster":        {{ID: "s1", Data: map[string]any{"student_id": "A001"}}},
		"registrations": {{ID: "r1", Data: map[string]any{"student_id": "A001"}}, {ID: "r2", Data: map[string]any{"student_id": "B2"}}},
	}
	rows, err := engine.PlanEach(context.Background(), a, &a.ServerRules[0], engine.Trigger{AppID: "a", Kind: engine.KindScheduled}, refs)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Trigger.Record.ID != "r2" {
		t.Fatalf("rows %+v", rows)
	}
}
