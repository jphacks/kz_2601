package dsl

import (
	"encoding/json"
	"strings"
	"testing"
)

func exportApp(actions ...Action) App {
	app := Initial()
	app.Entities = []Entity{{ID: "expense", Name: "支出", Fields: []Field{
		{ID: "date", Name: "日付", Type: FieldDate},
		{ID: "amount", Name: "金額", Type: FieldCurrency},
		{ID: "photo", Name: "レシート", Type: FieldPhoto},
		{ID: "tax", Name: "税", Type: FieldCurrency, Computed: &Expr{Kind: ExprOp, Op: "mul", Args: []Expr{
			{Kind: ExprRef, Ref: "amount"}, *NumExpr(0.1)}}},
	}}}
	var cs []Component
	for i := range actions {
		cs = append(cs, Component{Type: CompButton, Label: "書き出す", Action: &actions[i]})
	}
	app.Screens[0].Components = cs
	return app
}

func TestExportActionValid(t *testing.T) {
	app := exportApp(
		Action{Type: ActionExport, Entity: "expense", Format: "csv"},
		Action{Type: ActionExport, Entity: "expense", Format: "pdf", Title: "今月の家計簿",
			Filter: &Filter{Field: "date", Op: "thisMonth"}, Fields: []string{"date", "amount", "tax"}},
	)
	if err := app.Validate(); err != nil {
		t.Fatalf("valid export rejected: %v", err)
	}
}

func TestExportActionErrors(t *testing.T) {
	cases := []struct {
		act  Action
		want string
	}{
		{Action{Type: ActionExport, Entity: "expense"}, `export format "" must be csv or pdf`},
		{Action{Type: ActionExport, Entity: "expense", Format: "xlsx"}, `export format "xlsx"`},
		{Action{Type: ActionExport, Entity: "nope", Format: "csv"}, `export entity "nope" does not exist`},
		{Action{Type: ActionExport, Entity: "expense", Format: "csv", Fields: []string{"memo"}}, `export field "memo" not in entity`},
		{Action{Type: ActionExport, Entity: "expense", Format: "csv", Fields: []string{"photo"}}, `export field "photo" is photo`},
		{Action{Type: ActionExport, Entity: "expense", Format: "csv", Fields: []string{"date", "date"}}, `listed twice`},
		{Action{Type: ActionExport, Entity: "expense", Format: "csv", Filter: &Filter{Field: "memo", Op: "eq", Value: "x"}}, `field "memo" not in entity`},
		{Action{Type: ActionExport, Entity: "expense", Format: "csv", Filter: &Filter{Field: "amount", Op: "thisMonth"}}, `date`},
	}
	for _, tc := range cases {
		app := exportApp(tc.act)
		err := app.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%+v: want %q, got %v", tc.act, tc.want, err)
		}
	}
}

// The action round-trips through JSON with the schema's key names.
func TestExportActionJSON(t *testing.T) {
	raw := []byte(`{"type":"export","entity":"expense","format":"csv","filter":{"field":"date","op":"thisMonth","value":null},"fields":["date","amount"],"title":"家計簿"}`)
	var a Action
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatal(err)
	}
	if a.Type != ActionExport || a.Format != "csv" || a.Filter == nil || a.Filter.Op != "thisMonth" || len(a.Fields) != 2 || a.Title != "家計簿" {
		t.Fatalf("decoded %+v", a)
	}
	app := exportApp(a)
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
}
