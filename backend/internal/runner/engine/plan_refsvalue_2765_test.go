package engine

import (
	"reflect"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

func refsValueRow2765(data map[string]any) *expreval.Row {
	return &expreval.Row{
		Entity: &dsl.Entity{ID: "events", Name: "予定", Fields: []dsl.Field{
			{ID: "members", Name: "参加者", Type: dsl.FieldRefs, RefEntity: "people"},
			{ID: "title", Name: "タイトル", Type: dsl.FieldString},
		}},
		Record: expreval.Record{ID: "e1", Data: data},
	}
}

func TestRefsValueCopiesOnlyRefsFieldIDs(t *testing.T) {
	ref := func(id string) dsl.Expr { return dsl.Expr{Kind: dsl.ExprRef, Ref: id} }
	for _, c := range []struct {
		name string
		row  *expreval.Row
		x    dsl.Expr
		want any
	}{
		{"一つの参照", refsValueRow2765(map[string]any{"members": []any{"p1"}}), ref("members"), []any{"p1"}},
		{"複数の参照", refsValueRow2765(map[string]any{"members": []any{"p1", "p2", "p3"}}), ref("members"), []any{"p1", "p2", "p3"}},
		{"空の一覧", refsValueRow2765(map[string]any{"members": []any{}}), ref("members"), []any{}},
		{"空の文字と文字でない要素は落ちる", refsValueRow2765(map[string]any{"members": []any{"p1", "", 1.0, nil, "p2"}}), ref("members"), []any{"p1", "p2"}},
		{"値が無い", refsValueRow2765(map[string]any{}), ref("members"), nil},
		{"値が一覧でない", refsValueRow2765(map[string]any{"members": "p1"}), ref("members"), nil},
		{"参照でない式", refsValueRow2765(map[string]any{"members": []any{"p1"}}), dsl.Expr{Kind: dsl.ExprLiteral, Lit: "p1"}, nil},
		{"refs でない欄への参照", refsValueRow2765(map[string]any{"title": "合宿"}), ref("title"), nil},
		{"無い欄への参照", refsValueRow2765(map[string]any{"members": []any{"p1"}}), ref("unknown"), nil},
		{"記録のきっかけが無い", nil, ref("members"), nil},
	} {
		p := planner{row: c.row}
		if got := p.refsValue(c.x); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: refsValue = %#v, want %#v", c.name, got, c.want)
		}
	}
}
