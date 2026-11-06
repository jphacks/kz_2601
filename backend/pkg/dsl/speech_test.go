package dsl

import (
	"strings"
	"testing"
)

func speechApp() App {
	app := memoApp()
	app.Entities[0].Fields = append(app.Entities[0].Fields,
		Field{ID: "body", Name: "本文", Type: FieldText},
		Field{ID: "photo", Name: "写真", Type: FieldPhoto},
		Field{ID: "voice", Name: "録音", Type: FieldAudio},
		Field{ID: "secret", Name: "秘密", Type: FieldString, Hidden: true},
		Field{ID: "len", Name: "長さ", Type: FieldInteger, Computed: &Expr{Kind: ExprLiteral, Lit: float64(1)}},
	)
	return app
}

func TestSpeechLeavesValid(t *testing.T) {
	app := speechApp()
	app.Screens[0].Components = []Component{
		{Type: CompSpeakButton, Text: "おはようございます", Label: "あいさつを聞く"},
		{Type: CompSpeakButton, Value: &Expr{Kind: ExprOp, Op: "concat", Args: []Expr{
			{Kind: ExprLiteral, Lit: "記録は "},
			{Kind: ExprAgg, Agg: &AggExpr{Agg: "count", Entity: "memo"}},
		}}},
	}
	app.Screens[1].Components = []Component{
		{Type: CompTranscribeField, Field: "body", Label: "議事録", Placeholder: "マイクを押して話してください"},
		{Type: CompTranscribeField, Field: "note"},
		{Type: CompSpeakButton, Field: "body"},
		{Type: CompSpeakButton, Field: "secret"}, // hidden from others like any field-bound leaf
		{Type: CompSpeakButton, Field: "len"},
		{Type: CompSpeakButton, Text: "{{note}} の本文は {{body}}"},
		{Type: CompSpeakButton, Value: &Expr{Kind: ExprRef, Ref: "body"}},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("valid speech leaves rejected: %v", err)
	}
}

func TestSpeechLeavesSemanticErrors(t *testing.T) {
	app := speechApp()
	app.Screens[0].Components = []Component{
		{Type: CompTranscribeField, Field: "body"},                        // needs screen.entity
		{Type: CompSpeakButton},                                           // no source
		{Type: CompSpeakButton, Text: "a", Field: "note"},                 // two sources
		{Type: CompSpeakButton, Field: "note"},                            // field needs screen.entity
		{Type: CompSpeakButton, Text: "{{note}}"},                         // placeholder needs screen.entity
		{Type: CompSpeakButton, Text: strings.Repeat("あ", SpeakMaxText+1)}, // too long
	}
	app.Screens[1].Components = []Component{
		{Type: CompTranscribeField, Field: "nope"},
		{Type: CompTranscribeField, Field: "elapsed"},
		{Type: CompTranscribeField, Field: "len"},
		{Type: CompSpeakButton, Field: "voice"},
		{Type: CompSpeakButton, Text: "{{zzz}}"},
		{Type: CompSpeakButton, Text: "{{secret}}"},
		{Type: CompSpeakButton, Value: &Expr{Kind: ExprRef, Ref: "secret"}},
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		`screen "home" components[0]: transcribeField requires screen.entity`,
		`screen "home" components[1]: speakButton takes exactly one of text`,
		`screen "home" components[2]: speakButton takes exactly one of text`,
		"speakButton.field requires screen.entity",
		"speakButton text {{note}} requires screen.entity",
		`screen "home" components[5]: speakButton text is too long`,
		`field "nope" not in entity "memo"`,
		`transcribeField field "elapsed" must be string or text`,
		`field "len" is computed`,
		`speakButton field "voice" is type audio`,
		`speakButton text {{zzz}} is not a field of entity "memo"`,
		`speakButton text {{secret}} is a hidden field`,
		"shows hidden field memo.secret",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

// The schema accepts only the shared keys.
func TestSpeechLeavesSchema(t *testing.T) {
	doc := func(comp string) []byte {
		return []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home",
"entities":[],"screens":[{"id":"home","title":"h","components":[` + comp + `]}]}`)
	}
	if _, err := Parse(doc(`{"type":"speakButton","text":"こんにちは","label":"聞く","area":"A1:C2"}`)); err != nil {
		t.Fatalf("valid speakButton: %v", err)
	}
	if _, err := Parse(doc(`{"type":"speakButton","text":"x","rate":2}`)); err == nil {
		t.Error("unknown key rate should fail the schema")
	}
	if _, err := Parse(doc(`{"type":"transcribeField","label":"x"}`)); err == nil {
		t.Error("transcribeField without field should fail the schema")
	}
	if _, err := Parse(doc(`{"type":"transcribeField","field":"body","append":true}`)); err == nil {
		t.Error("unknown key append should fail the schema")
	}
}
