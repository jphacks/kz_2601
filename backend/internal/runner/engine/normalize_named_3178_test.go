package engine

import (
	"math"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

func TestNormalizeNumber_3178(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   any
		want any
	}{
		{"int", 3, 3.0},
		{"int64", int64(4), 4.0},
		{"float32", float32(2.5), 2.5},
		{"float64 はそのまま", 1.5, 1.5},
		{"文字はそのまま", "x", "x"},
		{"真偽はそのまま", true, true},
		{"nil はそのまま", nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := normalize(tc.in)
			if got != tc.want {
				t.Errorf("normalize(%#v) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

func TestPlannerNamed_3178(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 6, 20, 0, 0, 0, time.UTC) // JST では 11-07
	lit := func(v any) dsl.Expr { return dsl.Expr{Kind: dsl.ExprLiteral, Lit: v} }
	tests := []struct {
		name          string
		in            dsl.Expr
		wantText      string
		wantFormatted bool
	}{
		{"日付は書式済みで true", dsl.Expr{Kind: dsl.ExprOp, Op: "today"}, "2026-11-07", true},
		{"整数は文字", lit(3.0), "3", false},
		{"小数は文字", lit(2.5), "2.5", false},
		{"NaN は空で false", lit(math.NaN()), "", false},
		{"+Inf は空で false", lit(math.Inf(1)), "", false},
		{"-Inf は空で false", lit(math.Inf(-1)), "", false},
		{"文字は使えない", lit("abc"), "", false},
		{"真偽は使えない", lit(true), "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := planner{e: expreval.New(expreval.Env{Now: now, Location: tokyo}), loc: tokyo}
			text, formatted := p.named(tc.in)
			if text != tc.wantText || formatted != tc.wantFormatted {
				t.Errorf("named() = (%q, %v), want (%q, %v)", text, formatted, tc.wantText, tc.wantFormatted)
			}
		})
	}
}
