package dsl

import (
	"encoding/json"
	"strings"
	"testing"
)

func placeApp() App {
	app := memoApp()
	app.Entities[0].Fields = append(app.Entities[0].Fields,
		Field{ID: "addr", Name: "a", Type: FieldString},
		Field{ID: "body", Name: "b", Type: FieldText},
	)
	return app
}

func TestPlaceSearchSemanticErrors(t *testing.T) {
	app := placeApp()
	app.Screens[0].Components = []Component{
		{Type: CompPlaceSearch, NameField: "note"}, // needs screen.entity
	}
	app.Screens[1].Components = []Component{
		{Type: CompPlaceSearch},                                          // fills nothing
		{Type: CompPlaceSearch, NameField: "elapsed"},                    // not text
		{Type: CompPlaceSearch, LocationField: "note"},                   // not a location
		{Type: CompPlaceSearch, AddressField: "nope"},                    // missing
		{Type: CompPlaceSearch, NameField: "addr", AddressField: "addr"}, // same field twice
		{Type: CompPlaceSearch, NameField: "note", Category: "zoo", Near: "home", Query: strings.Repeat("あ", 51)},
		{Type: CompPlaceSearch, NameField: "note", Query: "  "},
		{Type: CompLocationButton, Field: "place", AddressField: "elapsed"},
		{Type: CompLocationButton, Field: "place", AddressField: "place"},
		{Type: CompLocationButton, Field: "place", AddressField: "nope"},
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		"placeSearch requires screen.entity",
		"placeSearch needs nameField, locationField or addressField",
		`placeSearch nameField "elapsed" must be a stored string or text field`,
		`placeSearch locationField "note" must be a stored location field`,
		`placeSearch addressField "nope" not in entity "memo"`,
		`placeSearch nameField and addressField are both "addr"`,
		`placeSearch category "zoo" must be one of`,
		`placeSearch near "home" must be current`,
		"placeSearch query is longer than 50 characters",
		"placeSearch query is blank",
		`locationButton addressField "elapsed" must be a stored string or text field`,
		"locationButton addressField must differ from field",
		`locationButton addressField "nope" not in entity "memo"`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

func TestPlaceSearchValidAndParses(t *testing.T) {
	app := placeApp()
	app.Screens[1].Components = []Component{
		{Type: CompPlaceSearch, Label: "近くのカフェ", Category: "cafe", Near: "current", NameField: "note", LocationField: "place", AddressField: "addr", Pins: true},
		{Type: CompPlaceSearch, Query: "コワーキング", AddressField: "body"}, // text works too
		{Type: CompPlaceSearch, LocationField: "place"},                 // free search, one field
		{Type: CompLocationButton, Field: "place", AddressField: "addr"},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	raw, err := json.Marshal(app)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(raw) // the schema accepts the new leaf and key
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c := got.Screens[1].Components[0]; c.Category != "cafe" || !c.Pins || c.AddressField != "addr" {
		t.Fatalf("round trip lost keys: %+v", c)
	}
}

func TestPlaceCategoriesMatchSchema(t *testing.T) {
	var s struct {
		Defs map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(SchemaJSON(), &s); err != nil {
		t.Fatal(err)
	}
	var ps struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(s.Defs["PlaceSearch"], &ps); err != nil {
		t.Fatal(err)
	}
	got := ps.Properties["category"].Enum
	if strings.Join(got, ",") != strings.Join(PlaceCategories, ",") {
		t.Fatalf("schema %v != PlaceCategories %v", got, PlaceCategories)
	}
}
