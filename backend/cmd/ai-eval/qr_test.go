package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckQR(t *testing.T) {
	ref := &dsl.Expr{Kind: dsl.ExprOp, Op: "concat", Args: []dsl.Expr{{Kind: dsl.ExprLiteral, Lit: "M:"}, {Kind: dsl.ExprRef, Ref: "number"}}}
	app := &dsl.App{Screens: []dsl.Screen{
		{ID: "home", Components: []dsl.Component{{Type: dsl.CompQrCode, Text: "https://cafe.example.jp"}}},
		{ID: "form", Entity: "card", Components: []dsl.Component{{Type: dsl.CompSection, Children: []dsl.Component{
			{Type: dsl.CompQrCode, Value: ref},
		}}}},
	}}
	for _, want := range []string{"text", "value", "record", "any"} {
		if miss := checkQR(map[string]any{"qrSource": want}, app); miss != nil {
			t.Errorf("qrSource=%s: %v", want, miss)
		}
	}
	if miss := checkQR(map[string]any{"qrSource": "field"}, app); miss == nil {
		t.Error("no qrCode uses field")
	}
	if miss := checkQR(map[string]any{}, app); miss != nil {
		t.Errorf("no expect key: %v", miss)
	}

	fixed := &dsl.App{Screens: []dsl.Screen{{ID: "home", Components: []dsl.Component{{Type: dsl.CompQrCode, Text: "https://x.com/example"}}}}}
	if miss := checkQR(map[string]any{"qrSource": "record"}, fixed); miss == nil {
		t.Error("a fixed example URL must not count as the record")
	}
	if miss := checkQR(map[string]any{"qrSource": "any"}, &dsl.App{}); miss == nil {
		t.Error("no qrCode at all should miss")
	}
	if bad := validateExpect(map[string]any{"qrSource": "photo"}); len(bad) != 1 {
		t.Errorf("unknown qrSource value should be reported: %v", bad)
	}
}
