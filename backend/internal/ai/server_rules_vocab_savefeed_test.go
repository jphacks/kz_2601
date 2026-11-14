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

// The saveFeed vocabulary reached the prompts and every line was swapped.
func TestServerRulesSaveFeedPrompt(t *testing.T) {
	for _, l := range serverRulesSaveFeedLines {
		if strings.Contains(serverRulesPrompt, l[0]) || !strings.Contains(serverRulesPrompt, l[1]) {
			t.Fatalf("line not swapped: %s", l[0])
		}
	}
	if !strings.Contains(serverRulesPrompt, serverRulesSaveFeedSection) {
		t.Fatal("saveFeed section not in the prompt")
	}
	if !strings.Contains(systemPrompt, weatherLogRules) || strings.Contains(weatherLogRules, "」は feed") {
		t.Fatal("the weather log line is not in the main prompt (or drops with feed)")
	}
	for _, s := range []string{"気圧を毎日残して頭痛と比べたい", "毎朝天気を記録して"} {
		if !wantsServerRules(s) {
			t.Errorf("%q should run the pass", s)
		}
	}
	if !strings.Contains(string(ServerRulesGenerationSchema()), `"saveFeed"`) {
		t.Fatal("server-rules schema lacks saveFeed")
	}
	if strings.Contains(string(GenerationSchema()), "saveFeed") {
		t.Fatal("generation schema mentions saveFeed")
	}
	t.Logf("server-rules schema %d bytes, generation schema %d bytes", len(ServerRulesGenerationSchema()), len(GenerationSchema()))
}

const pressureLogApp = `{"schemaVersion":1,"app":{"name":"頭痛と気圧","voiceEntry":"none"},"initialScreen":"home",
"entities":[
 {"id":"pressure_log","name":"気圧の記録","titleField":"note","fields":[
  {"id":"day","name":"日付","type":"date","required":true},{"id":"note","name":"気圧","type":"string"},
  {"id":"low","name":"最低","type":"number"},{"id":"high","name":"最高","type":"number"},{"id":"falling","name":"急な低下","type":"boolean"}]}],
"screens":[{"id":"home","title":"気圧","components":[{"type":"list","entity":"pressure_log","item":{"title":"note"}}]}]}`

type fakeFeed struct{}

func (fakeFeed) Record(context.Context, string, string) (expreval.Record, bool, error) {
	return expreval.Record{}, false, nil
}

func (fakeFeed) Feed(_ context.Context, provider, region string) (engine.FeedSnapshot, error) {
	lo, hi, drop := 1003.0, 1012.5, true
	return engine.FeedSnapshot{Summary: provider + region, Min: &lo, Max: &hi, Drop: &drop}, nil
}

// The prompt's example in the wire form validates and plans one record.
func TestServerRulesSaveFeedWire(t *testing.T) {
	reply := `{"unchanged":false,"declined":"","serverRules":[{"id":"log_pressure","on":{"every":"daily","time":"07:00"},
"do":[{"saveFeed":{"provider":"pressure","region":"130000","entity":"pressure_log","fields":{"date":"day","summary":"note","min":"low","max":"high","drop":"falling"}}}]}]}`
	var r serverRulesReply
	if err := json.Unmarshal([]byte(reply), &r); err != nil {
		t.Fatal(err)
	}
	rules, err := serverRulesFromWire(r.ServerRules)
	if err != nil {
		t.Fatal(err)
	}
	out, err := withServerRules([]byte(pressureLogApp), rules)
	if err != nil {
		t.Fatal(err)
	}
	a, err := dsl.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	p, err := engine.PlanRule(context.Background(), a, &a.ServerRules[0], engine.Trigger{AppID: "a", Kind: engine.KindScheduled}, fakeFeed{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Writes) != 1 || p.Writes[0].Values["note"] != "pressure130000" || p.Writes[0].Values["falling"] != true {
		t.Fatalf("plan %+v", p)
	}
}
