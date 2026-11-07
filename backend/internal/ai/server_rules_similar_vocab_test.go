package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/runner/engine"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// The similar vocabulary reached the prompt and every reads-section line was
// swapped (a renamed line would keep "each・agg・lookup を合わせて").
func TestServerRulesSimilarPrompt(t *testing.T) {
	for _, l := range serverRulesSimilarLines {
		if strings.Contains(serverRulesPrompt, l[0]) || !strings.Contains(serverRulesPrompt, l[1]) {
			t.Fatalf("line not swapped: %s", l[0])
		}
	}
	if !strings.Contains(serverRulesPrompt, serverRulesSimilarSection) {
		t.Fatal("similar section not in the prompt")
	}
	for _, s := range []string{"趣味が似ている人をおすすめして", "共通点の多い人を表示", "サークルが学生をスカウトできる"} {
		if !wantsServerRules(s) {
			t.Errorf("%q should run the pass", s)
		}
	}
}

const profilesApp = `{"schemaVersion":1,"app":{"name":"趣味の近い人","voiceEntry":"none"},"initialScreen":"home",
"entities":[
 {"id":"profiles","name":"プロフィール","scope":"shared","titleField":"name","fields":[
  {"id":"name","name":"名前","type":"string","required":true},
  {"id":"hobbies","name":"趣味","type":"string"},
  {"id":"level","name":"レベル","type":"enum","options":["beginner","expert"]},
  {"id":"best_match","name":"おすすめの人","type":"ref","refEntity":"profiles"},
  {"id":"match_score","name":"共通点","type":"integer"}]}],
"screens":[{"id":"home","title":"人","components":[{"type":"list","entity":"profiles","item":{"title":"name"}}]}]}`

// The prompt's recommendation example in the wire form validates and writes
// the best match.
func TestServerRulesSimilarWire(t *testing.T) {
	reply := `{"unchanged":false,"declined":"","serverRules":[{"id":"recommend","on":{"record":"updated","entity":"profiles","fields":["hobbies","level"]},
"similar":{"entity":"profiles","fields":["hobbies","level"],"min":2},
"do":[{"updateRecord":{"values":[{"field":"best_match","value":{"ref":"$similar"}},{"field":"match_score","value":{"ref":"$similarScore"}}]}}]}]}`
	var r serverRulesReply
	if err := json.Unmarshal([]byte(reply), &r); err != nil {
		t.Fatal(err)
	}
	rules, err := serverRulesFromWire(r.ServerRules)
	if err != nil {
		t.Fatal(err)
	}
	out, err := withServerRules([]byte(profilesApp), rules)
	if err != nil {
		t.Fatal(err)
	}
	a, err := dsl.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	refs := listMap{"profiles": {
		{ID: "p1", Author: "u1", Data: map[string]any{"hobbies": "映画、音楽", "level": "expert"}},
		{ID: "p2", Author: "u2", Data: map[string]any{"hobbies": "釣り", "level": "expert"}},
	}}
	prev := expreval.Record{ID: "me", Author: "u3", Data: map[string]any{}}
	ev := engine.Trigger{AppID: "a", Kind: engine.KindUpdated, Entity: "profiles", Prev: &prev,
		Record: expreval.Record{ID: "me", Author: "u3", Data: map[string]any{"hobbies": "音楽", "level": "expert"}}}
	p, err := engine.PlanRule(context.Background(), a, &a.ServerRules[0], ev, refs)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Writes) != 1 || p.Writes[0].Values["best_match"] != "p1" || p.Writes[0].Values["match_score"] != float64(2) {
		t.Fatalf("plan %+v", p)
	}
}
