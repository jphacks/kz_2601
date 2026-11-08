package dsl

import (
	"encoding/json"
	"strings"
	"testing"
)

const settingMaxMinMsg = "is the largest/smallest value ever entered"

// budgetApp: lintBase plus a budget entity (amount, date) with its own form,
// opened by a button on home; comps go first on home, budgetFields extend
// the budget entity.
func budgetApp(t *testing.T, comps, budgetFields string, withButton bool) []string {
	t.Helper()
	button := ""
	if withButton {
		button = `{"type":"button","label":"予算","action":{"type":"navigate","screen":"budget_form","presentation":"sheet"}},`
	}
	raw := strings.Replace(strings.Replace(lintBase, "%s", "", 1), "%s", comps+button, 1)
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	var fields []any
	if err := json.Unmarshal([]byte(`[{"id":"amount","name":"予算額","type":"currency"},{"id":"date","name":"日付","type":"date","defaultValue":"now"}`+budgetFields+`]`), &fields); err != nil {
		t.Fatal(err)
	}
	doc["entities"] = append(doc["entities"].([]any), map[string]any{"id": "budget", "name": "予算", "fields": fields})
	doc["screens"] = append(doc["screens"].([]any), map[string]any{"id": "budget_form", "title": "予算", "entity": "budget",
		"toolbar":    []any{map[string]any{"label": "保存", "action": map[string]any{"type": "saveRecord", "entity": "budget"}}},
		"components": []any{map[string]any{"type": "numberField", "field": "amount"}}})
	b, _ := json.Marshal(doc)
	return lintApp(t, string(b))
}

func TestLintSettingMaxMin(t *testing.T) {
	remaining := `{"type":"stat","label":"今月の残り","value":{"op":"sub","args":[{"agg":"max","entity":"budget","field":"amount"},{"agg":"sum","entity":"log","field":"amount","filter":{"field":"date","op":"thisMonth","value":null}}]}},`
	wantLint(t, budgetApp(t, remaining, "", true), `screen "home" components[0].value: max of "budget" `+settingMaxMinMsg)

	// min, and the fix in the message
	got := budgetApp(t, strings.Replace(remaining, `"agg":"max"`, `"agg":"min"`, 1), "", true)
	wantLint(t, got, `min of "budget"`)
	wantLint(t, got, `"single": true`)
	wantLint(t, got, `{"agg":"latest","entity":"budget"`)

	// stat aggregate (progressBar / gauge allow count / sum only)
	comp := `{"type":"stat","label":"予算","entity":"budget","aggregate":"max","field":"amount"},`
	wantLint(t, budgetApp(t, comp, "", true), `components[0] aggregate: max of "budget"`)

	// a computed field of another entity
	computed := `,{"id":"left","name":"残り","type":"currency","computed":{"op":"sub","args":[{"agg":"max","entity":"budget","field":"amount"},{"ref":"amount"}]}}`
	raw := strings.Replace(strings.Replace(lintBase, "%s", computed, 1), "%s",
		`{"type":"button","label":"予算","action":{"type":"navigate","screen":"budget_form","presentation":"sheet"}},`, 1)
	var doc map[string]any
	_ = json.Unmarshal([]byte(raw), &doc)
	doc["entities"] = append(doc["entities"].([]any), map[string]any{"id": "budget", "name": "予算", "fields": []any{map[string]any{"id": "amount", "name": "予算額", "type": "currency"}}})
	doc["screens"] = append(doc["screens"].([]any), map[string]any{"id": "budget_form", "title": "予算", "entity": "budget",
		"toolbar":    []any{map[string]any{"label": "保存", "action": map[string]any{"type": "saveRecord", "entity": "budget"}}},
		"components": []any{map[string]any{"type": "numberField", "field": "amount"}}})
	b, _ := json.Marshal(doc)
	wantLint(t, lintApp(t, string(b)), `entity "log" field "left" computed: max of "budget"`)
}

func TestLintSettingMaxMin_Clean(t *testing.T) {
	stat := func(agg, field string) string {
		return `{"type":"stat","label":"予算","value":{"agg":"` + agg + `","entity":"budget","field":"` + field + `"}},`
	}
	cases := map[string][]string{
		"latest": budgetApp(t, `{"type":"stat","label":"予算","value":{"agg":"latest","entity":"budget","field":"amount","by":"date"}},`, "", true),
		"sum":    budgetApp(t, stat("sum", "amount"), "", true),
		// nothing opens a new budget record (the form is only reachable some other way)
		"no add button": budgetApp(t, stat("max", "amount"), "", false),
		// the next exam: min of a date, not a goal
		"date field": budgetApp(t, stat("min", "date"), "", true),
		// a computed number (days left) is not an entered goal
		"computed field": budgetApp(t, stat("min", "days"), `,{"id":"days","name":"残り日数","type":"integer","computed":{"op":"daysUntil","args":[{"ref":"date"}]}}`, true),
		// listed records (a best score) are not a setting
		"listed entity": budgetApp(t, stat("max", "amount")+`{"type":"list","entity":"budget","item":{"title":"amount"}},`, "", true),
		// a log also summed / counted is a record, so max is its best (#769 review)
		"summed log": budgetApp(t, stat("max", "amount")+`{"type":"stat","label":"合計","value":{"agg":"sum","entity":"budget","field":"amount"}},`, "", true),
		"counted log": budgetApp(t, stat("max", "amount")+`{"type":"stat","label":"回数","entity":"budget","aggregate":"count"},`, "", true),
		// the label asks for the best record
		"best label": budgetApp(t, `{"type":"stat","label":"自己ベスト","entity":"budget","aggregate":"max","field":"amount"},`, "", true),
		"best label value": budgetApp(t, `{"type":"stat","label":"Best run","value":{"agg":"max","entity":"budget","field":"amount"}},`, "", true),
	}
	for name, got := range cases {
		t.Run(name, func(t *testing.T) { noLint(t, got, settingMaxMinMsg) })
	}
}
