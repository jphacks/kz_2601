package ai

import (
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

func TestAdaptiveAmountRules_InPrompt(t *testing.T) {
	for _, s := range []string{
		"## 前の期間の結果で次の目標を決める(先週の達成率で今週の量を調整)",
		adaptiveTotalExpr, adaptiveRateExpr, adaptiveTargetExpr,
		`{"field":"done","op":"eq","value":true}`,
		"行ごとの先週に使わない",
		"固定の値で足りるので、この計算を足さない",
	} {
		if !strings.Contains(systemPrompt, s) {
			t.Errorf("systemPrompt lacks %q", s)
		}
	}
}

// last_done = the total's filter plus done eq true, as the rule says.
var adaptiveDoneExpr = strings.TrimSuffix(adaptiveTotalExpr, "]}") + `,{"field":"done","op":"eq","value":true}]}`

// The smallest app that follows the rule. It must pass dsl.Parse with no lint.
var adaptiveRuleApp = `{"schemaVersion":1,"app":{"name":"t","voiceEntry":"floating"},"initialScreen":"home","entities":[
 {"id":"weeks","name":"週","titleField":"start","fields":[
  {"id":"start","name":"週の初め","type":"date","required":true},
  {"id":"base","name":"基準の量","type":"integer","unit":"分","defaultValue":300},
  {"id":"last_total","name":"先週の予定","type":"integer","computed":` + adaptiveTotalExpr + `},
  {"id":"last_done","name":"先週の完了","type":"integer","computed":` + adaptiveDoneExpr + `},
  {"id":"last_rate","name":"先週の達成率","type":"integer","unit":"%","computed":` + adaptiveRateExpr + `},
  {"id":"target","name":"今週の目標","type":"integer","unit":"分","computed":` + adaptiveTargetExpr + `}]},
 {"id":"logs","name":"予定","titleField":"task","fields":[
  {"id":"date","name":"日付","type":"date","defaultValue":"today"},{"id":"task","name":"内容","type":"string","required":true},
  {"id":"done","name":"完了","type":"boolean","defaultValue":false}]}],
"screens":[
 {"id":"home","title":"週","toolbar":[{"label":"週を足す","action":{"type":"navigate","screen":"week_form","presentation":"sheet"}},{"label":"予定を足す","action":{"type":"navigate","screen":"log_form","presentation":"sheet"}}],
  "components":[{"type":"table","entity":"weeks","columns":[{"field":"start"},{"field":"last_rate"},{"field":"target"}]},
   {"type":"list","entity":"logs","item":{"title":"task","subtitle":"date","toggle":"done"}}]},
 {"id":"week_form","title":"週","entity":"weeks","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"weeks"}}],
  "components":[{"type":"datePicker","field":"start"},{"type":"numberField","field":"base"}]},
 {"id":"log_form","title":"予定","entity":"logs","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"logs"}}],
  "components":[{"type":"datePicker","field":"date"},{"type":"textField","field":"task"},{"type":"toggle","field":"done","label":"完了"}]}]}`

func TestAdaptiveAmountRules_ExprsEvaluate(t *testing.T) {
	app, err := dsl.Parse([]byte(adaptiveRuleApp))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if l := app.Lint(); len(l) > 0 {
		t.Errorf("lint %v", l)
	}
	week := func(id, start string, base float64) expreval.Record {
		return expreval.Record{ID: id, Data: map[string]any{"start": start, "base": base}}
	}
	log := func(id, date string, done bool) expreval.Record {
		return expreval.Record{ID: id, Data: map[string]any{"date": date, "task": "x", "done": done}}
	}
	weeks := []expreval.Record{
		week("low", "2026-11-02", 300),  // 10-26..11-01: 1 of 4 done
		week("keep", "2026-11-09", 300), // 11-02..11-08: 4 of 5 done
		week("high", "2026-11-16", 300), // 11-09..11-15: 4 of 4 done
		week("none", "2026-11-23", 200), // 11-16..11-22: nothing
	}
	logs := []expreval.Record{
		log("a", "2026-10-25", true), // before the window
		log("b", "2026-10-26", true), log("c", "2026-10-28", false), log("d", "2026-10-30", false), log("e", "2026-11-01", false),
		log("f", "2026-11-02", true), log("g", "2026-11-03", true), log("h", "2026-11-05", true), log("i", "2026-11-07", false), log("j", "2026-11-08", true),
		log("k", "2026-11-09", true), log("l", "2026-11-10", true), log("m", "2026-11-12", true), log("n", "2026-11-15", true),
	}
	src := expreval.SourceFunc(func(e string) []expreval.Record {
		return map[string][]expreval.Record{"weeks": weeks, "logs": logs}[e]
	})
	ev := expreval.New(expreval.Env{Entities: app.Entities, Source: src, Now: time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)})
	ent := app.EntityByID("weeks")
	// last_total, last_done, last_rate, target
	want := map[string][4]float64{
		"low":  {4, 1, 25, 150},  // 300*25/80 = 93.75 -> floor 150
		"keep": {5, 4, 80, 300},  // 80% keeps the base
		"high": {4, 4, 100, 375}, // 300*100/80
		"none": {0, 0, 80, 200},  // no history: keep the base
	}
	for _, w := range weeks {
		row := &expreval.Row{Entity: ent, Record: w}
		for i, f := range []string{"last_total", "last_done", "last_rate", "target"} {
			if got := ev.FieldValue(f, row); got != expreval.NumberValue(want[w.ID][i]) {
				t.Errorf("%s %s = %+v, want %v", w.ID, f, got, want[w.ID][i])
			}
		}
	}
}
