package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func randomApp() App {
	app := Initial()
	app.Entities = []Entity{{ID: "dish", Name: "料理", Fields: []Field{
		{ID: "name", Name: "料理名", Type: FieldString},
		{ID: "genre", Name: "ジャンル", Type: FieldEnum, Options: []string{"和食", "洋食"}},
	}}}
	app.Screens = append(app.Screens, Screen{ID: "form", Title: "f", Entity: "dish"})
	return app
}

func TestRandomPickerValid(t *testing.T) {
	app := randomApp()
	app.Screens[0].Components = []Component{
		{Type: CompRandomPicker, Entity: "dish", TitleField: "name"},
		{Type: CompRandomPicker, Entity: "dish", TitleField: "name", SubtitleField: "genre", Label: "決める", Title: "今日",
			Filter:    &Filter{Field: "genre", Op: "eq", Value: "和食"},
			OnItemTap: &Action{Type: ActionNavigate, Screen: "form"}},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("valid randomPicker rejected: %v", err)
	}
}

func TestRandomPickerSemanticErrors(t *testing.T) {
	app := randomApp()
	r := func(mod func(c *Component)) Component {
		c := Component{Type: CompRandomPicker, Entity: "dish", TitleField: "name"}
		mod(&c)
		return c
	}
	app.Screens[0].Components = []Component{
		r(func(c *Component) { c.Entity = "nope" }),
		r(func(c *Component) { c.TitleField = "zzz" }),
		r(func(c *Component) { c.SubtitleField = "yyy" }),
		r(func(c *Component) { c.Filter = &Filter{Field: "xxx", Op: "eq", Value: "a"} }),
		r(func(c *Component) { c.OnItemTap = &Action{Type: ActionNavigate, Screen: "missing"} }),
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		`randomPicker entity "nope" does not exist`,
		`randomPicker titleField "zzz" not in entity`,
		`randomPicker subtitleField "yyy" not in entity`,
		`filter field "xxx" not in entity`,
		`"missing"`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

// The schema requires titleField; a DSL without it never reaches Validate.
func TestRandomPickerSchemaRequiresTitleField(t *testing.T) {
	raw := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home",
"entities":[{"id":"dish","name":"料理","fields":[{"id":"name","name":"n","type":"string"}]}],
"screens":[{"id":"home","title":"h","components":[{"type":"randomPicker","entity":"dish"}]}]}`)
	if _, err := Parse(raw); err == nil {
		t.Fatal("randomPicker without titleField should fail schema validation")
	}
}

// The bundled dinner example keeps the picker next to the list of candidates.
func TestRandomPickerExample(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "dinner.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatalf("dinner.json: %v", err)
	}
	var picker, list bool
	for _, c := range app.Screens[0].Components {
		picker = picker || (c.Type == CompRandomPicker && c.Entity == "dish" && c.TitleField == "name")
		list = list || (c.Type == CompList && c.Entity == "dish")
	}
	if !picker || !list {
		t.Fatalf("dinner home should have randomPicker + list of dish (picker=%v list=%v)", picker, list)
	}
	if missing := app.UnlistedEntities(); len(missing) != 0 {
		t.Errorf("unlisted=%v", missing)
	}
}
