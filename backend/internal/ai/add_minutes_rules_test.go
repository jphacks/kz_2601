package ai

import (
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

func TestAddMinutesRulesInPrompt(t *testing.T) {
	if !strings.Contains(systemPrompt, addMinutesRules) {
		t.Fatal("addMinutes rules are not in the system prompt")
	}
	if !strings.Contains(string(GenerationSchema()), `"addMinutes"`) {
		t.Fatal("generation schema lacks addMinutes")
	}
}

// The prompt's example (departure = arrival − travel minutes) is valid DSL
// and computes the departure time.
func TestAddMinutesRulesExampleValid(t *testing.T) {
	raw := `{"schemaVersion":1,"app":{"name":"出発時刻","voiceEntry":"none"},"initialScreen":"home",
"entities":[{"id":"trip","name":"予定","titleField":"dest","fields":[
 {"id":"dest","name":"行き先","type":"string","required":true},
 {"id":"arrive","name":"着きたい時刻","type":"string"},
 {"id":"travel_min","name":"移動時間","type":"integer","unit":"分"},
 {"id":"depart","name":"出発時刻","type":"string","computed":{"op":"addMinutes","args":[{"ref":"arrive"},{"op":"neg","args":[{"op":"coalesce","args":[{"ref":"travel_min"},0]}]}]}}]}],
"screens":[
 {"id":"home","title":"予定","components":[
  {"type":"list","entity":"trip","item":{"title":"dest","subtitle":"depart"},"onItemTap":{"type":"navigate","screen":"form"},"area":"A1:F9"},
  {"type":"openUrlButton","label":"乗換案内を開く","url":"https://transit.yahoo.co.jp/","area":"A10:F10"},
  {"type":"button","label":"追加","action":{"type":"navigate","screen":"form"},"area":"A11:F12"}]},
 {"id":"form","title":"予定","entity":"trip","components":[
  {"type":"textField","field":"dest"},{"type":"timePicker","field":"arrive"},{"type":"numberField","field":"travel_min"}],
  "toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"trip"}}]}]}`
	app, err := dsl.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("example rejected: %v", err)
	}
	ev := expreval.New(expreval.Env{Entities: app.Entities, Location: time.UTC, Now: time.Now()})
	rec := expreval.Record{ID: "r", Data: map[string]any{"dest": "空港", "arrive": "00:20", "travel_min": 90.0}}
	if got := ev.Eval(*app.Entities[0].FieldByID("depart").Computed, &expreval.Row{Entity: &app.Entities[0], Record: rec}); got.Str != "22:50" {
		t.Fatalf("depart = %+v, want 22:50", got)
	}
}
