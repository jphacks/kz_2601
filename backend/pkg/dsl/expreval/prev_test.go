package expreval

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// "$prev.<field>" reads Row.Prev (#341); without Prev it is nil as before.
func TestPrev(t *testing.T) {
	ents := []dsl.Entity{
		{ID: "events", Scope: dsl.ScopeShared, Fields: []dsl.Field{
			{ID: "title", Type: dsl.FieldString},
			{ID: "day", Type: dsl.FieldDate},
			{ID: "twice", Type: dsl.FieldInteger, Computed: &dsl.Expr{Kind: dsl.ExprLiteral, Lit: 2.0}},
		}},
		{ID: "rsvps", Scope: dsl.ScopeShared, Fields: []dsl.Field{
			{ID: "event", Type: dsl.FieldRef, RefEntity: "events"},
			{ID: "n", Type: dsl.FieldInteger},
		}},
	}
	src := SourceFunc(func(e string) []Record {
		if e == "events" {
			return []Record{{ID: "e1", Data: map[string]any{"title": "合宿"}}}
		}
		return nil
	})
	ev := New(Env{Entities: ents, Source: src})
	prev := &Record{ID: "r1", Data: map[string]any{"n": 1.0, "event": "e0"}}
	row := &Row{Entity: &ents[1], Record: Record{ID: "r1", Data: map[string]any{"n": 3.0, "event": "e1"}}, Prev: prev}
	ref := func(s string) dsl.Expr { return dsl.Expr{Kind: dsl.ExprRef, Ref: s} }
	cases := []struct {
		name string
		row  *Row
		ref  string
		want Value
	}{
		{"前の値", row, "$prev.n", NumberValue(1)},
		{"今の値", row, "n", NumberValue(3)},
		{"ref 先の lookup は今の値", row, "event.title", StringValue("合宿")},
		{"前の値が無い field", &Row{Entity: &ents[1], Record: row.Record, Prev: &Record{}}, "$prev.n", Value{}},
		{"Prev が無い", &Row{Entity: &ents[1], Record: row.Record}, "$prev.n", Value{}},
		{"知らない field", row, "$prev.zzz", Value{}},
		{"lookup の形は読まない", row, "$prev.event.title", Value{}},
		{"computed は保存されないので nil", &Row{Entity: &ents[0], Record: Record{}, Prev: &Record{}}, "$prev.twice", Value{}},
	}
	for _, c := range cases {
		if got := ev.Eval(ref(c.ref), c.row); got != c.want {
			t.Errorf("%s: %s = %+v, want %+v", c.name, c.ref, got, c.want)
		}
	}
	// A date field's previous value is a date.
	drow := &Row{Entity: &ents[0], Record: Record{}, Prev: &Record{Data: map[string]any{"day": "2026-11-06"}}}
	if got := ev.Eval(ref("$prev.day"), drow); got.Kind != Date {
		t.Errorf("$prev.day = %+v, want a date", got)
	}
}
