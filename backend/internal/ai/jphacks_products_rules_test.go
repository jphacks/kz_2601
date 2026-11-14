package ai

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

func TestJPHACKSProductsRules_InPrompt(t *testing.T) {
	for _, s := range []string{
		`"unique":["slot"]`,
		"gridTable{entity: task, columnField: urgency, rowField: importance",
		`{"op":"div","args":[{"agg":"count","entity":"menu","filter":{"field":"done","op":"eq","value":true}}`,
		"randomPicker の filter done eq false",
		"boolean done(defaultValue false",
		"申し込みの一覧(list booking)",
		`{"op":"clamp","args":[{"op":"add","args":[50,{"agg":"sum","entity":"habit_log"`,
	} {
		if !strings.Contains(systemPrompt, s) {
			t.Errorf("systemPrompt lacks %q", s)
		}
	}
}

// The stat.value expressions exactly as the rule text writes them.
const (
	jpRateExpr = `{"op":"div","args":[{"agg":"count","entity":"menu","filter":{"field":"done","op":"eq","value":true}},{"agg":"count","entity":"menu"}]}`
	jpHPExpr   = `{"op":"clamp","args":[{"op":"add","args":[50,{"agg":"sum","entity":"habit_log","field":"points","filter":{"field":"date","op":"today","value":null}}]},0,100]}`
)

const jpHead = `{"schemaVersion":1,"app":{"name":"t","voiceEntry":"floating"},"initialScreen":"home",`

// The smallest app that follows each rule. They must pass dsl.Parse with no lint.
var jpRuleApps = map[string]string{
	"slot booking": jpHead + `"entities":[
 {"id":"slot","name":"枠","scope":"public","titleField":"start","fields":[{"id":"start","name":"開始","type":"datetime","required":true},{"id":"memo","name":"メモ","type":"string"}]},
 {"id":"booking","name":"申し込み","scope":"public","unique":["slot"],"fields":[{"id":"slot","name":"枠","type":"ref","refEntity":"slot","required":true},{"id":"note","name":"ひとこと","type":"string"}]}],
"screens":[
 {"id":"home","title":"枠","toolbar":[{"label":"枠を追加","action":{"type":"navigate","screen":"slot_form","presentation":"sheet"}}],
  "components":[{"type":"list","entity":"slot","item":{"title":"start","subtitle":"memo"},"onItemTap":{"type":"navigate","screen":"booking_form","presentation":"sheet","prefillRef":"slot"}},
   {"type":"list","entity":"booking","item":{"title":"slot","subtitle":"note"},"onItemTap":{"type":"navigate","screen":"booking_form"}}]},
 {"id":"slot_form","title":"枠","entity":"slot","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"slot"}}],
  "components":[{"type":"datePicker","field":"start"},{"type":"textField","field":"memo"}]},
 {"id":"booking_form","title":"申し込み","entity":"booking","toolbar":[{"label":"申し込む","action":{"type":"saveRecord","entity":"booking"}}],
  "components":[{"type":"picker","field":"slot"},{"type":"textField","field":"note"}]}]}`,

	"priority matrix": jpHead + `"entities":[
 {"id":"task","name":"タスク","titleField":"title","fields":[{"id":"title","name":"タスク","type":"string","required":true},
  {"id":"importance","name":"重要度","type":"enum","options":["重要","重要でない"]},{"id":"urgency","name":"緊急度","type":"enum","options":["急ぐ","急がない"]}]}],
"screens":[
 {"id":"home","title":"マトリクス","toolbar":[{"label":"追加","action":{"type":"navigate","screen":"task_form","presentation":"sheet"}}],
  "components":[{"type":"gridTable","entity":"task","columnField":"urgency","rowField":"importance","titleField":"title","onItemTap":{"type":"navigate","screen":"task_form"}}]},
 {"id":"task_form","title":"タスク","entity":"task","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"task"}}],
  "components":[{"type":"textField","field":"title"},{"type":"picker","field":"importance"},{"type":"picker","field":"urgency"}]}]}`,

	"completion rate": jpHead + `"entities":[
 {"id":"menu","name":"メニュー","titleField":"name","fields":[{"id":"name","name":"メニュー","type":"string","required":true},{"id":"done","name":"食べた","type":"boolean","defaultValue":false}]}],
"screens":[
 {"id":"home","title":"制覇","toolbar":[{"label":"追加","action":{"type":"navigate","screen":"menu_form","presentation":"sheet"}}],
  "components":[{"type":"stat","label":"制覇率","format":"percent","value":` + jpRateExpr + `},
   {"type":"randomPicker","entity":"menu","titleField":"name","label":"まだのから選ぶ","filter":{"field":"done","op":"eq","value":false}},
   {"type":"list","entity":"menu","item":{"title":"name","toggle":"done"}}]},
 {"id":"menu_form","title":"メニュー","entity":"menu","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"menu"}}],
  "components":[{"type":"textField","field":"name"},{"type":"toggle","field":"done","label":"食べた"}]}]}`,

	"hp": jpHead + `"entities":[
 {"id":"habit_log","name":"記録","titleField":"what","fields":[{"id":"what","name":"したこと","type":"string","required":true},
  {"id":"date","name":"日付","type":"date","defaultValue":"now"},{"id":"points","name":"ポイント","type":"integer"}]}],
"screens":[
 {"id":"home","title":"HP","toolbar":[{"label":"記録","action":{"type":"navigate","screen":"log_form","presentation":"sheet"}}],
  "components":[{"type":"stat","label":"HP","value":` + jpHPExpr + `},{"type":"list","entity":"habit_log","item":{"title":"what","subtitle":"points"}}]},
 {"id":"log_form","title":"記録","entity":"habit_log","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"habit_log"}}],
  "components":[{"type":"textField","field":"what"},{"type":"datePicker","field":"date"},{"type":"numberField","field":"points"}]}]}`,
}

func TestJPHACKSProductsRules_AppsValidate(t *testing.T) {
	for _, e := range []string{jpRateExpr, jpHPExpr} {
		if !strings.Contains(systemPrompt, e) {
			t.Errorf("systemPrompt lacks %s", e)
		}
	}
	for name, raw := range jpRuleApps {
		app, err := dsl.Parse([]byte(raw))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if l := app.Lint(); len(l) > 0 {
			t.Errorf("%s: lint %v", name, l)
		}
	}
}

func TestJPHACKSProductsRules_ExprsEvaluate(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	ev := func(name string, recs map[string][]expreval.Record) *expreval.Evaluator {
		app, err := dsl.Parse([]byte(jpRuleApps[name]))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		src := expreval.SourceFunc(func(id string) []expreval.Record { return recs[id] })
		return expreval.New(expreval.Env{Entities: app.Entities, Source: src, Now: now})
	}
	eval := func(name, raw string, recs map[string][]expreval.Record) any {
		var e dsl.Expr
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			t.Fatal(err)
		}
		v := ev(name, recs).Eval(e, nil)
		if v.IsNil() {
			return nil
		}
		if v.Kind != expreval.Number {
			t.Fatalf("%s: %s is %v, want a number", name, raw, v)
		}
		return v.Num
	}
	rec := func(id string, data map[string]any) expreval.Record { return expreval.Record{ID: id, Data: data} }

	{ // completion rate
		menu := map[string][]expreval.Record{"menu": {
			rec("1", map[string]any{"name": "a", "done": true}), rec("2", map[string]any{"name": "b", "done": false}),
			rec("3", map[string]any{"name": "c", "done": true}), rec("4", map[string]any{"name": "d", "done": false}),
		}}
		if got := eval("completion rate", jpRateExpr, menu); got != 0.5 { // percent shows 0.5 as 50%
			t.Errorf("rate = %v, want 0.5", got)
		}
		// no menu yet: div by zero is nil (blank), not NaN / Inf
		if got := eval("completion rate", jpRateExpr, nil); got != nil {
			t.Errorf("rate with no records = %v, want nil", got)
		}
		var ids []string
		for _, r := range ev("completion rate", menu).Matching("menu", []dsl.Filter{{Field: "done", Op: "eq", Value: false}}, nil) {
			ids = append(ids, r.ID)
		}
		if strings.Join(ids, ",") != "2,4" {
			t.Errorf("randomPicker filter done eq false keeps %v, want 2,4", ids)
		}
		// A record with no done is not "eq false" (Go and iOS alike), hence defaultValue false in the rule.
		unset := map[string][]expreval.Record{"menu": {rec("5", map[string]any{"name": "e"})}}
		if got := ev("completion rate", unset).Matching("menu", []dsl.Filter{{Field: "done", Op: "eq", Value: false}}, nil); len(got) != 0 {
			t.Errorf("a record without done matched eq false: %v", got)
		}
	}

	{ // hp
		log := func(id, date string, p float64) expreval.Record {
			return rec(id, map[string]any{"what": "x", "date": date, "points": p})
		}
		for _, c := range []struct {
			name string
			logs []expreval.Record
			want float64
		}{
			{"no records", nil, 50},
			{"today only", []expreval.Record{log("a", "2026-10-25", 10), log("b", "2026-10-25", -5), log("c", "2026-10-24", 100)}, 55},
			{"capped at 100", []expreval.Record{log("a", "2026-10-25", 80)}, 100},
			{"floored at 0", []expreval.Record{log("a", "2026-10-25", -70)}, 0},
		} {
			if got := eval("hp", jpHPExpr, map[string][]expreval.Record{"habit_log": c.logs}); got != c.want {
				t.Errorf("hp %s = %v, want %v", c.name, got, c.want)
			}
		}
	}
}
