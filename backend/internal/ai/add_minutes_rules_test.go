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

// The rules cover "N hours later" (×60 into addMinutes) and send day units
// to addDays, not addMinutes(-4320).
func TestAddMinutesRulesLaterAndDayUnits(t *testing.T) {
	for _, want := range []string{
		"○分後・○時間後",
		`{"op":"addMinutes","args":[{"ref":"taken_at"},240]}`,
		"日単位(「3日前」「1週間後」)は addMinutes でなく addDays",
		`{"op":"addDays","args":[{"ref":"due"},-3]}`,
	} {
		if !strings.Contains(addMinutesRules, want) {
			t.Errorf("addMinutes rules lack %q", want)
		}
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

// The datetime example (depart_at: datetime) is valid DSL and keeps the
// time; the same expression in a string / date field is rejected (it would
// show only the date).
func TestAddMinutesRulesDatetimeExample(t *testing.T) {
	if !strings.Contains(addMinutesRules, "日時を渡すなら field は datetime 型") || !strings.Contains(addMinutesRules, "時刻だけなら HH:mm 文字列") {
		t.Fatal("rules lack the datetime / HH:mm guidance")
	}
	doc := func(typ string) string {
		return `{"schemaVersion":1,"app":{"name":"出発日時","voiceEntry":"none"},"initialScreen":"home",
"entities":[{"id":"trip","name":"予定","titleField":"dest","fields":[
 {"id":"dest","name":"行き先","type":"string","required":true},
 {"id":"arrive_at","name":"着く日時","type":"datetime"},
 {"id":"travel_min","name":"移動時間","type":"integer","unit":"分"},
 {"id":"depart_at","name":"出発日時","type":"` + typ + `","computed":{"op":"addMinutes","args":[{"ref":"arrive_at"},{"op":"neg","args":[{"ref":"travel_min"}]}]}}]}],
"screens":[
 {"id":"home","title":"予定","components":[
  {"type":"list","entity":"trip","item":{"title":"dest","subtitle":"depart_at"},"onItemTap":{"type":"navigate","screen":"form"},"area":"A1:F10"},
  {"type":"button","label":"追加","action":{"type":"navigate","screen":"form"},"area":"A11:F12"}]},
 {"id":"form","title":"予定","entity":"trip","components":[
  {"type":"textField","field":"dest"},{"type":"datePicker","field":"arrive_at","includeTime":true},{"type":"numberField","field":"travel_min"}],
  "toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"trip"}}]}]}`
	}
	app, err := dsl.Parse([]byte(doc("datetime")))
	if err != nil {
		t.Fatalf("datetime example rejected: %v", err)
	}
	ev := expreval.New(expreval.Env{Entities: app.Entities, Location: time.UTC, Now: time.Now()})
	rec := expreval.Record{ID: "r", Data: map[string]any{"dest": "空港", "arrive_at": "2026-10-28T09:00:00Z", "travel_min": 45.0}}
	got := ev.Eval(*app.Entities[0].FieldByID("depart_at").Computed, &expreval.Row{Entity: &app.Entities[0], Record: rec})
	if want := time.Date(2026, 10, 28, 8, 15, 0, 0, time.UTC); got.Kind != expreval.Date || !got.Time.Equal(want) {
		t.Fatalf("depart_at = %+v, want %v", got, want)
	}
	for _, typ := range []string{"string", "date"} {
		if _, err := dsl.Parse([]byte(doc(typ))); err == nil || !strings.Contains(err.Error(), "make it type datetime") {
			t.Errorf("%s depart_at: want a datetime repair, got %v", typ, err)
		}
	}
}
