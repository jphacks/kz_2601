package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func growthApp() App {
	app := Initial()
	app.Entities = []Entity{
		{ID: "saving", Name: "貯金", Fields: []Field{
			{ID: "amount", Name: "金額", Type: FieldCurrency, Required: true},
			{ID: "memo", Name: "メモ", Type: FieldString},
		}},
		{ID: "setting", Name: "設定", Single: true, Fields: []Field{
			{ID: "goal", Name: "目標", Type: FieldInteger, DefaultValue: 100000.0},
			{ID: "count", Name: "回数", Type: FieldInteger, DefaultValue: 0.0},
			{ID: "title", Name: "名前", Type: FieldString},
		}},
	}
	return app
}

func num(v float64) *Expr { return &Expr{Kind: ExprLiteral, Lit: v} }

func TestGrowthValid(t *testing.T) {
	app := growthApp()
	step, ten, one := 1000.0, 10.0, 1.0
	app.Screens[0].Components = []Component{
		{Type: CompGrowth, Label: "歩いて育てる木", Metric: "steps", Live: true, Target: num(8000), Step: &step},
		{Type: CompGrowth, Label: "花", Kind: "flower", Metric: "steps", Target: num(10000)},
		{Type: CompGrowth, Label: "貯金の木", Kind: "plant", Entity: "saving", Field: "amount", Target: &Expr{Kind: ExprSetting, Setting: "goal"}, Unit: "円"},
		{Type: CompGrowth, Label: "記録の数", Entity: "saving", Aggregate: "count", Target: num(20)},
		{Type: CompGrowth, Label: "カウンタ", Value: &Expr{Kind: ExprSetting, Setting: "count"}, Target: num(10)},
		// booth demo: from when the card appeared, a fruit every 10 steps (target = step × 10)
		{Type: CompGrowth, Label: "展示の木", Metric: "steps", Live: true, Since: "open", Step: &ten},
		{Type: CompGrowth, Label: "今日の木", Metric: "steps", Since: "today", Target: num(100), Step: &ten},
		// target / step > 30 goes round (30 fruits, harvest, again) instead of failing
		{Type: CompGrowth, Label: "たくさんの実", Metric: "steps", Since: "open", Target: num(1000), Step: &one},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("valid growth rejected: %v", err)
	}
}

func TestGrowthSemanticErrors(t *testing.T) {
	app := growthApp()
	g := func(mod func(c *Component)) Component {
		c := Component{Type: CompGrowth, Label: "木", Metric: "steps", Target: num(8000)}
		mod(&c)
		return c
	}
	zero := 0.0
	app.Screens[0].Components = []Component{
		g(func(c *Component) { c.Label = "" }),
		g(func(c *Component) { c.Kind = "cactus" }),
		g(func(c *Component) { c.Metric = "" }),
		g(func(c *Component) { c.Entity = "saving"; c.Field = "amount" }),
		g(func(c *Component) { c.Metric = "weight" }),
		g(func(c *Component) { c.Metric = ""; c.Live = true; c.Value = num(1) }),
		g(func(c *Component) { c.Target = nil }),
		g(func(c *Component) { c.Target = num(-1) }),
		g(func(c *Component) { c.Target = &Expr{Kind: ExprSetting, Setting: "title"} }),
		g(func(c *Component) { c.Since = "week" }),
		g(func(c *Component) { c.Metric = ""; c.Value = num(1); c.Since = "open" }),
		g(func(c *Component) { c.Step = &zero }),
		g(func(c *Component) { c.Step = &zero; c.Target = nil }),
		g(func(c *Component) { c.Metric = ""; c.Entity = "saving"; c.Field = "memo" }),
		g(func(c *Component) { c.Metric = ""; c.Entity = "saving"; c.Field = "amount"; c.Aggregate = "avg" }),
		g(func(c *Component) { c.Metric = ""; c.Entity = "nope"; c.Aggregate = "count" }),
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		"growth needs label",
		`growth kind "cactus" must be tree, flower or plant`,
		"growth needs one number source",
		"growth takes only one of metric, entity or value",
		"growth metric must be steps",
		"growth live is only for metric steps",
		"growth needs target > 0",
		"growth target must be > 0",
		`growth target {"setting":"title"} must be an integer, number or currency field`,
		`growth since "week" must be today or open`,
		"growth since open is only for metric steps",
		"growth step must be > 0",
		`growth field "memo" must be an integer, number or currency field`,
		"growth aggregate must be count or sum",
		`growth entity "nope" does not exist`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

// The schema rejects unknown kinds and accepts the example (歩数で木に実がなる).
func TestGrowthExampleAndSchema(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "steps_tree.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatalf("steps_tree.json: %v", err)
	}
	if probs := app.Lint(); len(probs) > 0 {
		t.Errorf("steps_tree.json lint: %v", probs)
	}
	c := app.Screens[0].Components[0]
	if c.Type != CompGrowth || c.Metric != "steps" || !c.Live || c.Kind != "tree" || c.Step == nil || *c.Step != 1000 {
		t.Errorf("growth did not round-trip: %+v", c)
	}
	bad := strings.Replace(string(raw), `"kind": "tree"`, `"kind": "cactus"`, 1)
	if _, err := Parse([]byte(bad)); err == nil {
		t.Error("schema accepted growth kind cactus")
	}
}
