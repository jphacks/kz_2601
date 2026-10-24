package dsl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustExpr(t *testing.T, s string) *Expr {
	t.Helper()
	var e Expr
	if err := json.Unmarshal([]byte(s), &e); err != nil {
		t.Fatalf("unmarshal %s: %v", s, err)
	}
	return &e
}

// warikanApp loads docs/dsl/examples/warikan.json as the base for mutations.
func warikanApp(t *testing.T) App {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "warikan.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatalf("warikan example invalid: %v", err)
	}
	return *app
}

func TestExprRoundTrip(t *testing.T) {
	for _, s := range []string{
		`12.5`, `"a"`, `true`,
		`{"ref":"id"}`,
		`{"agg":"count","entity":"members"}`,
		`{"agg":"sum","entity":"e","field":"x","filter":{"field":"p","op":"eq","value":{"ref":"id"}}}`,
		`{"agg":"sum","entity":"e","field":"x","filter":[{"field":"p","op":"eq","value":1},{"field":"d","op":"thisMonth","value":null}]}`,
		`{"op":"today","args":[]}`,
		`{"op":"add","args":[1,2,3]}`,
		`{"if":{"op":"gt","args":[{"ref":"a"},0]},"then":"+","else":"-"}`,
	} {
		e := mustExpr(t, s)
		out, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		var a, b any
		_ = json.Unmarshal([]byte(s), &a)
		_ = json.Unmarshal(out, &b)
		if !jsonEqual(a, b) {
			t.Errorf("round-trip %s -> %s", s, out)
		}
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func TestExprUnmarshalRejects(t *testing.T) {
	for _, s := range []string{`null`, `[1,2]`, `{"foo":1}`, `{"ref":"a","op":"add"}`, `{"if":true,"then":1}`} {
		var e Expr
		if err := json.Unmarshal([]byte(s), &e); err == nil {
			t.Errorf("%s: expected error", s)
		}
	}
}

func TestWarikanExampleValid(t *testing.T) {
	app := warikanApp(t)
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestExprSemanticErrors(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(a *App)
		want   string
	}{
		{"unknown op", func(a *App) {
			a.Entities[0].Fields[3].Computed = &Expr{Kind: ExprOp, Op: "median", Args: []Expr{{Kind: ExprRef, Ref: "paid"}}}
		}, `unknown op "median"`},
		{"arity", func(a *App) {
			a.Entities[0].Fields[3].Computed = &Expr{Kind: ExprOp, Op: "sub", Args: []Expr{{Kind: ExprRef, Ref: "paid"}}}
		}, "op sub takes exactly 2 args (got 1)"},
		{"variadic min", func(a *App) {
			a.Entities[0].Fields[3].Computed = &Expr{Kind: ExprOp, Op: "add", Args: []Expr{*NumExpr(1)}}
		}, "op add needs at least 2 args"},
		{"bad ref", func(a *App) {
			a.Entities[0].Fields[3].Computed = &Expr{Kind: ExprRef, Ref: "nope"}
		}, `ref "nope" is not a field of entity "members"`},
		{"agg bad entity", func(a *App) {
			a.Entities[0].Fields[1].Computed.Agg.Entity = "ghost"
		}, `entity "ghost" does not exist`},
		{"agg bad field", func(a *App) {
			a.Entities[0].Fields[1].Computed.Agg.Field = "zzz"
		}, `field "zzz" not in entity "expenses"`},
		{"agg bad filter field", func(a *App) {
			a.Entities[0].Fields[1].Computed.Agg.Filter[0].Field = "who"
		}, `field "who" not in entity "expenses"`},
		{"agg sum on string", func(a *App) {
			a.Entities[0].Fields[1].Computed.Agg.Field = "title"
		}, "sum needs a numeric field"},
		{"cycle", func(a *App) {
			a.Entities[0].Fields[1].Computed = &Expr{Kind: ExprRef, Ref: "balance"}
		}, "computed field cycle: members.balance -> members.paid -> members.balance"},
		{"cycle via agg", func(a *App) {
			a.Entities[0].Fields[2].Computed = &Expr{Kind: ExprAgg, Agg: &AggExpr{Agg: "sum", Entity: "members", Field: "balance"}}
		}, "computed field cycle"},
		{"depth", func(a *App) {
			e := NumExpr(1)
			for range 9 {
				e = &Expr{Kind: ExprOp, Op: "neg", Args: []Expr{*e}}
			}
			a.Entities[0].Fields[3].Computed = e
		}, "nested deeper than 8"},
		{"type mismatch", func(a *App) {
			a.Entities[0].Fields[3].Computed = &Expr{Kind: ExprOp, Op: "sub", Args: []Expr{{Kind: ExprRef, Ref: "name"}, *NumExpr(1)}}
		}, "sub expects a number but got a string"},
		{"declared type mismatch", func(a *App) {
			a.Entities[0].Fields[3].Computed = &Expr{Kind: ExprRef, Ref: "name"}
		}, "yields a string but the field type is currency"},
		{"computed bad type", func(a *App) {
			a.Entities[0].Fields[3].Type = FieldPhoto
		}, "computed field type must be one of"},
		{"computed required", func(a *App) {
			a.Entities[0].Fields[3].Required = true
		}, "remove required"},
		{"ref needs refEntity", func(a *App) {
			a.Entities[1].Fields[2].RefEntity = ""
		}, "type ref needs refEntity"},
		{"refEntity missing", func(a *App) {
			a.Entities[1].Fields[2].RefEntity = "people"
		}, `refEntity "people" does not exist`},
		{"refEntity on non-ref", func(a *App) {
			a.Entities[1].Fields[0].RefEntity = "members"
		}, "refEntity is only allowed on type ref"},
		{"stat both", func(a *App) {
			a.Screens[0].Components[0].Children[1].Aggregate = "sum"
		}, "either aggregate or value, not both"},
		{"stat neither", func(a *App) {
			a.Screens[0].Components[0].Children[1].Value = nil
		}, "stat needs aggregate (with entity) or value"},
		{"stat value ref", func(a *App) {
			a.Screens[0].Components[0].Children[1].Value = &Expr{Kind: ExprRef, Ref: "amount"}
		}, `{"ref":"amount"} is not allowed here`},
		{"stat value stray entity", func(a *App) {
			a.Screens[0].Components[0].Children[1].Entity = "expenses"
		}, "stat with value must not set entity/field/filter"},
		{"settlement balance not numeric", func(a *App) {
			a.Screens[0].Components[2].BalanceField = "name"
		}, "balanceField \"name\" must be number/integer/currency"},
		{"settlement bad entity", func(a *App) {
			a.Screens[0].Components[2].Entity = "ghost"
		}, `settlement entity "ghost" does not exist`},
		{"settlement bad name", func(a *App) {
			a.Screens[0].Components[2].NameField = "nick"
		}, `settlement nameField "nick"`},
		{"input on computed", func(a *App) {
			a.Screens[2].Components[0].Children = append(a.Screens[2].Components[0].Children, Component{Type: CompNumberField, Field: "paid"})
		}, `field "paid" is computed (read-only)`},
		{"textField on ref", func(a *App) {
			a.Screens[1].Components[0].Children[2].Type = CompTextField
		}, "is type ref; use a picker"},
		{"list filter ref", func(a *App) {
			a.Screens[0].Components[3].Children[0].Filter = &Filter{Field: "payer", Op: "eq", Value: map[string]any{"ref": "id"}}
		}, `{"ref":"id"} is not allowed here`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := warikanApp(t)
			tc.mutate(&app)
			expectProblem(t, app, tc.want)
		})
	}
}

func TestExprFilterValueOuterRow(t *testing.T) {
	// payer eq ref id: ref resolves against the outer record (members), so
	// referencing a members-only field there is fine, an unknown one is not.
	app := warikanApp(t)
	app.Entities[0].Fields[1].Computed.Agg.Filter[0].Value = map[string]any{"ref": "name"}
	if err := app.Validate(); err != nil {
		t.Fatalf("outer-row ref should be valid: %v", err)
	}
	app.Entities[0].Fields[1].Computed.Agg.Filter[0].Value = map[string]any{"ref": "amount"}
	ps := problems(t, app)
	if len(ps) == 0 || !strings.Contains(strings.Join(ps, ";"), `ref "amount" is not a field of entity "members"`) {
		t.Fatalf("expected outer-row ref error, got %v", ps)
	}
}

// A schema error inside an Expr reports the best-matching branch only, not
// one line per Expr shape (the list goes back to Claude verbatim).
func TestSchemaErrorInExprIsConcise(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "warikan.json"))
	if err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(string(raw), `"op": "sub"`, `"op": "minus"`, 1)
	_, perr := Parse([]byte(bad))
	ve, ok := perr.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", perr)
	}
	if len(ve.Problems) != 1 || !strings.Contains(ve.Problems[0], "/computed/op: value must be one of") {
		t.Fatalf("want one op problem, got %v", ve.Problems)
	}
}

func TestPickerAcceptsRef(t *testing.T) {
	app := warikanApp(t)
	if ps := problems(t, app); len(ps) != 0 {
		t.Fatal(ps)
	}
}
