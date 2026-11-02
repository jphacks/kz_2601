package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func flashApp() App {
	app := Initial()
	app.Entities = []Entity{{ID: "word", Name: "単語", Fields: []Field{
		{ID: "term", Name: "単語", Type: FieldString},
		{ID: "meaning", Name: "意味", Type: FieldString},
		{ID: "learned", Name: "覚えた", Type: FieldBoolean},
		{ID: "level", Name: "レベル", Type: FieldInteger},
		{ID: "done", Name: "済", Type: FieldBoolean, Computed: &Expr{Kind: ExprLiteral, Lit: true}},
	}}}
	return app
}

func TestFlashcardsValid(t *testing.T) {
	app := flashApp()
	app.Screens[0].Components = []Component{
		{Type: CompFlashcards, Entity: "word", Front: "term", Back: "meaning"},
		{Type: CompFlashcards, Entity: "word", Front: "term", Back: "meaning", Toggle: "learned",
			Filter: &Filter{Field: "learned", Op: "eq", Value: false}},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("valid flashcards rejected: %v", err)
	}
}

func TestFlashcardsSemanticErrors(t *testing.T) {
	app := flashApp()
	f := func(mod func(c *Component)) Component {
		c := Component{Type: CompFlashcards, Entity: "word", Front: "term", Back: "meaning"}
		mod(&c)
		return c
	}
	app.Screens[0].Components = []Component{
		f(func(c *Component) { c.Entity = "nope" }),
		f(func(c *Component) { c.Front = "zzz" }),
		f(func(c *Component) { c.Back = "" }),
		f(func(c *Component) { c.Back = "term" }),
		f(func(c *Component) { c.Toggle = "level" }),
		f(func(c *Component) { c.Toggle = "missing_toggle" }),
		f(func(c *Component) { c.Toggle = "done" }),
		f(func(c *Component) { c.Filter = &Filter{Field: "xxx", Op: "eq", Value: false} }),
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		`flashcards entity "nope" does not exist`,
		`flashcards front "zzz" not in entity "word"`,
		`flashcards needs back`,
		`front and back are the same field "term"`,
		`flashcards toggle "level" must be a boolean field (got integer)`,
		`flashcards toggle "missing_toggle" not in entity "word"`,
		`flashcards toggle "done" is computed`,
		`filter field "xxx" not in entity`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

// The schema requires front and back; a DSL without them never reaches Validate.
func TestFlashcardsSchemaRequiresFrontBack(t *testing.T) {
	for _, comp := range []string{
		`{"type":"flashcards","entity":"word","front":"term"}`,
		`{"type":"flashcards","entity":"word","back":"term"}`,
	} {
		raw := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home",
"entities":[{"id":"word","name":"単語","fields":[{"id":"term","name":"n","type":"string"}]}],
"screens":[{"id":"home","title":"h","components":[` + comp + `]}]}`)
		if _, err := Parse(raw); err == nil {
			t.Errorf("%s should fail schema validation", comp)
		}
	}
}

// A hidden answer key is not shown on the back of a card.
func TestFlashcardsHiddenBack(t *testing.T) {
	app := flashApp()
	app.Entities[0].Fields[1].Hidden = true
	app.Screens[0].Components = []Component{{Type: CompFlashcards, Entity: "word", Front: "term", Back: "meaning"}}
	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), `back "meaning" of entity "word" is hidden`) {
		t.Fatalf("hidden back should be rejected: %v", err)
	}
}

// The bundled vocab example: flashcards over the unlearned words + the word list.
func TestFlashcardsExample(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "vocab.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatalf("vocab.json: %v", err)
	}
	var cards bool
	for _, c := range app.Screens[0].Components {
		cards = cards || (c.Type == CompFlashcards && c.Toggle == "learned" && c.Filter != nil && c.Filter.Field == "learned")
	}
	if !cards {
		t.Fatal("vocab home should have flashcards with toggle + filter on learned")
	}
	if missing := app.UnlistedEntities(); len(missing) != 0 {
		t.Errorf("unlisted=%v", missing)
	}
	if issues := app.Lint(); len(issues) != 0 {
		t.Errorf("lint: %v", issues)
	}
}
