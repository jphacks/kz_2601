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
	barLit   = `{"type":"progressBar","label":"今日","entity":"study","field":"minutes","aggregate":"sum","target":120,"tone":"positive"},`
	gaugeLit = `{"type":"gauge","label":"今日","entity":"study","field":"minutes","aggregate":"sum","max":120},`
	rateStat = `{"type":"stat","label":"達成率","format":"percent","value":{"op":"div","args":[{"agg":"sum","entity":"study","field":"minutes"},{"setting":"target_minutes"}]}},`
)

func TestLintGoalTarget(t *testing.T) {
	for name, tc := range map[string]struct {
		entity, ename, single, field, ftype, comps string
		want                                       string
	}{
		// nav_study: a goal entity with target_minutes, the bar says 120
		"single setting": {"goal", "目標", `"single":true,`, "target_minutes", "integer", rateStat + barLit, `{"setting":"target_minutes"}`},
		// logic_budget / logic_oshi: a goal entity named for it, read by nothing
		"named entity":  {"budget", "予算", "", "amount", "currency", barLit, `{"agg":"latest","entity":"budget","field":"amount"`},
		"japanese name": {"plan", "目標", "", "minutes", "integer", barLit, `plan.minutes`},
		"gauge max":     {"goal", "目標", `"single":true,`, "target_minutes", "integer", gaugeLit, "gauge max is the literal 120"},
		// fine: the bar is the only goal (no goal entity) / the rate reads it
		"no goal entity": {"memo", "メモ", `"single":true,`, "note_count", "integer", barLit, ""},
		"rate stat only": {"goal", "目標", `"single":true,`, "target_minutes", "integer", rateStat, ""},
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
