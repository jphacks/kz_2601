package dsl

import (
	"fmt"
	"strings"
	"testing"
)

// progressTargetApp: walk records, a single settings entity (%s = its field
// type and extras) and a home bar with target %s.
const progressTargetApp = `{"schemaVersion":1,"app":{"name":"歩数","voiceEntry":"floating"},"initialScreen":"home",
"entities":[
 {"id":"walk","name":"歩いた記録","fields":[{"id":"steps","name":"歩数","type":"integer","required":true}]},
 {"id":"settings","name":"設定","single":true,"fields":[{"id":"daily_goal","name":"目標","type":%s},{"id":"memo","name":"メモ","type":"string"}]}],
"screens":[
 {"id":"home","title":"ホーム","toolbar":[{"label":"追加","action":{"type":"navigate","screen":"form"}},{"label":"目標","action":{"type":"navigate","screen":"goal"}}],
  "components":[
   {"type":"progressBar","label":"今日","entity":"walk","field":"steps","target":%s,"tone":"positive"},
   {"type":"list","entity":"walk","item":{"title":"steps"},"onItemTap":{"type":"navigate","screen":"form"}}]},
 {"id":"form","title":"記録","entity":"walk","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"walk"}}],"components":[{"type":"numberField","field":"steps"}]},
 {"id":"goal","title":"目標","entity":"settings","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"settings"}}],"components":[{"type":"numberField","field":"daily_goal"}]}]}`

func TestProgressBarSettingTarget(t *testing.T) {
	for name, tc := range map[string]struct {
		field, target, err string
	}{
		"setting":       {`"integer","defaultValue":8000`, `{"setting":"daily_goal"}`, ""},
		"number":        {`"integer"`, `8000`, ""},
		"zero":          {`"integer"`, `0`, "needs target > 0"},
		"unknown field": {`"integer"`, `{"setting":"nope"}`, "is not a field of a single entity"},
		"not numeric":   {`"integer"`, `{"setting":"memo"}`, "must be an integer, number or currency field"},
		"other expr":    {`"integer"`, `{"agg":"count","entity":"walk"}`, "missing property 'setting'"}, // the schema rejects it first
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(fmt.Sprintf(progressTargetApp, tc.field, tc.target)))
			switch {
			case tc.err == "" && err != nil:
				t.Fatalf("unexpected: %v", err)
			case tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)):
				t.Fatalf("want %q, got %v", tc.err, err)
			}
		})
	}
}

// A setting target without a default > 0 shows "—" until the goal is saved.
func TestLintProgressTargetDefault(t *testing.T) {
	has := func(ws []string) bool {
		for _, w := range ws {
			if strings.Contains(w, "has no defaultValue > 0") {
				return true
			}
		}
		return false
	}
	if !has(lintApp(t, fmt.Sprintf(progressTargetApp, `"integer"`, `{"setting":"daily_goal"}`))) {
		t.Error("no default: want the lint")
	}
	if !has(lintApp(t, fmt.Sprintf(progressTargetApp, `"integer","defaultValue":0`, `{"setting":"daily_goal"}`))) {
		t.Error("default 0: want the lint")
	}
	if ws := lintApp(t, fmt.Sprintf(progressTargetApp, `"integer","defaultValue":8000`, `{"setting":"daily_goal"}`)); has(ws) {
		t.Errorf("default 8000: unexpected %v", ws)
	}
}
