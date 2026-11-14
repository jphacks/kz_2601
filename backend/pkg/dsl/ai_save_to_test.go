package dsl

import (
	"strings"
	"testing"
)

func saveToApp(comps ...Component) App {
	app := aiApp(comps...)
	app.Entities = append(app.Entities,
		Entity{ID: "card", Name: "カード", TitleField: "q", Fields: []Field{
			{ID: "q", Name: "問題", Type: FieldString, Required: true},
			{ID: "note", Name: "メモ", Type: FieldText},
			{ID: "learned", Name: "覚えた", Type: FieldBoolean, Required: true},
			{ID: "n", Name: "点", Type: FieldInteger},
			{ID: "fixed", Name: "固定", Type: FieldString, Computed: &Expr{Kind: ExprLiteral, Lit: "x"}},
		}},
		Entity{ID: "wall", Name: "掲示", Scope: ScopePublic, Fields: []Field{{ID: "t", Name: "t", Type: FieldString}}},
		Entity{ID: "prefs", Name: "設定", Single: true, Fields: []Field{{ID: "t", Name: "t", Type: FieldString}}},
		Entity{ID: "board", Name: "共有", Scope: ScopeShared, Fields: []Field{
			{ID: "t", Name: "t", Type: FieldString},
			{ID: "pin", Name: "固定", Type: FieldString, EditableBy: RoleAdmins},
		}},
		Entity{ID: "strict", Name: "必須", Fields: []Field{
			{ID: "t", Name: "t", Type: FieldString},
			{ID: "who", Name: "誰", Type: FieldString, Required: true},
		}},
	)
	return app
}

func TestAISaveToValidPasses(t *testing.T) {
	app := saveToApp(
		Component{Type: CompAiButton, Task: "generate", Prompt: "英単語の問題", Shape: "list", Count: 5, SaveTo: &AISaveTo{Entity: "card", Field: "q"}},
		Component{Type: CompAiButton, ID: "b", Task: "generate", Prompt: "アイデア", Shape: "list", SaveTo: &AISaveTo{Entity: "strict", Field: "who"}},
		Component{Type: CompAiButton, ID: "c", Task: "generate", Prompt: "{{input}} の続き", Entity: "card", Shape: "list", SaveTo: &AISaveTo{Entity: "board", Field: "t"}},
	)
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAISaveToSemanticErrors(t *testing.T) {
	gen := func(id string, s *AISaveTo) Component {
		return Component{Type: CompAiButton, ID: id, Task: "generate", Prompt: "x", Shape: "list", SaveTo: s}
	}
	app := saveToApp(
		Component{Type: CompAiButton, Task: "generate", Prompt: "x", SaveTo: &AISaveTo{Entity: "card", Field: "q"}},
		Component{Type: CompAiButton, ID: "s", Task: "summarize", Input: "body", Output: "summary", SaveTo: &AISaveTo{Entity: "card", Field: "q"}},
		Component{Type: CompAiButton, ID: "o", Task: "generate", Prompt: "x", Shape: "list", Output: "summary", SaveTo: &AISaveTo{Entity: "card", Field: "q"}},
		gen("a", &AISaveTo{Entity: "nope", Field: "q"}),
		gen("b", &AISaveTo{Entity: "wall", Field: "t"}),
		gen("c", &AISaveTo{Entity: "prefs", Field: "t"}),
		gen("d", &AISaveTo{Entity: "card", Field: "zz"}),
		gen("e", &AISaveTo{Entity: "card", Field: "n"}),
		gen("f", &AISaveTo{Entity: "card", Field: "fixed"}),
		gen("g", &AISaveTo{Entity: "board", Field: "pin"}),
		gen("h", &AISaveTo{Entity: "strict", Field: "t"}),
		gen("i", &AISaveTo{Entity: "card", Field: "note"}),
	)
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, w := range []string{
		"saveTo is only for task generate with shape list",
		"saveTo and output are exclusive",
		`saveTo entity "nope" does not exist`,
		`cannot add to public entity "wall"`,
		`cannot add to single entity "prefs"`,
		`saveTo field "zz" not in entity "card"`,
		`saveTo field "n" must be string or text (got integer)`,
		`saveTo field "fixed" is computed`,
		`saveTo field "pin" is editableBy admins`,
		`required field "who" of entity "strict" would stay empty`,
		`required field "q" of entity "card" would stay empty`,
	} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing %q in\n%s", w, joined)
		}
	}
	if strings.Contains(joined, `required field "learned"`) {
		t.Errorf("boolean fills itself:\n%s", joined)
	}
}

func TestAISaveToSchema(t *testing.T) {
	doc := func(comp string) []byte {
		return []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home",
"entities":[{"id":"card","name":"カード","fields":[{"id":"q","name":"問題","type":"string"}]}],
"screens":[{"id":"home","title":"h","components":[` + comp + `]}]}`)
	}
	a, err := Parse(doc(`{"type":"aiButton","task":"generate","prompt":"問題","shape":"list","count":5,"saveTo":{"entity":"card","field":"q"},"area":"A1:F6"}`))
	if err != nil {
		t.Fatalf("valid saveTo: %v", err)
	}
	if s := a.Screens[0].Components[0].SaveTo; s == nil || s.Entity != "card" || s.Field != "q" {
		t.Fatalf("decoded %+v", a.Screens[0].Components[0])
	}
	if _, err := Parse(doc(`{"type":"aiButton","task":"generate","prompt":"x","shape":"list","saveTo":{"entity":"card"}}`)); err == nil {
		t.Error("saveTo without field should fail the schema")
	}
	if _, err := Parse(doc(`{"type":"aiButton","task":"generate","prompt":"x","shape":"list","saveTo":{"entity":"card","field":"q","url":"x"}}`)); err == nil {
		t.Error("saveTo extra key should fail the schema")
	}
}
