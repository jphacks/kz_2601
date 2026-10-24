package dsl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Logic v3: agg latest/earliest (#133), {"ref":"a.b"} lookup (#134),
// list.item.resets daily (#132).

func loadExample(t *testing.T, name string) App {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatalf("%s example invalid: %v", name, err)
	}
	return *app
}

func wantValid(t *testing.T, app App) {
	t.Helper()
	if err := app.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

// withStat returns app with a stat appended to its home screen.
func withStat(t *testing.T, app App, c Component) App {
	t.Helper()
	c.Type = CompStat
	c.Label = "x"
	c.Area = ""
	app.Screens[0].Components = append(app.Screens[0].Components, c)
	return app
}

func TestAggLatest(t *testing.T) {
	// warikan: expenses{title, amount, payer: ref, date}
	ok := []Component{
		{Aggregate: "latest", Entity: "expenses", Field: "amount", By: "date"},
		{Aggregate: "earliest", Entity: "expenses", Field: "title"}, // by omitted = first date field
		{Value: mustExpr(t, `{"op":"sub","args":[{"agg":"latest","entity":"expenses","field":"amount","by":"date"},{"agg":"earliest","entity":"expenses","field":"amount","by":"date"}]}`)},
		{Value: mustExpr(t, `{"agg":"latest","entity":"expenses","field":"date"}`)}, // 最後に○○した日
	}
	for i, c := range ok {
		app := withStat(t, loadExample(t, "warikan"), c)
		if err := app.Validate(); err != nil {
			t.Errorf("case %d: %v", i, err)
		}
	}
	bad := []struct {
		c    Component
		want string
	}{
		{Component{Aggregate: "latest", Entity: "expenses", Field: "amount", By: "title"}, `latest by "title" must be a date or datetime field`},
		{Component{Aggregate: "latest", Entity: "expenses", Field: "amount", By: "ghost"}, `latest by "ghost" not in entity "expenses"`},
		{Component{Aggregate: "sum", Entity: "expenses", Field: "amount", By: "date"}, `by is only for aggregate latest/earliest`},
		{Component{Aggregate: "latest", Entity: "expenses"}, `stat field "" not in entity "expenses"`},
		{Component{Value: mustExpr(t, `{"agg":"max","entity":"expenses","field":"amount","by":"date"}`)}, `by is only for agg latest/earliest`},
		{Component{Value: mustExpr(t, `{"op":"add","args":[{"agg":"latest","entity":"expenses","field":"title"},1]}`)}, `add expects a number but got a string`},
	}
	for _, tc := range bad {
		wantProblem(t, withStat(t, loadExample(t, "warikan"), tc.c), tc.want)
	}
	// the schema accepts latest on stat (and the JSON round-trips `by`)
	e := mustExpr(t, `{"agg":"latest","entity":"body","field":"weight","by":"date"}`)
	b, _ := json.Marshal(e)
	if string(b) != `{"agg":"latest","entity":"body","field":"weight","by":"date"}` {
		t.Fatalf("round trip = %s", b)
	}
}

func TestRefLookup(t *testing.T) {
	addComputed := func(app *App, f Field) {
		app.Entities[1].Fields = append(app.Entities[1].Fields, f) // expenses
	}
	app := loadExample(t, "warikan")
	addComputed(&app, Field{ID: "payer_name", Name: "払った人", Type: FieldString, Computed: mustExpr(t, `{"ref":"payer.name"}`)})
	addComputed(&app, Field{ID: "payer_balance", Name: "残高", Type: FieldCurrency, Computed: mustExpr(t, `{"op":"mul","args":[{"ref":"payer.balance"},1]}`)})
	addComputed(&app, Field{ID: "payer_id", Name: "id", Type: FieldString, Computed: mustExpr(t, `{"ref":"payer.id"}`)})
	wantValid(t, app)

	bad := []struct{ expr, want string }{
		{`{"ref":"payer.ghost"}`, `"ghost" is not a field of entity "members"`},
		{`{"ref":"title.name"}`, `"title" is string, not a ref field`},
		{`{"ref":"nobody.name"}`, `"nobody" is not a field of entity "expenses"`},
		{`{"ref":"payer.name.x"}`, `only one level`},
		{`{"op":"add","args":[{"ref":"payer.name"},1]}`, `add expects a number but got a string`},
	}
	for _, tc := range bad {
		app := loadExample(t, "warikan")
		addComputed(&app, Field{ID: "z", Name: "z", Type: FieldString, Computed: mustExpr(t, tc.expr)})
		wantProblem(t, app, tc.want)
	}
	// stat.value has no row: lookup is a ref, so it is rejected there too
	wantProblem(t, withStat(t, loadExample(t, "warikan"), Component{Value: mustExpr(t, `{"ref":"payer.name"}`)}), `is not allowed here`)

	// a cycle through the lookup: members.paid reads expenses.payer_paid reads members.paid
	app = loadExample(t, "warikan")
	addComputed(&app, Field{ID: "payer_paid", Name: "p", Type: FieldCurrency, Computed: mustExpr(t, `{"ref":"payer.paid"}`)})
	for i := range app.Entities[0].Fields {
		if app.Entities[0].Fields[i].ID == "paid" {
			app.Entities[0].Fields[i].Computed = mustExpr(t, `{"agg":"sum","entity":"expenses","field":"payer_paid"}`)
		}
	}
	wantProblem(t, app, "computed field cycle")
}

func medicineList(app *App) *Component { return &app.Screens[0].Components[1].Children[0] }

func TestDailyToggle(t *testing.T) {
	app := loadExample(t, "medicine")
	if it := medicineList(&app).Item; it.Toggle != "taken_on" || it.Resets != "daily" {
		t.Fatalf("medicine list item = %+v", it)
	}
	cases := []struct {
		name string
		mut  func(*App)
		want string
	}{
		{"unknown resets", func(a *App) { medicineList(a).Item.Resets = "weekly" }, `list item.resets "weekly" must be one of daily`},
		{"daily on boolean", func(a *App) {
			a.Entities[0].Fields = append(a.Entities[0].Fields, Field{ID: "done", Name: "済", Type: FieldBoolean})
			medicineList(a).Item.Toggle = "done"
		}, `with resets daily must be a date field (got boolean)`},
		{"date without daily", func(a *App) { medicineList(a).Item.Resets = "" }, `add "resets":"daily"`},
		{"resets without toggle", func(a *App) { medicineList(a).Item.Toggle = "" }, `list item.resets needs item.toggle`},
		{"defaultValue", func(a *App) { a.Entities[0].Fields[2].DefaultValue = "now" }, `must not have defaultValue`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := loadExample(t, "medicine")
			tc.mut(&app)
			wantProblem(t, app, tc.want)
		})
	}
}

func TestSchemaHasLogicV3(t *testing.T) {
	s := string(SchemaJSON())
	for _, w := range []string{`"latest"`, `"earliest"`, `"resets"`, `"by"`} {
		if !strings.Contains(s, w) {
			t.Errorf("schema lacks %s", w)
		}
	}
}
