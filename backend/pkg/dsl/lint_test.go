package dsl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The older 今年 recipe (computed year + filter eq year(today)) must still
// validate and be lint-clean (the prompt now uses filter thisYear).
func TestLintThisYearRecipe(t *testing.T) {
	got := lintWith(t, `,{"id":"year","name":"年","type":"integer","computed":{"op":"year","args":[{"ref":"date"}]}}`,
		`{"type":"stat","label":"今年の合計","aggregate":"sum","entity":"log","field":"amount","filter":{"field":"year","op":"eq","value":{"op":"year","args":[{"op":"today","args":[]}]}}},`)
	if len(got) > 0 {
		t.Fatalf("unexpected: %v", got)
	}
}

// lintApp parses a document (must pass Validate) and returns its lint warnings.
func lintApp(t *testing.T, raw string) []string {
	t.Helper()
	app, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return app.Lint()
}

func TestLintExamplesClean(t *testing.T) {
	for _, name := range []string{"kakeibo", "warikan", "bmi", "training", "walk", "diary_ai"} {
		raw, err := os.ReadFile(filepath.Join(docsDir, "examples", name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		app, err := Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if w := app.Lint(); len(w) > 0 {
			t.Errorf("%s: unexpected lint warnings:\n%s", name, strings.Join(w, "\n"))
		}
	}
}

// base: one entity with a form, a list and a stat; tests override pieces.
const lintBase = `{"schemaVersion":1,"app":{"name":"t","voiceEntry":"floating"},"initialScreen":"home",
"entities":[{"id":"log","name":"記録","fields":[
  {"id":"title","name":"内容","type":"string"},
  {"id":"amount","name":"金額","type":"currency"},
  {"id":"start","name":"開始","type":"string"},
  {"id":"date","name":"日付","type":"date","defaultValue":"now"}%s]}],
"screens":[
 {"id":"home","title":"ホーム","toolbar":[{"label":"追加","action":{"type":"navigate","screen":"form","presentation":"sheet"}}],
  "components":[%s
   {"type":"list","entity":"log","item":{"title":"title"}}]},
 {"id":"form","title":"追加","entity":"log","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"log"}}],
  "components":[{"type":"textField","field":"title"}]}]}`

func lintWith(t *testing.T, extraFields, extraComps string) []string {
	t.Helper()
	return lintApp(t, strings.Replace(strings.Replace(lintBase, "%s", extraFields, 1), "%s", extraComps, 1))
}

func wantLint(t *testing.T, got []string, substr string) {
	t.Helper()
	for _, w := range got {
		if strings.Contains(w, substr) {
			return
		}
	}
	t.Errorf("want a warning containing %q, got:\n%s", substr, strings.Join(got, "\n"))
}

func TestLintBaseClean(t *testing.T) {
	if w := lintWith(t, "", ""); len(w) > 0 {
		t.Fatalf("base should be clean: %v", w)
	}
}

func TestLintDateFilterLiteral(t *testing.T) {
	got := lintWith(t, "", `{"type":"list","entity":"log","item":{"title":"title"},"filter":{"field":"date","op":"lte","value":"now"}},`)
	wantLint(t, got, `lte on date field "date" compares with the literal "now"`)
	wantLint(t, got, `daysUntil`)

	// inside an agg of stat.value too
	got = lintWith(t, "", `{"type":"stat","label":"件数","value":{"agg":"count","entity":"log","filter":{"field":"date","op":"gte","value":"2026-01-01"}}},`)
	wantLint(t, got, `agg(log).filter[0]`)

	// expressions are fine; string fields are not date fields
	ok := lintWith(t, "", `{"type":"list","entity":"log","item":{"title":"title"},"filter":{"field":"date","op":"gte","value":{"op":"addDays","args":[{"op":"today","args":[]},-6]}}},
	 {"type":"list","entity":"log","item":{"title":"title"},"filter":{"field":"start","op":"gte","value":"09:00"}},`)
	if len(ok) > 0 {
		t.Errorf("unexpected: %v", ok)
	}
}

func TestLintEntityReach(t *testing.T) {
	raw := strings.Replace(strings.Replace(lintBase, "%s", "", 1), "%s", "", 1)
	var doc map[string]any
	_ = json.Unmarshal([]byte(raw), &doc)
	ents := doc["entities"].([]any)
	doc["entities"] = append(ents, map[string]any{"id": "chores", "name": "家事", "fields": []any{map[string]any{"id": "name", "name": "名前", "type": "string"}}})
	b, _ := json.Marshal(doc)
	got := lintApp(t, string(b))
	wantLint(t, got, `entity "chores" has no add form`)
	wantLint(t, got, `entity "chores" has no list`)

	// form exists but nothing navigates to it
	unreachable := strings.Replace(raw, `"toolbar":[{"label":"追加","action":{"type":"navigate","screen":"form","presentation":"sheet"}}],`, "", 1)
	wantLint(t, lintApp(t, unreachable), `form screen "form" is not reachable`)

	// audit a07: the only list sits on a screen nothing opens
	hidden := strings.Replace(raw, `{"type":"list","entity":"log","item":{"title":"title"}}]},`,
		`{"type":"text","text":"x"}]},{"id":"history","title":"履歴","components":[{"type":"list","entity":"log","item":{"title":"title"}}]},`, 1)
	wantLint(t, lintApp(t, hidden), `entity "log" has no list on a reachable screen`)
}

// logic_budget / logic_oshi: a budget entity read only through an agg needs
// a form but no list; once a ref or a list-like component uses it, it does.
func TestLintSettingEntityNeedsNoList(t *testing.T) {
	raw := strings.Replace(strings.Replace(lintBase, "%s", "", 1), "%s",
		`{"type":"stat","label":"今月の残り","value":{"op":"sub","args":[{"agg":"max","entity":"budget","field":"amount"},{"agg":"sum","entity":"log","field":"amount","filter":{"field":"date","op":"thisMonth","value":null}}]}},
		 {"type":"button","label":"予算","action":{"type":"navigate","screen":"budget_form","presentation":"sheet"}},`, 1)
	var doc map[string]any
	_ = json.Unmarshal([]byte(raw), &doc)
	doc["entities"] = append(doc["entities"].([]any), map[string]any{"id": "budget", "name": "予算", "fields": []any{map[string]any{"id": "amount", "name": "予算額", "type": "currency"}}})
	doc["screens"] = append(doc["screens"].([]any), map[string]any{"id": "budget_form", "title": "予算", "entity": "budget",
		"toolbar":    []any{map[string]any{"label": "保存", "action": map[string]any{"type": "saveRecord", "entity": "budget"}}},
		"components": []any{map[string]any{"type": "numberField", "field": "amount"}}})
	b, _ := json.Marshal(doc)
	if w := lintApp(t, string(b)); len(w) > 0 {
		t.Fatalf("setting entity should be clean: %v", w)
	}

	// still needs an add form
	noForm := strings.Replace(string(b), `"entity":"budget","id":"budget_form"`, `"id":"budget_form"`, 1)
	noForm = strings.Replace(noForm, `{"field":"amount","type":"numberField"}`, `{"text":"x","type":"text"}`, 1)
	wantLint(t, lintApp(t, noForm), `entity "budget" has no add form`)

	// a ref to it makes it a master list again
	withRef := strings.Replace(string(b), `{"defaultValue":"now","id":"date","name":"日付","type":"date"}`,
		`{"defaultValue":"now","id":"date","name":"日付","type":"date"},{"id":"b","name":"予算","type":"ref","refEntity":"budget"}`, 1)
	if withRef == string(b) {
		t.Fatal("fixture did not change")
	}
	wantLint(t, lintApp(t, withRef), `entity "budget" has no list`)
}

func TestLintConstantExpr(t *testing.T) {
	// audit a03: round((toNumber(coalesce(end,"0")) * 0 + 0 - 0) / 1, 1)
	a03 := `,{"id":"hours","name":"勤務時間","type":"number","computed":{"op":"round","args":[{"op":"div","args":[{"op":"sub","args":[{"op":"add","args":[{"op":"mul","args":[{"op":"toNumber","args":[{"op":"coalesce","args":[{"ref":"start"},"0"]}]},0]},0]},0]},1]},1]}}`
	wantLint(t, lintWith(t, a03, ""), `entity "log" field "hours" computed: the expression is always 0`)

	// mul by 0 inside a non-constant expression
	part := `,{"id":"x","name":"x","type":"number","computed":{"op":"add","args":[{"ref":"amount"},{"op":"mul","args":[{"ref":"amount"},0]}]}}`
	wantLint(t, lintWith(t, part, ""), `multiplies by 0`)

	// constants built from literals in stat.value
	wantLint(t, lintWith(t, "", `{"type":"stat","label":"残り","value":{"op":"sub","args":[30000,0]}},`), `always 30000`)

	// real calculations and plain literals are fine
	ok := lintWith(t, `,{"id":"tax","name":"税","type":"number","computed":{"op":"mul","args":[{"ref":"amount"},0.1]}}`,
		`{"type":"stat","label":"残り","value":{"op":"max","args":[{"op":"sub","args":[30000,{"agg":"sum","entity":"log","field":"amount"}]},0]}},`)
	if len(ok) > 0 {
		t.Errorf("unexpected: %v", ok)
	}
}

func TestLintPeriodLabel(t *testing.T) {
	// #87-1: 今月 without a filter
	wantLint(t, lintWith(t, "", `{"type":"stat","label":"今月の支出","aggregate":"sum","entity":"log","field":"amount"},`), `says "今月" but the value has no filter`)
	// wrong period
	wantLint(t, lintWith(t, "", `{"type":"stat","label":"Today's total","aggregate":"sum","entity":"log","field":"amount","filter":{"field":"date","op":"thisMonth","value":null}},`), `the filter is thisMonth; use op today`)
	// value mode: the filter lives in the agg
	wantLint(t, lintWith(t, "", `{"type":"stat","label":"今月の残り","value":{"op":"sub","args":[30000,{"agg":"sum","entity":"log","field":"amount"}]}},`), `has no filter`)
	// weeks / years (#109): thisWeek / thisYear are the answer
	wantLint(t, lintWith(t, "", `{"type":"stat","label":"Points this week","aggregate":"sum","entity":"log","field":"amount","filter":{"field":"date","op":"thisMonth","value":null}},`), `the filter is thisMonth; use op thisWeek`)
	wantLint(t, lintWith(t, "", `{"type":"stat","label":"今年の合計","aggregate":"sum","entity":"log","field":"amount"},`), `"op":"thisYear"`)
	wantLint(t, lintWith(t, "", `{"type":"stat","label":"今月の合計","aggregate":"sum","entity":"log","field":"amount","filter":{"field":"date","op":"thisYear","value":null}},`), `the filter is thisYear; use op thisMonth`)

	// audit a08 (after): 直近7日 without a date filter / a12: 最新 is an avg
	wantLint(t, lintWith(t, "", `{"type":"stat","label":"筋トレ(直近7日)","aggregate":"count","entity":"log"},`), `has no date filter`)
	wantLint(t, lintWith(t, "", `{"type":"stat","label":"最新の体重","aggregate":"avg","entity":"log","field":"amount"},`), `no aggregate returns the newest record`)

	ok := lintWith(t, "", `{"type":"stat","label":"直近7日","aggregate":"count","entity":"log","filter":{"field":"date","op":"gte","value":{"op":"addDays","args":[{"op":"today","args":[]},-6]}}},
	 {"type":"stat","label":"今月の支出","aggregate":"sum","entity":"log","field":"amount","filter":{"field":"date","op":"thisMonth","value":null}},
	 {"type":"stat","label":"今月の残り","value":{"op":"sub","args":[30000,{"agg":"sum","entity":"log","field":"amount","filter":{"field":"date","op":"thisMonth","value":null}}]}},
	 {"type":"stat","label":"今週(直近7日)","aggregate":"sum","entity":"log","field":"amount","filter":{"field":"date","op":"gte","value":{"op":"addDays","args":[{"op":"today","args":[]},-6]}}},
	 {"type":"stat","label":"今日水やりが必要","aggregate":"count","entity":"log","filter":{"field":"amount","op":"lte","value":0}},
	 {"type":"stat","label":"合計","aggregate":"sum","entity":"log","field":"amount"},
	 {"type":"stat","label":"今日までの記録日数","aggregate":"count","entity":"log"},
	 {"type":"stat","label":"今月の予算","aggregate":"max","entity":"log","field":"amount"},
	 {"type":"stat","label":"今週のポイント","aggregate":"sum","entity":"log","field":"amount","filter":{"field":"date","op":"thisWeek","value":null}},
	 {"type":"stat","label":"今年の合計","value":{"agg":"sum","entity":"log","field":"amount","filter":{"field":"date","op":"thisYear","value":null}}},`)
	// ...but the remaining budget is a sum over this month
	wantLint(t, lintWith(t, "", `{"type":"stat","label":"今月の予算の残り","value":{"op":"sub","args":[30000,{"agg":"sum","entity":"log","field":"amount"}]}},`), `has no filter`)
	if len(ok) > 0 {
		t.Errorf("unexpected: %v", ok)
	}
}
