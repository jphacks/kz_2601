package dsl

import (
	"strings"
	"testing"
)

// #194: a ref field may carry options (initial records of refEntity); other
// non-enum types still may not.
func TestRefOptionsAllowed(t *testing.T) {
	app := memoApp()
	app.Entities = append(app.Entities, Entity{ID: "kind", Name: "k", TitleField: "name",
		Fields: []Field{{ID: "name", Name: "n", Type: FieldString, Required: true}}})
	app.Entities[0].Fields = append(app.Entities[0].Fields,
		Field{ID: "kind", Name: "k", Type: FieldRef, RefEntity: "kind", Options: []string{"a", "b"}})
	if err := app.Validate(); err != nil {
		t.Fatalf("ref options should validate: %v", err)
	}
	app.Entities[0].Fields[0].Options = []string{"x"}
	if err := app.Validate(); err == nil || !strings.Contains(err.Error(), "options only allowed for enum, ref and refs") {
		t.Fatalf("string options should fail, got %v", err)
	}
}
