package dsl

import (
	"strings"
	"testing"
)

func TestLocationPickerSemanticErrors(t *testing.T) {
	app := memoApp()
	app.Screens[0].Components = []Component{
		{Type: CompLocationPicker, Field: "place"}, // needs screen.entity
	}
	app.Screens[1].Components = []Component{
		{Type: CompLocationPicker, Field: "note"},                        // not a location
		{Type: CompLocationPicker, Field: "place", NameField: "elapsed"}, // name not a string
		{Type: CompLocationPicker, Field: "place", NameField: "nope"},
		{Type: CompLocationPicker, Field: "place", NameField: "place"},
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		"locationPicker requires screen.entity",
		`locationPicker field "note" must be a stored location field`,
		`locationPicker nameField "elapsed" must be a stored string field`,
		`locationPicker nameField "nope" not in entity "memo"`,
		"locationPicker nameField must differ from field",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

func TestLocationPickerValid(t *testing.T) {
	app := memoApp()
	app.Screens[1].Components = []Component{
		{Type: CompLocationPicker, Field: "place", NameField: "note", Label: "場所を選ぶ"},
		{Type: CompLocationPicker, Field: "place"}, // nameField is optional
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}
