package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func aiChatApp() App {
	app := memoApp()
	app.Entities[0].Fields = append(app.Entities[0].Fields,
		Field{ID: "role", Name: "役", Type: FieldEnum, Options: []string{"user", "assistant"}},
		Field{ID: "who", Name: "誰", Type: FieldString},
		Field{ID: "bad", Name: "x", Type: FieldEnum, Options: []string{"me", "ai"}},
	)
	return app
}

func TestAIChatSemanticErrors(t *testing.T) {
	app := aiChatApp()
	app.Entities[0].Fields = append(app.Entities[0].Fields, Field{ID: "must", Name: "x", Type: FieldString, Required: true})
	p := "やさしい相談相手"
	app.Screens[0].Components = []Component{
		{Type: CompAiChat, Entity: "memo", TextField: "note", RoleField: "role", Persona: p}, // ok except "must"
		{Type: CompAiChat, Entity: "memo", TextField: "elapsed", Persona: p},                 // not text
		{Type: CompAiChat, Entity: "memo", TextField: "gone", Persona: p},
		{Type: CompAiChat, Entity: "memo", TextField: "note", RoleField: "bad", Persona: p},
		{Type: CompAiChat, Entity: "memo", TextField: "note", RoleField: "note", Persona: p},
		{Type: CompAiChat, Entity: "memo", TextField: "note", RoleField: "elapsed", Persona: p},
		{Type: CompAiChat, Entity: "memo", TextField: "note"},
		{Type: CompAiChat, Entity: "memo", TextField: "note", Persona: strings.Repeat("あ", 501)},
		{Type: CompAiChat, Entity: "memo", TextField: "note", Persona: p, Length: "huge"},
		{Type: CompAiChat, Entity: "nope", TextField: "note", Persona: p},
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		`aiChat saves turns without a form, so required field "must" of entity "memo" needs a defaultValue`,
		`aiChat textField "elapsed" must be a stored string or text field`,
		`aiChat textField "gone" not in entity "memo"`,
		`aiChat roleField "bad" is an enum, so its options must include user and assistant`,
		`aiChat roleField and textField must differ`,
		`aiChat roleField "elapsed" must be a stored string or enum field`,
		`aiChat needs persona`,
		`aiChat persona must be at most 500 characters`,
		`aiChat length "huge" must be short|long`,
		`aiChat entity "nope" does not exist`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

func TestAIChatValid(t *testing.T) {
	for _, c := range []Component{
		{Type: CompAiChat, Area: "A1:F12", Entity: "memo", TextField: "note", RoleField: "role", Persona: "家庭教師", Length: "long"},
		{Type: CompAiChat, Area: "A1:F12", Entity: "memo", TextField: "note", RoleField: "who", Persona: "英会話の相手"},
		{Type: CompAiChat, Area: "A1:F12", Entity: "memo", TextField: "note", Persona: "相談相手"}, // roleField omitted: alternating
	} {
		app := aiChatApp()
		app.Screens[0].Components = []Component{c}
		if err := app.Validate(); err != nil {
			t.Fatalf("%+v: %v", c, err)
		}
	}
}

// The conversation the AI reads is only the user's own: private passes,
// shared / public is an error.
func TestAIChatEntityMustBePrivate(t *testing.T) {
	for scope, ok := range map[EntityScope]bool{"": true, ScopePrivate: true, ScopeShared: false, ScopePublic: false} {
		app := aiChatApp()
		app.Entities[0].Scope = scope
		app.Screens[0].Components = []Component{{Type: CompAiChat, Area: "A1:F12", Entity: "memo", TextField: "note", RoleField: "role", Persona: "相談相手"}}
		err := app.Validate()
		if ok && err != nil {
			t.Fatalf("scope %q: %v", scope, err)
		}
		if !ok && (err == nil || !strings.Contains(err.Error(), `aiChat entity "memo" must be private`)) {
			t.Fatalf("scope %q: want private error, got %v", scope, err)
		}
	}
}

// The example passes the schema and lint (the entity needs no form or list);
// a shared / public conversation entity is a validation error.
func TestAIChatExampleLint(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "ai_counselor.json"))
	if err != nil {
		t.Fatal(err)
	}
	if w := lintApp(t, string(raw)); len(w) != 0 {
		t.Fatalf("ai_counselor lint: %v", w)
	}
	for _, scope := range []string{"shared", "public"} {
		shared := strings.Replace(string(raw), `"titleField": "text",`, `"titleField": "text", "scope": "`+scope+`",`, 1)
		if shared == string(raw) {
			t.Fatal("example changed: no titleField to anchor scope")
		}
		if _, err := Parse([]byte(shared)); err == nil || !strings.Contains(err.Error(), `aiChat entity "turn" must be private`) {
			t.Fatalf("%s aiChat entity: %v", scope, err)
		}
	}
	// persona over 500 characters is caught by the schema too
	long := strings.Replace(string(raw), `"persona": "`, `"persona": "`+strings.Repeat("あ", 500), 1)
	if _, err := Parse([]byte(long)); err == nil {
		t.Fatal("long persona must not parse")
	}
}
