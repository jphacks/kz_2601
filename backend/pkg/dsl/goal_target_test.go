package dsl

import (
	"fmt"
	"strings"
	"testing"
)

// goalApp: study records, a goal entity (%s = its fields / single flag) and
// home components (%s).
const goalApp = `{"schemaVersion":1,"app":{"name":"勉強","voiceEntry":"floating"},"initialScreen":"home",
"entities":[
 {"id":"study","name":"勉強","fields":[{"id":"subject","name":"科目","type":"string","required":true},{"id":"minutes","name":"分","type":"integer"},{"id":"date","name":"日付","type":"date","defaultValue":"now"}]},
 {"id":"%s","name":"%s",%s"fields":[%s]}],
"screens":[
 {"id":"home","title":"ホーム","toolbar":[
   {"label":"追加","action":{"type":"navigate","screen":"form"}},
   {"label":"目標","action":{"type":"navigate","screen":"goal_form"}}],
  "components":[%s
   {"type":"list","entity":"study","item":{"title":"subject"},"onItemTap":{"type":"navigate","screen":"form"}}]},
 {"id":"form","title":"記録","entity":"study","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"study"}}],
  "components":[{"type":"textField","field":"subject"},{"type":"numberField","field":"minutes"}]},
 {"id":"goal_form","title":"目標","entity":"%s","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"%s"}}],
  "components":[{"type":"numberField","field":"%s"}]}]}`

func goalLint(t *testing.T, entity, name, single, field, fieldType, comps string) []string {
	t.Helper()
	fields := fmt.Sprintf(`{"id":%q,"name":"値","type":%q}`, field, fieldType)
	raw := fmt.Sprintf(goalApp, entity, name, single, fields, comps, entity, entity, field)
	var out []string
	for _, w := range lintApp(t, raw) {
		if strings.Contains(w, "while the goal is kept") {
			out = append(out, w)
		}
	}
	return out
}

const (
	barLit     = `{"type":"progressBar","label":"今日","entity":"study","field":"minutes","aggregate":"sum","target":120,"tone":"positive"},`
	barSetting = `{"type":"progressBar","label":"今日","entity":"study","field":"minutes","aggregate":"sum","target":{"setting":"target_minutes"}},`
	gaugeLit   = `{"type":"gauge","label":"今日","entity":"study","field":"minutes","aggregate":"sum","max":120},`
	rateStat   = `{"type":"stat","label":"達成率","format":"percent","value":{"op":"div","args":[{"agg":"sum","entity":"study","field":"minutes"},{"setting":"target_minutes"}]}},`
)

func TestLintGoalTarget(t *testing.T) {
	for name, tc := range map[string]struct {
		entity, ename, single, field, ftype, comps string
		want                                       string
	}{
		// nav_study: a goal entity with target_minutes, the bar says 120
		"single setting": {"goal", "目標", `"single":true,`, "target_minutes", "integer", rateStat + barLit, `{"setting":"target_minutes"}`},
		// logic_budget / logic_oshi: a goal entity named for it, read by nothing
		"named entity":  {"budget", "予算", "", "minutes", "integer", barLit, `make "budget" "single": true and set target to {"setting":"minutes"}`},
		"japanese name": {"plan", "目標", "", "minutes", "integer", barLit, `plan.minutes`},
		"gauge max":     {"goal", "目標", `"single":true,`, "target_minutes", "integer", gaugeLit, "gauge max is the literal 120"},
		// fine: the bar is the only goal (no goal entity) / the rate reads it
		"no goal entity": {"memo", "メモ", `"single":true,`, "note_count", "integer", barLit, ""},
		"rate stat only": {"goal", "目標", `"single":true,`, "target_minutes", "integer", rateStat, ""},
		"bar reads goal": {"goal", "目標", `"single":true,`, "target_minutes", "integer", barSetting, ""},
	} {
		t.Run(name, func(t *testing.T) {
			got := goalLint(t, tc.entity, tc.ename, tc.single, tc.field, tc.ftype, tc.comps)
			if tc.want == "" {
				if len(got) > 0 {
					t.Fatalf("unexpected: %v", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0], tc.want) {
				t.Fatalf("want one warning with %q, got %v", tc.want, got)
			}
		})
	}
}

// goalProbe: an app with the given entities and home components; returns the
// goal-target warnings.
func goalProbe(t *testing.T, entities, comps string) []string {
	t.Helper()
	raw := `{"schemaVersion":1,"app":{"name":"probe","voiceEntry":"floating"},"initialScreen":"home",
"entities":[` + entities + `],
"screens":[{"id":"home","title":"ホーム","components":[` + comps + `]}]}`
	var out []string
	for _, w := range lintApp(t, raw) {
		if strings.Contains(w, "while the goal is kept") {
			out = append(out, w)
		}
	}
	return out
}

// The bar's literal is about something other than the goal entity: a budget
// kept per category, a game's targets, or a goal for another log.
func TestLintGoalTargetUnrelated(t *testing.T) {
	const expense = `{"id":"expense","name":"支出","fields":[{"id":"category","name":"カテゴリ","type":"enum","options":["食費","日用品"],"required":true},{"id":"amount","name":"金額","type":"currency"},{"id":"date","name":"日付","type":"date","defaultValue":"now"}]}`
	const monthBar = `{"type":"progressBar","label":"今月の支出(上限10万)","entity":"expense","field":"amount","aggregate":"sum","target":100000,"tone":"negative"}`
	for name, tc := range map[string]struct{ entities, comps string }{
		// budgets per category (an enum key shared with the log), listed
		"budget per category enum": {
			`{"id":"budget","name":"予算","fields":[{"id":"category","name":"カテゴリ","type":"enum","options":["食費","日用品"],"required":true},{"id":"amount","name":"金額","type":"currency"}]},` + expense,
			`{"type":"list","entity":"budget","item":{"title":"category","trailing":"amount"}},` + monthBar,
		},
		// budgets per category through a ref to a category master
		"budget per category ref": {
			`{"id":"category","name":"カテゴリ","fields":[{"id":"name","name":"名前","type":"string","required":true}]},
			 {"id":"budget","name":"予算","fields":[{"id":"category","name":"カテゴリ","type":"ref","refEntity":"category","required":true},{"id":"amount","name":"金額","type":"currency"}]},
			 {"id":"expense","name":"支出","fields":[{"id":"category","name":"カテゴリ","type":"ref","refEntity":"category"},{"id":"amount","name":"金額","type":"currency"},{"id":"date","name":"日付","type":"date","defaultValue":"now"}]}`,
			`{"type":"list","entity":"budget","item":{"title":"category","trailing":"amount"}},` + monthBar,
		},
		// a shooting game's targets (的) with points, and a high-score bar
		"game targets": {
			`{"id":"target","name":"的","fields":[{"id":"label","name":"名前","type":"string","required":true},{"id":"points","name":"点","type":"integer"}]},
			 {"id":"shot","name":"記録","fields":[{"id":"score","name":"点数","type":"integer"},{"id":"date","name":"日付","type":"date","defaultValue":"now"}]}`,
			`{"type":"list","entity":"target","item":{"title":"label","trailing":"points"}},
			 {"type":"progressBar","label":"1000点への道","entity":"shot","field":"score","aggregate":"sum","target":1000,"tone":"positive"}`,
		},
		// the setting is the steps goal; the bar is a book's fixed page count
		"goal for another log": {
			`{"id":"settings","name":"設定","single":true,"fields":[{"id":"daily_steps_goal","name":"1日の歩数目標","type":"integer","defaultValue":8000}]},
			 {"id":"steps","name":"歩数","fields":[{"id":"count","name":"歩数","type":"integer"},{"id":"date","name":"日付","type":"date","defaultValue":"now"}]},
			 {"id":"reading","name":"読書","fields":[{"id":"pages","name":"ページ","type":"integer"},{"id":"date","name":"日付","type":"date","defaultValue":"now"}]}`,
			`{"type":"stat","label":"歩数の達成率","format":"percent","value":{"op":"div","args":[{"agg":"sum","entity":"steps","field":"count","filter":{"field":"date","op":"today","value":null}},{"setting":"daily_steps_goal"}]}},
			 {"type":"progressBar","label":"300ページの本","entity":"reading","field":"pages","aggregate":"sum","target":300,"tone":"positive"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := goalProbe(t, tc.entities, tc.comps); len(got) > 0 {
				t.Fatalf("unexpected: %v", got)
			}
		})
	}
}

// Still reported when the goal and the bar's log meet in one expression though
// their field names differ, and a bar picks the goal that is about its log.
func TestLintGoalTargetRelated(t *testing.T) {
	for name, tc := range map[string]struct{ entities, comps, want string }{
		"combined in a stat": {
			`{"id":"goal","name":"目標","fields":[{"id":"yen","name":"目標額","type":"currency"},{"id":"date","name":"日付","type":"date","defaultValue":"now"}]},
			 {"id":"saving","name":"貯金","fields":[{"id":"amount","name":"金額","type":"currency"},{"id":"date","name":"日付","type":"date","defaultValue":"now"}]}`,
			`{"type":"stat","label":"残り","value":{"op":"sub","args":[{"agg":"latest","entity":"goal","field":"yen","by":"date"},{"agg":"sum","entity":"saving","field":"amount"}]}},
			 {"type":"progressBar","label":"貯金","entity":"saving","field":"amount","aggregate":"sum","target":100000}`,
			"goal.yen",
		},
		"picks the related goal": {
			`{"id":"settings","name":"設定","single":true,"fields":[{"id":"daily_steps_goal","name":"1日の歩数目標","type":"integer","defaultValue":8000},{"id":"pages_goal","name":"ページ目標","type":"integer","defaultValue":30}]},
			 {"id":"steps","name":"歩数","fields":[{"id":"count","name":"歩数","type":"integer"},{"id":"date","name":"日付","type":"date","defaultValue":"now"}]},
			 {"id":"reading","name":"読書","fields":[{"id":"pages","name":"ページ","type":"integer"},{"id":"date","name":"日付","type":"date","defaultValue":"now"}]}`,
			`{"type":"stat","label":"歩数の達成率","format":"percent","value":{"op":"div","args":[{"agg":"sum","entity":"steps","field":"count"},{"setting":"daily_steps_goal"}]}},
			 {"type":"progressBar","label":"今日のページ","entity":"reading","field":"pages","aggregate":"sum","target":30}`,
			"settings.pages_goal",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := goalProbe(t, tc.entities, tc.comps)
			if len(got) != 1 || !strings.Contains(got[0], tc.want) {
				t.Fatalf("want one warning with %q, got %v", tc.want, got)
			}
		})
	}
}
