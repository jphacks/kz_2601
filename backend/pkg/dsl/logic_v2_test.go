package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shoppingApp loads docs/dsl/examples/shopping.json (list.item.toggle + thisWeek).
func shoppingApp(t *testing.T) App {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "shopping.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatalf("shopping example invalid: %v", err)
	}
	return *app
}

// todoList is the "買うもの" list (section todo > list).
func todoList(app *App) *Component { return &app.Screens[0].Components[1].Children[0] }

func wantProblem(t *testing.T, app App, sub string) {
	t.Helper()
	err := app.Validate()
	if err == nil {
		t.Fatalf("expected a problem containing %q, got none", sub)
	}
	if !strings.Contains(err.Error(), sub) {
		t.Fatalf("expected a problem containing %q, got: %v", sub, err)
	}
}

func TestShoppingExampleValid(t *testing.T) {
	app := shoppingApp(t)
	if got := todoList(&app).Item.Toggle; got != "done" {
		t.Fatalf("todo list toggle = %q, want done", got)
	}
}

func TestListToggleRejects(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*App)
		want string
	}{
		{"unknown field", func(a *App) { todoList(a).Item.Toggle = "bought" }, `list item.toggle "bought" not in entity "groceries"`},
		{"not boolean", func(a *App) { todoList(a).Item.Toggle = "name" }, `list item.toggle "name" must be a boolean field (got string)`},
		{"computed", func(a *App) {
			a.Entities[0].Fields = append(a.Entities[0].Fields, Field{ID: "many", Name: "多い", Type: FieldBoolean,
				Computed: &Expr{Kind: ExprOp, Op: "gt", Args: []Expr{{Kind: ExprRef, Ref: "qty"}, {Kind: ExprLiteral, Lit: 3.0}}}})
			todoList(a).Item.Toggle = "many"
		}, `list item.toggle "many" is computed`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := shoppingApp(t)
			tc.mut(&app)
			wantProblem(t, app, tc.want)
		})
	}
}

func TestPeriodFilterOps(t *testing.T) {
	for _, op := range []string{"today", "thisWeek", "thisMonth", "thisYear"} {
		app := shoppingApp(t)
		todoList(&app).Filter = &Filter{Field: "added", Op: op}
		if err := app.Validate(); err != nil {
			t.Errorf("%s on a date field: %v", op, err)
		}
	}
	// schema accepts the new enum values
	raw, _ := os.ReadFile(filepath.Join(docsDir, "examples", "shopping.json"))
	yearly := strings.Replace(string(raw), `"op": "thisWeek"`, `"op": "thisYear"`, 1)
	if _, err := Parse([]byte(yearly)); err != nil {
		t.Errorf("thisYear rejected: %v", err)
	}
	if _, err := Parse([]byte(strings.Replace(string(raw), `"op": "thisWeek"`, `"op": "lastWeek"`, 1))); err == nil {
		t.Error("lastWeek should be rejected by the schema enum")
	}
}

func TestPeriodFilterRejects(t *testing.T) {
	t.Run("value must be null", func(t *testing.T) {
		app := shoppingApp(t)
		todoList(&app).Filter = &Filter{Field: "added", Op: "thisWeek", Value: "2026-10-19"}
		wantProblem(t, app, "filter op thisWeek takes value null")
	})
	t.Run("non-date field", func(t *testing.T) {
		app := shoppingApp(t)
		todoList(&app).Filter = &Filter{Field: "name", Op: "thisYear"}
		wantProblem(t, app, `filter op thisYear needs a date or datetime field but "name" is string`)
	})
	t.Run("inside agg", func(t *testing.T) {
		app := shoppingApp(t)
		stat := &app.Screens[0].Components[0].Children[1]
		stat.Aggregate, stat.Entity, stat.Field, stat.Filter = "", "", "", nil
		stat.Value = &Expr{Kind: ExprAgg, Agg: &AggExpr{Agg: "count", Entity: "groceries",
			Filter: []Filter{{Field: "qty", Op: "thisWeek"}}}}
		wantProblem(t, app, `filter op thisWeek needs a date or datetime field but "qty" is integer`)
	})
}

// shiftApp adds a shift entity whose computed fields use hoursBetween.
func shiftApp(t *testing.T, hours string) App {
	t.Helper()
	app := shoppingApp(t)
	app.Entities = append(app.Entities, Entity{
		ID: "shift", Name: "シフト", Fields: []Field{
			{ID: "date", Name: "日付", Type: FieldDate},
			{ID: "start", Name: "開始", Type: FieldString},
			{ID: "end", Name: "終了", Type: FieldString},
			{ID: "wage", Name: "時給", Type: FieldCurrency},
			{ID: "hours", Name: "時間", Type: FieldNumber, Computed: mustExpr(t, hours)},
			{ID: "pay", Name: "給料", Type: FieldCurrency, Computed: mustExpr(t, `{"op":"round","args":[{"op":"mul","args":[{"ref":"hours"},{"ref":"wage"}]}]}`)},
		},
	})
	return app
}

func TestHoursBetweenValid(t *testing.T) {
	for _, hours := range []string{
		`{"op":"hoursBetween","args":[{"ref":"start"},{"ref":"end"}]}`,
		`{"op":"hoursBetween","args":["22:00","05:00"]}`,
		`{"op":"hoursBetween","args":[{"ref":"start"},{"op":"coalesce","args":[{"ref":"end"},"18:00"]}]}`,
	} {
		app := shiftApp(t, hours)
		if err := app.Validate(); err != nil {
			t.Errorf("%s: %v", hours, err)
		}
	}
}

func TestHoursBetweenRejects(t *testing.T) {
	cases := map[string]string{
		`{"op":"hoursBetween","args":[{"ref":"start"}]}`:                              "op hoursBetween takes exactly 2 args (got 1)",
		`{"op":"hoursBetween","args":[{"op":"toNumber","args":[{"ref":"start"}]},9]}`: `hoursBetween expects a "HH:mm" time string`,
		`{"op":"hoursBetween","args":[{"ref":"wage"},{"ref":"end"}]}`:                 `hoursBetween expects a "HH:mm" time string (or a datetime field) but got a number`,
	}
	for hours, want := range cases {
		wantProblem(t, shiftApp(t, hours), want)
	}
	// result is a number: a string-typed computed field is rejected
	app := shiftApp(t, `{"op":"hoursBetween","args":["09:00","18:00"]}`)
	app.Entities[1].Fields[4].Type = FieldDate
	wantProblem(t, app, "computed expression yields a number but the field type is date")
}
