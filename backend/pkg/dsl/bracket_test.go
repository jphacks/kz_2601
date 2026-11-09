package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func bracketApp() App {
	app := Initial()
	app.Entities = []Entity{
		{ID: "entrant", Name: "出場者", Fields: []Field{
			{ID: "name", Name: "名前", Type: FieldString, Required: true},
			{ID: "wins", Name: "勝ち数", Type: FieldInteger, DefaultValue: 0.0},
			{ID: "seed", Name: "シード", Type: FieldInteger},
			{ID: "rating", Name: "レート", Type: FieldNumber},
			{ID: "memo", Name: "メモ", Type: FieldString},
			{ID: "auto", Name: "自動", Type: FieldInteger, Computed: &Expr{Kind: ExprLiteral, Lit: 1.0}},
		}},
		{ID: "setting", Name: "設定", Single: true, Fields: []Field{
			{ID: "name", Name: "名前", Type: FieldString},
			{ID: "wins", Name: "勝ち数", Type: FieldInteger},
		}},
	}
	return app
}

func TestBracketValid(t *testing.T) {
	app := bracketApp()
	app.Screens[0].Components = []Component{
		{Type: CompBracket, Entity: "entrant", TitleField: "name", WinsField: "wins"},
		{Type: CompBracket, Entity: "entrant", TitleField: "name", WinsField: "wins", SeedField: "seed", Title: "卓球大会"},
		{Type: CompBracket, Entity: "entrant", TitleField: "name", WinsField: "wins", SeedField: "rating"},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("valid bracket rejected: %v", err)
	}
}

func TestBracketSemanticErrors(t *testing.T) {
	app := bracketApp()
	b := func(mod func(c *Component)) Component {
		c := Component{Type: CompBracket, Entity: "entrant", TitleField: "name", WinsField: "wins"}
		mod(&c)
		return c
	}
	app.Screens[0].Components = []Component{
		b(func(c *Component) { c.Entity = "nope" }),
		b(func(c *Component) { c.TitleField = "" }),
		b(func(c *Component) { c.TitleField = "zzz" }),
		b(func(c *Component) { c.WinsField = "" }),
		b(func(c *Component) { c.WinsField = "missing_wins" }),
		b(func(c *Component) { c.WinsField = "memo" }),
		b(func(c *Component) { c.WinsField = "auto" }),
		b(func(c *Component) { c.SeedField = "missing_seed" }),
		b(func(c *Component) { c.SeedField = "memo" }),
		b(func(c *Component) { c.SeedField = "wins" }),
		b(func(c *Component) { c.Entity = "setting" }),
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		`bracket entity "nope" does not exist`,
		`bracket needs titleField`,
		`bracket titleField "zzz" not in entity "entrant"`,
		`bracket needs winsField`,
		`bracket winsField "missing_wins" not in entity "entrant"`,
		`bracket winsField "memo" must be an integer field`,
		`bracket winsField "auto" is computed`,
		`bracket seedField "missing_seed" not in entity "entrant"`,
		`bracket seedField "memo" must be integer or number`,
		`bracket winsField and seedField are the same field "wins"`,
		`bracket entity "setting" is single`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

// The example app validates and lints clean: the bracket counts as the
// entrants' list (lint.go listTypes).
func TestBracketExample(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "tournament.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatalf("tournament.json: %v", err)
	}
	if probs := app.Lint(); len(probs) > 0 {
		t.Errorf("tournament.json lint: %v", probs)
	}
}
