package expreval

import (
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Evaluate is the one-shot entry: same answers as New(env).Eval.
func TestEvaluate(t *testing.T) {
	ents := []dsl.Entity{{ID: "items", Fields: []dsl.Field{
		{ID: "price", Type: dsl.FieldNumber},
		{ID: "qty", Type: dsl.FieldInteger},
		{ID: "name", Type: dsl.FieldString},
	}}}
	row := &Row{Entity: &ents[0], Record: Record{ID: "r1", Data: map[string]any{"price": 120.0, "qty": 3.0, "name": "りんご"}}}
	lit := func(v any) dsl.Expr { return dsl.Expr{Kind: dsl.ExprLiteral, Lit: v} }
	ref := func(s string) dsl.Expr { return dsl.Expr{Kind: dsl.ExprRef, Ref: s} }
	op := func(o string, args ...dsl.Expr) dsl.Expr { return dsl.Expr{Kind: dsl.ExprOp, Op: o, Args: args} }
	env := Env{Entities: ents}
	cases := []struct {
		name string
		e    dsl.Expr
		row  *Row
		want Value
	}{
		{"number", lit(1.5), nil, NumberValue(1.5)},
		{"string", lit("こんにちは"), nil, StringValue("こんにちは")},
		{"bool", lit(true), nil, BoolValue(true)},
		{"row field", ref("name"), row, StringValue("りんご")},
		{"row fields in an op", op("mul", ref("price"), ref("qty")), row, NumberValue(360)},
		{"unknown field", ref("zzz"), row, Value{}},
		{"field without a row", ref("price"), nil, Value{}},
		{"division by zero", op("div", lit(1.0), lit(0.0)), nil, Value{}},
	}
	for _, c := range cases {
		if got := Evaluate(c.e, env, c.row); got != c.want {
			t.Errorf("%s: Evaluate = %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestRecordJSON(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	ny := time.FixedZone("EST", -5*60*60)
	// 2026-11-06 23:30 JST = 2026-11-06 14:30 UTC = 2026-11-06 09:30 EST
	late := DateValue(time.Date(2026, 11, 6, 23, 30, 0, 0, jst))
	// 2026-11-07 08:00 JST = 2026-11-06 23:00 UTC: the day differs by zone
	early := DateValue(time.Date(2026, 11, 7, 8, 0, 0, 0, jst))
	cases := []struct {
		name     string
		v        Value
		datetime bool
		loc      *time.Location
		want     any
	}{
		{"number", NumberValue(42.5), false, nil, 42.5},
		{"string", StringValue("メモ"), false, nil, "メモ"},
		{"empty string", StringValue(""), false, nil, ""},
		{"bool", BoolValue(false), false, nil, false},
		{"nil", Value{}, false, jst, nil},
		{"nil datetime", Value{}, true, jst, nil},
		{"date in the zone", early, false, jst, "2026-11-07"},
		{"date without zone is UTC", early, false, nil, "2026-11-06"},
		{"date in another zone", late, false, ny, "2026-11-06"},
		{"datetime is UTC whatever the zone", early, true, jst, "2026-11-06T23:00:00Z"},
		{"datetime drops sub-seconds", DateValue(time.Date(2026, 1, 2, 3, 4, 5, 999, time.UTC)), true, ny, "2026-01-02T03:04:05Z"},
	}
	for _, c := range cases {
		if got := c.v.RecordJSON(c.datetime, c.loc); got != c.want {
			t.Errorf("%s: RecordJSON = %#v, want %#v", c.name, got, c.want)
		}
	}
}
