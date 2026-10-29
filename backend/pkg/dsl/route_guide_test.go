package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRouteGuideValid(t *testing.T) {
	app := memoApp()
	app.Screens[0].Components = []Component{
		{Type: CompRouteGuide, Entity: "memo", LocationField: "place", TitleField: "note"}, // choose among records
		{Type: CompRouteGuide, Address: "金沢駅", Mode: "walking", Title: "駅まで", Label: "出発"},
	}
	app.Screens[1].Components = []Component{
		{Type: CompRouteGuide, LocationField: "place", TitleField: "note", Mode: "automobile"}, // the form's record
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("valid routeGuide rejected: %v", err)
	}
}

func TestRouteGuideSemanticErrors(t *testing.T) {
	app := memoApp()
	app.Screens[0].Components = []Component{
		{Type: CompRouteGuide},                         // no destination
		{Type: CompRouteGuide, LocationField: "place"}, // no screen.entity, no entity
		{Type: CompRouteGuide, Entity: "nope", LocationField: "place"},
		{Type: CompRouteGuide, Address: "金沢駅", LocationField: "place", Entity: "memo"},
		{Type: CompRouteGuide, Address: "金沢駅", Entity: "memo"},
		{Type: CompRouteGuide, Address: strings.Repeat("あ", 201)},
		{Type: CompRouteGuide, Address: "金沢駅", Mode: "bicycle"},
	}
	app.Screens[1].Components = []Component{
		{Type: CompRouteGuide, LocationField: "note"},                   // not a location
		{Type: CompRouteGuide, LocationField: "zzz"},                    // missing
		{Type: CompRouteGuide, LocationField: "place", TitleField: "x"}, // missing title
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		"routeGuide needs a destination",
		"routeGuide locationField needs screen.entity",
		`routeGuide entity "nope" does not exist`,
		"routeGuide takes either locationField or address, not both",
		"a fixed address needs neither",
		"routeGuide address is longer than 200 characters",
		`routeGuide mode "bicycle" must be automobile or walking`,
		`routeGuide locationField "note" must be type location`,
		`routeGuide locationField "zzz" not in entity "memo"`,
		`routeGuide titleField "x" not in entity "memo"`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

// The schema keeps mode closed and the component strict.
func TestRouteGuideSchema(t *testing.T) {
	base := `{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home","entities":[],
"screens":[{"id":"home","title":"h","components":[%s]}]}`
	if _, err := Parse([]byte(strings.Replace(base, "%s", `{"type":"routeGuide","address":"金沢駅","mode":"walking"}`, 1))); err != nil {
		t.Fatalf("address routeGuide rejected: %v", err)
	}
	for _, bad := range []string{
		`{"type":"routeGuide","address":"金沢駅","mode":"bicycle"}`,
		`{"type":"routeGuide","address":"金沢駅","voice":true}`,
	} {
		if _, err := Parse([]byte(strings.Replace(base, "%s", bad, 1))); err == nil {
			t.Errorf("schema accepted %s", bad)
		}
	}
}

// The drive example guides from the home (choose a spot) and from the spot form.
func TestRouteGuideExample(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "drive.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatalf("drive.json: %v", err)
	}
	var home, form bool
	for _, s := range app.Screens {
		for _, c := range s.Components {
			if c.Type != CompRouteGuide || c.LocationField != "place" {
				continue
			}
			home = home || (s.Entity == "" && c.Entity == "spot")
			form = form || (s.Entity == "spot" && c.Entity == "")
		}
	}
	if !home || !form {
		t.Fatalf("drive should guide from home and the spot form (home=%v form=%v)", home, form)
	}
	if missing := app.UnlistedEntities(); len(missing) != 0 {
		t.Errorf("unlisted=%v", missing)
	}
}
