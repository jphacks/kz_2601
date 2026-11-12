package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustLoad(t *testing.T, raw string) *app {
	t.Helper()
	a, err := load([]byte(raw))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return a
}

// screen wraps components in a one-screen app with the given extra top-level JSON.
func screen(components, extra string) string {
	s := `{"initialScreen":"home","screens":[{"id":"home","components":[` + components + `]}]`
	if extra != "" {
		s += "," + extra
	}
	return s + "}"
}

func TestNeedMarks(t *testing.T) {
	cases := []struct {
		need, dsl, want string
	}{
		{"agg", screen(`{"type":"gauge"}`, ""), "○"},
		{"agg", screen(`{"type":"text"}`, ""), "×"},
		{"filter_month", screen(`{"type":"list","filter":{"field":"date","op":"thisMonth"}}`, ""), "○"},
		{"filter_month", screen(`{"type":"list","filter":{"field":"date","op":"today"}}`, ""), "×"},
		{"filter_today", screen(`{"type":"stat","value":{"agg":"sum","filter":{"field":"date","op":"today"}}}`, ""), "○"},
		{"filter_week", screen(`{"type":"list","filter":{"field":"date","op":"thisWeek"}}`, ""), "○"},
		{"filter_week", screen(`{"type":"text"}`, `"entities":[{"id":"e","fields":[{"id":"w","computed":{"agg":"sum","filter":{"field":"d","op":"thisWeek"}}}]}]`), "○"},
		{"filter_week", screen(`{"type":"stat","value":{"op":"weekday","args":[]}}`, ""), "△"},
		{"filter_week", screen(`{"type":"text"}`, ""), "×"},
		{"filter_year", screen(`{"type":"stat","value":{"op":"year","args":[]}}`, ""), "○"},
		{"filter_year", screen(`{"type":"stat","value":{"op":"addDays","args":[]}}`, ""), "△"},
		{"toggle", screen(`{"type":"list","item":{"toggle":"done"}}`, ""), "○"},
		{"toggle", screen(`{"type":"toggle","field":"done"}`, ""), "△"},
		{"toggle", screen(`{"type":"text"}`, ""), "×"},
		{"tabs3", screen(``, `"app":{"tabs":[{"screen":"a"},{"screen":"b"},{"screen":"c"}]}`), "○"},
		{"tabs3", screen(``, `"app":{"tabs":[{"screen":"a"},{"screen":"b"}]}`), "×"},
		{"filter_bool", screen(`{"type":"list","entity":"todo","filter":{"field":"done","op":"eq"}}`, `"entities":[{"id":"todo","fields":[{"id":"done","type":"boolean"}]}]`), "○"},
		{"filter_bool", screen(`{"type":"list","entity":"todo","filter":{"field":"title","op":"eq"}}`, `"entities":[{"id":"todo","fields":[{"id":"title","type":"string"}]}]`), "×"},
		{"filter_bool", screen(`{"type":"list","entity":"todo"}`, ""), "×"},
		{"notify", screen(``, `"app":{"notifications":[{"id":"n"}]}`), "○"},
		{"notify", screen(`{"type":"button","action":{"type":"scheduleNotification"}}`, ""), "○"},
		{"notify", screen(`{"type":"button","action":{"type":"navigate","screen":"home"}}`, ""), "×"},
		{"timer", screen(`{"type":"timerDisplay"}`, ""), "○"},
		{"timer_cycle", screen(`{"type":"timerDisplay"}`, ""), "×"},
		{"chart", screen(`{"type":"chart"}`, ""), "○"},
		{"progress", screen(`{"type":"progressBar"}`, ""), "○"},
		{"progress", screen(`{"type":"stat"}`, ""), "×"},
		{"sort_time", screen(`{"type":"list","sort":{"field":"date"}}`, ""), "○"},
		{"sort_time", screen(`{"type":"table","sort":{"field":"date"}}`, ""), "×"},
		{"two_entities", screen(``, `"entities":[{"id":"a"},{"id":"b"}]`), "○"},
		{"two_entities", screen(``, `"entities":[{"id":"a"}]`), "×"},
		{"group_by", screen(`{"type":"chart","groupBy":"category"}`, ""), "△"},
		{"group_by_month", screen(`{"type":"chart","bucket":"month"}`, ""), "△"},
		{"group_by", screen(`{"type":"chart"}`, ""), "×"},
		{"group_by_member", screen(``, `"entities":[{"id":"m","fields":[{"id":"t","computed":{"agg":"sum"}}]}]`), "○"},
		{"group_by_member", screen(`{"type":"chart"}`, ""), "△"},
		{"group_by_member", screen(``, ""), "×"},
		{"ref", screen(``, `"entities":[{"id":"m","fields":[{"id":"p","type":"ref"}]}]`), "○"},
		{"ref", screen(``, `"entities":[{"id":"m","fields":[{"id":"p","type":"string"}]}]`), "×"},
		{"settlement", screen(`{"type":"settlement"}`, ""), "○"},
		{"calc", screen(``, `"entities":[{"id":"m","fields":[{"id":"c","computed":{"op":"mul","args":[]}}]}]`), "○"},
		{"calc", screen(`{"type":"stat","value":{"op":"sub","args":[]}}`, ""), "○"},
		{"calc", screen(`{"type":"stat","aggregate":"sum"}`, ""), "×"},
		{"remaining", screen(`{"type":"stat","value":{"op":"sub","args":[]}}`, ""), "○"},
		{"remaining", screen(`{"type":"gauge"}`, ""), "△"},
		{"remaining", screen(``, ""), "×"},
		{"days_until", screen(`{"type":"stat","value":{"op":"daysUntil","args":[]}}`, ""), "○"},
		{"date_add", screen(`{"type":"stat","value":{"op":"plus","args":[]}}`, ""), "×"},
		{"time_diff", screen(`{"type":"stat","value":{"op":"hoursBetween","args":[]}}`, ""), "○"},
		{"filter_due", screen(`{"type":"list","value":{"op":"lte","args":[]}}`, ""), "○"},
		{"pb_detect", screen(``, `"entities":[{"id":"m","fields":[{"id":"best","computed":{"agg":"max"}}]}]`), "○"},
		{"pb_detect", screen(`{"type":"stat","aggregate":"max"}`, ""), "△"},
		{"pb_detect", screen(`{"type":"stat","aggregate":"sum"}`, ""), "×"},
		{"rank", screen(``, ""), "×"},
		{"streak", screen(`{"type":"stat","value":{"op":"streak","args":[]}}`, ""), "○"},
		{"streak", screen(`{"type":"stat","value":{"op":"doneInDays","args":[]}}`, ""), "△"},
		{"streak", screen(``, ""), "×"},
		{"bulk_delete", screen(``, ""), "×"},
		{"no_such_need", screen(``, ""), "?"},
	}
	for _, c := range cases {
		m, why := need(mustLoad(t, c.dsl), c.need)
		if m != c.want {
			t.Errorf("need(%s) on %s = %s (%s), want %s", c.need, c.dsl, m, why, c.want)
		}
		if why == "" {
			t.Errorf("need(%s): empty reason", c.need)
		}
	}
}

// An op name that is not an Expr (no args) does not count as a date function.
func TestExprUsesNeedsArgs(t *testing.T) {
	a := mustLoad(t, screen(`{"type":"list","filter":{"field":"d","op":"year"}}`, ""))
	if m, _ := need(a, "filter_year"); m != "×" {
		t.Fatalf("a filter op named year is not an Expr: %s", m)
	}
}

func TestLintKinds(t *testing.T) {
	raw := `{
	  "initialScreen": "home",
	  "app": {"tabs": [{"screen": "home"}]},
	  "entities": [
	    {"id": "expense", "fields": [
	      {"id": "amount", "type": "number", "required": true},
	      {"id": "memo", "type": "string"},
	      {"id": "date", "type": "date"},
	      {"id": "total", "computed": {"op": "sum", "args": []}}
	    ]},
	    {"id": "orphan", "fields": [{"id": "x", "type": "string"}]},
	    {"id": "hidden", "fields": []}
	  ],
	  "screens": [
	    {"id": "home", "components": [
	      {"type": "button", "action": {}},
	      {"type": "button", "action": {"type": "navigate", "screen": "nowhere"}},
	      {"type": "list", "entity": "expense", "label": "今月の支出", "filter": {"field": "date", "op": "today"}},
	      {"type": "stat", "entity": "expense", "label": "今年の合計"},
	      {"type": "stat", "entity": "expense", "label": "残り", "aggregate": "sum"},
	      {"type": "text", "text": "残り 3000円"},
	      {"type": "toggle", "field": "flag"}
	    ]},
	    {"id": "add", "entity": "expense", "components": [
	      {"type": "textField", "field": "memo"}
	    ]},
	    {"id": "hiddenForm", "entity": "hidden", "components": [
	      {"type": "button", "action": {"type": "saveRecord"}}
	    ]},
	    {"id": "lost", "components": []}
	  ]
	}`
	got := strings.Join(lint(mustLoad(t, raw)), "\n")
	for _, want := range []string{
		"dead_button: home",
		`bad_nav: home → 存在しない画面 "nowhere"`,
		"unreachable: 画面 lost",
		"unreachable: 画面 add",
		"unreachable: 画面 hiddenForm", // its own saveRecord is no way in
		"no_save: フォーム add(entity expense)",
		"no_add_form: entity orphan",
		"form_unreachable: entity hidden",
		"no_list: entity orphan",
		"no_list: entity hidden",
		"required_no_input: expense.amount",
		`label_filter_mismatch: home list "今月の支出" は今月と言うが filter thisMonth`,
		`label_filter_mismatch: home stat "今年の合計" は今年と言うが`,
		`label_promises_calc: home stat "残り"`,
		`static_value_text: home の text "残り 3000円"`,
		"toggle_invisible: toggle flag",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("lint missing %q in:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{
		"expense.date",  // date fields are filled automatically
		"expense.total", // computed fields have no input
		"expense.memo",  // bound to a textField on the form
		"orphan.x",      // no form at all: only no_add_form
		"unreachable: 画面 home",
	} {
		if strings.Contains(got, unwanted) {
			t.Errorf("lint must not report %q:\n%s", unwanted, got)
		}
	}
}

func TestLintCleanApp(t *testing.T) {
	raw := `{
	  "initialScreen": "home",
	  "entities": [{"id": "todo", "fields": [{"id": "title", "type": "string", "required": true}, {"id": "done", "type": "boolean"}]}],
	  "screens": [
	    {"id": "home", "toolbar": [{"action": {"type": "navigate", "screen": "add"}}], "components": [
	      {"type": "list", "entity": "todo", "label": "今日のやること", "filter": {"field": "done", "op": "today"}, "item": {"title": "title"}}
	    ]},
	    {"id": "add", "entity": "todo", "components": [
	      {"type": "textField", "field": "title"},
	      {"type": "toggle", "field": "done"},
	      {"type": "button", "action": {"type": "saveRecord", "screen": "home"}}
	    ]}
	  ]
	}`
	if l := lint(mustLoad(t, raw)); len(l) != 0 {
		t.Fatalf("clean app linted: %v", l)
	}
}

func TestLoadRejectsInvalidJSON(t *testing.T) {
	if _, err := load([]byte(`{`)); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}

func TestEvaluateAndScore(t *testing.T) {
	dir := t.TempDir()
	run := `{"model":"m","tag":"t","cases":[
	  {"id":"good","ok":true,"dsl":` + screen(`{"type":"chart","groupBy":"c"}`, `"app":{"tabs":[{"screen":"a"},{"screen":"b"},{"screen":"c"}]}`) + `},
	  {"id":"broken","ok":true,"dsl":"not an object"},
	  {"id":"failed","ok":false,"error":"timeout"}
	]}`
	path := filepath.Join(dir, "run.json")
	if err := os.WriteFile(path, []byte(run), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []auditCase{
		{ID: "good", Needs: []string{"chart", "tabs3", "progress", "group_by"}},
		{ID: "broken", Needs: []string{"chart"}},
		{ID: "failed", Needs: []string{"chart"}},
	}
	res, rf := evaluate(cases, path)
	if rf.Model != "m" || rf.Tag != "t" || len(res) != 3 {
		t.Fatalf("run file = %+v, results %d", rf, len(res))
	}
	if ok, part, ng := score(res["good"]); ok != 2 || part != 1 || ng != 1 {
		t.Fatalf("good score = %d/%d/%d, want 2/1/1 (%v)", ok, part, ng, res["good"].marks)
	}
	if r := res["broken"]; r.ok || r.err == "" || len(r.marks) != 0 {
		t.Fatalf("undecodable DSL must be a failure: %+v", r)
	}
	if r := res["failed"]; r.ok || r.err != "timeout" {
		t.Fatalf("failed case = %+v", r)
	}
	if ok, part, ng := score(result{}); ok+part+ng != 0 {
		t.Fatal("empty result must score 0/0/0")
	}
}
