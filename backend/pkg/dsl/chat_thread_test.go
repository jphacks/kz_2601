package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func chatApp() App {
	app := memoApp()
	app.Entities[0].Scope = ScopePublic
	app.Entities[0].Fields = append(app.Entities[0].Fields, Field{ID: "pic", Name: "写真", Type: FieldPhoto})
	return app
}

func TestChatThreadSemanticErrors(t *testing.T) {
	app := chatApp()
	app.Entities[0].Fields = append(app.Entities[0].Fields, Field{ID: "must", Name: "x", Type: FieldString, Required: true})
	app.Screens[0].Components = []Component{
		{Type: CompChatThread, Entity: "memo", TextField: "note", PhotoField: "pic"}, // ok except "must"
		{Type: CompChatThread, Entity: "memo", TextField: "elapsed"},                 // not text
		{Type: CompChatThread, Entity: "memo", TextField: "gone"},
		{Type: CompChatThread, Entity: "memo", TextField: "note", PhotoField: "note"}, // not a photo
		{Type: CompChatThread, Entity: "nope", TextField: "note"},
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		`required field "must" of entity "memo" needs a defaultValue`,
		`chatThread textField "elapsed" must be a stored string or text field`,
		`chatThread textField "gone" not in entity "memo"`,
		`chatThread photoField "note" must be a stored photo field`,
		`chatThread entity "nope" does not exist`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

func TestChatThreadValid(t *testing.T) {
	app := chatApp()
	app.Entities[0].Fields = append(app.Entities[0].Fields, Field{ID: "kind", Name: "k", Type: FieldString, Required: true, DefaultValue: "msg"})
	app.Screens[0].Components = []Component{{Type: CompChatThread, Area: "A1:F12", Entity: "memo", TextField: "note", PhotoField: "pic"}}
	if err := app.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

// The chat entity is listed and written by chatThread: no "no add form" /
// "no list" warnings. A private chat warns (nobody else sees it).
func TestChatThreadLint(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "chat.json"))
	if err != nil {
		t.Fatal(err)
	}
	if w := lintApp(t, string(raw)); len(w) != 0 {
		t.Fatalf("chat example lint: %v", w)
	}
	private := strings.Replace(string(raw), `"scope": "public",`, "", 1)
	w := lintApp(t, private)
	if len(w) != 1 || !strings.Contains(w[0], `chatThread entity "messages" is private`) {
		t.Fatalf("private chat lint: %v", w)
	}
}
