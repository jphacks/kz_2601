package dsl

import (
	"strings"
	"testing"
)

func qrApp() App {
	app := memoApp()
	app.Entities[0].Fields = append(app.Entities[0].Fields,
		Field{ID: "photo", Name: "写真", Type: FieldPhoto},
		Field{ID: "secret", Name: "秘密", Type: FieldString, Hidden: true},
	)
	return app
}

func TestQRCodeValid(t *testing.T) {
	app := qrApp()
	app.Screens[0].Components = []Component{
		{Type: CompQrCode, Text: "https://example.com", Label: "公式サイト", Title: "QR"},
		{Type: CompQrCode, Value: &Expr{Kind: ExprOp, Op: "concat", Args: []Expr{
			{Kind: ExprLiteral, Lit: "件数 "},
			{Kind: ExprAgg, Agg: &AggExpr{Agg: "count", Entity: "memo"}},
		}}},
	}
	app.Screens[1].Components = []Component{
		{Type: CompQrCode, Field: "note"},
		{Type: CompQrCode, Field: "elapsed"},
		{Type: CompQrCode, Text: "https://x.com/{{note}}"},
		{Type: CompQrCode, Value: &Expr{Kind: ExprOp, Op: "concat", Args: []Expr{
			{Kind: ExprLiteral, Lit: "MEMBER:"}, {Kind: ExprRef, Ref: "note"},
		}}},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("valid qrCode rejected: %v", err)
	}
}

func TestQRCodeSemanticErrors(t *testing.T) {
	app := qrApp()
	app.Screens[0].Components = []Component{
		{Type: CompQrCode},                                           // no source
		{Type: CompQrCode, Text: "a", Field: "note"},                 // two sources
		{Type: CompQrCode, Field: "note"},                            // field needs screen.entity
		{Type: CompQrCode, Text: "id={{note}}"},                      // placeholder needs screen.entity
		{Type: CompQrCode, Value: &Expr{Kind: ExprRef, Ref: "note"}}, // ref without a row
		{Type: CompQrCode, Text: strings.Repeat("a", QRCodeMaxText+1)},
		{Type: CompQrCode, Text: strings.Repeat("あ", 800)}, // 2400 bytes > capacity
	}
	app.Screens[1].Components = []Component{
		{Type: CompQrCode, Field: "photo"},
		{Type: CompQrCode, Field: "nope"},
		{Type: CompQrCode, Field: "secret"},
		{Type: CompQrCode, Text: "{{zzz}}"},
		{Type: CompQrCode, Value: &Expr{Kind: ExprRef, Ref: "secret"}},
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		`screen "home" components[0]: qrCode takes exactly one of text`,
		`screen "home" components[1]: qrCode takes exactly one of text`,
		"qrCode.field requires screen.entity",
		"qrCode text {{note}} requires screen.entity",
		`screen "home" components[4].value`,
		`screen "home" components[5]: qrCode text is too long`,
		`screen "home" components[6]: qrCode text is too long`,
		`qrCode field "photo" is type photo`,
		`field "nope" not in entity "memo"`,
		`qrCode field "secret" is hidden`,
		`qrCode text {{zzz}} is not a field of entity "memo"`,
		"shows hidden field memo.secret",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

// The schema accepts the shared keys and caps the fixed text.
func TestQRCodeSchema(t *testing.T) {
	doc := func(comp string) []byte {
		return []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home",
"entities":[],"screens":[{"id":"home","title":"h","components":[` + comp + `]}]}`)
	}
	if _, err := Parse(doc(`{"type":"qrCode","text":"https://example.com","label":"見せてください","title":"会員証","area":"B3:E8"}`)); err != nil {
		t.Fatalf("valid qrCode: %v", err)
	}
	if _, err := Parse(doc(`{"type":"qrCode","text":"x","entity":"memo"}`)); err == nil {
		t.Error("unknown key entity should fail the schema")
	}
	if _, err := Parse(doc(`{"type":"qrCode","text":"` + strings.Repeat("a", QRCodeMaxText+1) + `"}`)); err == nil {
		t.Error("text over the cap should fail")
	}
}
