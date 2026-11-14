package dsl

import (
	"strings"
	"testing"
)

func highlightApp() App {
	app := memoApp()
	app.Entities[0].Fields = append(app.Entities[0].Fields,
		Field{ID: "body", Name: "本文", Type: FieldText},
		Field{ID: "keywords", Name: "重要語", Type: FieldString},
		Field{ID: "photo", Name: "写真", Type: FieldPhoto},
		Field{ID: "secret", Name: "秘密", Type: FieldString, Hidden: true},
	)
	return app
}

func TestTextHighlightValid(t *testing.T) {
	app := highlightApp()
	app.Screens[0].Components = []Component{
		{Type: CompText, Text: "注意: 期限と金額を確かめる", Highlight: &TextHighlight{Words: []string{"期限", "金額"}}},
		{Type: CompText, Text: "{ literal } braces stay"},
	}
	app.Screens[1].Components = []Component{
		{Type: CompText, Text: "{{body}}", Highlight: &TextHighlight{Field: "keywords"}},
		{Type: CompText, Text: "日付 {{date}}・{{note}}", Highlight: &TextHighlight{Field: "body", Style: "bold"}},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("valid text.highlight rejected: %v", err)
	}
}

func TestTextHighlightSemanticErrors(t *testing.T) {
	app := highlightApp()
	many := make([]string, TextHighlightMaxWords+1)
	for i := range many {
		many[i] = "w"
	}
	app.Screens[0].Components = []Component{
		{Type: CompText, Text: "x", Highlight: &TextHighlight{Field: "keywords"}},    // needs screen.entity
		{Type: CompText, Text: "x", Highlight: &TextHighlight{}},                     // no source
		{Type: CompText, Text: "x", Highlight: &TextHighlight{Words: many}},          // too many
		{Type: CompText, Text: "x", Highlight: &TextHighlight{Words: []string{" "}}}, // blank word
		{Type: CompText, Text: "x", Highlight: &TextHighlight{Words: []string{"a"}, Style: "neon"}},
	}
	app.Screens[1].Components = []Component{
		{Type: CompText, Text: "x", Highlight: &TextHighlight{Words: []string{"a"}, Field: "keywords"}},
		{Type: CompText, Text: "x", Highlight: &TextHighlight{Field: "date"}},
		{Type: CompText, Text: "x", Highlight: &TextHighlight{Field: "secret"}},
		{Type: CompText, Text: "x", Highlight: &TextHighlight{Field: "nope"}},
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		`screen "home" components[0]: text.highlight.field requires screen.entity`,
		`screen "home" components[1]: text.highlight takes exactly one of words`,
		`screen "home" components[2]: text.highlight.words has 21 words`,
		`screen "home" components[3]: text.highlight.words[0] must be`,
		`screen "home" components[4]: text.highlight.style "neon"`,
		`screen "form" components[0]: text.highlight takes exactly one of words`,
		`text.highlight.field "date" is type datetime`,
		`text.highlight.field "secret" is hidden`,
		`text.highlight.field "nope" is not a field`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

// A saved app with {{x}} in a text on a screen without screen.entity still
// validates: there the text is shown as written, as before text.highlight.
func TestTextPlaceholderWithoutEntityStaysValid(t *testing.T) {
	raw := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home",
"entities":[{"id":"memo","name":"m","fields":[{"id":"body","name":"b","type":"text"}]}],
"screens":[{"id":"home","title":"h","components":[
 {"type":"text","text":"{{name}} さん、こんにちは"},
 {"type":"text","text":"テンプレ: {{body}} / {{ }}"},
 {"type":"list","entity":"memo","item":{"title":"body"}}]}]}`)
	if _, err := Parse(raw); err != nil {
		t.Fatalf("existing app with literal {{x}} rejected: %v", err)
	}
}

// On a screen.entity screen, {{x}} of an unknown, hidden or non-scalar field
// is not an error either: it stays literal (TextSubstitutes is false).
func TestTextPlaceholderOnEntityScreenNeverFails(t *testing.T) {
	app := highlightApp()
	app.Screens[1].Components = []Component{
		{Type: CompText, Text: "{{zzz}}"},
		{Type: CompText, Text: "{{photo}}"},
		{Type: CompText, Text: "{{secret}}"},
		{Type: CompText, Text: "{{ }} {{body}}"},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("literal {{x}} rejected: %v", err)
	}
	e := &app.Entities[0]
	for id, want := range map[string]bool{"body": true, "keywords": true, "date": true, "zzz": false, "photo": false, "secret": false, " ": false} {
		if got := TextSubstitutes(e, id); got != want {
			t.Errorf("TextSubstitutes(%q) = %v, want %v", id, got, want)
		}
	}
	if TextSubstitutes(nil, "body") {
		t.Error("no screen.entity: never substitutes")
	}
}

// The schema accepts highlight on text only and caps the fixed words.
func TestTextHighlightSchema(t *testing.T) {
	doc := func(comp string) []byte {
		return []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home",
"entities":[],"screens":[{"id":"home","title":"h","components":[` + comp + `]}]}`)
	}
	if _, err := Parse(doc(`{"type":"text","text":"期限は明日","highlight":{"words":["期限"],"style":"marker"}}`)); err != nil {
		t.Fatalf("valid highlight: %v", err)
	}
	if _, err := Parse(doc(`{"type":"label","text":"x","icon":"star","highlight":{"words":["x"]}}`)); err == nil {
		t.Error("highlight on label should fail the schema")
	}
	if _, err := Parse(doc(`{"type":"text","text":"x","highlight":{"words":["` + strings.Repeat("a", TextHighlightMaxWordLen+1) + `"]}}`)); err == nil {
		t.Error("word over the cap should fail")
	}
	if _, err := Parse(doc(`{"type":"text","text":"x","highlight":{"words":["a"],"color":"red"}}`)); err == nil {
		t.Error("unknown highlight key should fail")
	}
}
