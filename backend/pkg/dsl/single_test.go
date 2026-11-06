package dsl

import (
	"encoding/json"
	"strings"
	"testing"
)

// periodApp is the d_period shape of #267: the cycle is a setting, not a record.
const periodApp = `{
  "schemaVersion": 1,
  "app": {"name": "生理の記録", "voiceEntry": "floating"},
  "initialScreen": "home",
  "entities": [
    {"id": "periods", "name": "生理", "fields": [{"id": "start_on", "name": "開始日", "type": "date"}]},
    {"id": "settings", "name": "設定", "single": true, "fields": [
      {"id": "cycle_days", "name": "周期", "type": "integer", "defaultValue": 28, "unit": "日"}
    ]}
  ],
  "screens": [
    {"id": "home", "title": "記録", "toolbar": [
      {"label": "追加", "icon": "plus", "action": {"type": "navigate", "screen": "period_form"}},
      {"label": "周期", "icon": "gearshape", "action": {"type": "navigate", "screen": "settings_form"}}
    ], "components": [
      {"type": "stat", "label": "次の予定日", "value": {"op": "addDays", "args": [
        {"agg": "latest", "entity": "periods", "field": "start_on", "by": "start_on"}, {"setting": "cycle_days"}]}},
      {"type": "list", "entity": "periods", "item": {"title": "start_on"}, "sort": {"field": "start_on", "direction": "desc"},
       "onItemTap": {"type": "navigate", "screen": "period_form"}}
    ]},
    {"id": "period_form", "title": "記録", "entity": "periods", "toolbar": [
      {"label": "保存", "action": {"type": "saveRecord", "entity": "periods"}}
    ], "components": [{"type": "datePicker", "field": "start_on", "label": "開始日"}]},
    {"id": "settings_form", "title": "周期の設定", "entity": "settings", "toolbar": [
      {"label": "保存", "action": {"type": "saveRecord", "entity": "settings"}}
    ], "components": [{"type": "numberField", "field": "cycle_days", "label": "周期", "unit": "日"}]}
  ]
}`

func parsePeriod(t *testing.T) *App {
	t.Helper()
	app, err := Parse([]byte(periodApp))
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func TestSingleEntityParses(t *testing.T) {
	app := parsePeriod(t)
	if got := app.SingleEntities(); len(got) != 1 || got[0] != "settings" {
		t.Fatalf("single entities = %v", got)
	}
	for _, w := range app.Lint() {
		if strings.Contains(w, `"settings"`) {
			t.Errorf("the single entity needs no list: %s", w)
		}
	}
	raw, err := json.Marshal(app)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"single":true`, `{"setting":"cycle_days"}`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("round trip lost %s", want)
		}
	}
}

func singleProblems(t *testing.T, mod func(a *App)) string {
	t.Helper()
	app := parsePeriod(t)
	mod(app)
	err := app.Validate()
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestSingleEntityMisuse(t *testing.T) {
	setting := func(id string) *Expr { return &Expr{Kind: ExprSetting, Setting: id} }
	for name, c := range map[string]struct {
		mod  func(a *App)
		want string
	}{
		"list": {func(a *App) {
			a.Screens[0].Components = append(a.Screens[0].Components, Component{Type: CompList, Entity: "settings", Item: &ListItem{Title: "cycle_days"}})
		}, `list over single entity "settings"`},
		"stat aggregate": {func(a *App) {
			a.Screens[0].Components = append(a.Screens[0].Components, Component{Type: CompStat, Label: "周期", Entity: "settings", Aggregate: "latest", Field: "cycle_days"})
		}, `stat over single entity "settings"`},
		"agg": {func(a *App) {
			a.Screens[0].Components[0].Value = &Expr{Kind: ExprAgg, Agg: &AggExpr{Agg: "latest", Entity: "settings", Field: "cycle_days"}}
		}, `read its value with {"setting":"<fieldId>"} instead of agg`},
		"delete": {func(a *App) {
			a.Screens[2].Toolbar = append(a.Screens[2].Toolbar, ToolbarButton{Label: "削除", Action: Action{Type: ActionDeleteRecord, Entity: "settings"}})
		}, `deleteRecord on single entity "settings"`},
		"ref": {func(a *App) {
			a.Entities[0].Fields = append(a.Entities[0].Fields, Field{ID: "cfg", Name: "設定", Type: FieldRef, RefEntity: "settings"})
		}, `refEntity "settings" is single`},
		"shared":          {func(a *App) { a.Entities[1].Scope = ScopeShared }, `only for a private entity`},
		"retention":       {func(a *App) { a.Entities[1].RetentionDays = 7 }, `remove retentionDays`},
		"hidden":          {func(a *App) { a.Entities[1].Fields[0].Hidden = true }, `cannot be hidden`},
		"unknown setting": {func(a *App) { a.Screens[0].Components[0].Value = setting("cycle") }, `{"setting":"cycle"} is not a field of a single entity (settings: cycle_days)`},
		"no single entity": {func(a *App) {
			a.Entities[1].Single = false
			a.Screens[0].Components[0].Value = setting("cycle_days")
		}, `no single entity in this app`},
		"ambiguous": {func(a *App) {
			a.Entities = append(a.Entities, Entity{ID: "more", Name: "他", Single: true, Fields: []Field{{ID: "cycle_days", Name: "周期", Type: FieldInteger}}})
		}, `is ambiguous`},
		"type": {func(a *App) {
			a.Screens[0].Components[0].Value = &Expr{Kind: ExprOp, Op: "len", Args: []Expr{*setting("cycle_days")}}
		}, `len expects a string but got a number`},
	} {
		t.Run(name, func(t *testing.T) {
			if got := singleProblems(t, c.mod); !strings.Contains(got, c.want) {
				t.Fatalf("want %q in:\n%s", c.want, got)
			}
		})
	}
}

// A setting is readable anywhere: a computed of another entity and a row-less value.
func TestSettingInComputed(t *testing.T) {
	if got := singleProblems(t, func(a *App) {
		a.Entities[0].Fields = append(a.Entities[0].Fields, Field{ID: "next_on", Name: "次の予定日", Type: FieldDate,
			Computed: &Expr{Kind: ExprOp, Op: "addDays", Args: []Expr{{Kind: ExprRef, Ref: "start_on"}, {Kind: ExprSetting, Setting: "cycle_days"}}}})
	}); got != "" {
		t.Fatal(got)
	}
}

func TestSettingExprShape(t *testing.T) {
	var e Expr
	if err := json.Unmarshal([]byte(`{"setting":"goal","ref":"x"}`), &e); err == nil {
		t.Fatal("mixed keys must fail")
	}
	if err := json.Unmarshal([]byte(`{"setting":"goal"}`), &e); err != nil || e.Kind != ExprSetting || e.Setting != "goal" {
		t.Fatalf("%v %+v", err, e)
	}
}
