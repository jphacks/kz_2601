package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gallery-structure.json must exercise every structure/nav/native type so that
// the backend, iOS decoding and the renderer all see each one at least once.
func TestGalleryStructureCoversAllTypes(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "gallery-structure.json"))
	if err != nil {
		t.Skip("gallery not found:", err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[ComponentType]bool{}
	for _, s := range app.Screens {
		for _, c := range s.Components {
			seen[c.Type] = true
			for _, ch := range c.Children {
				seen[ch.Type] = true
			}
		}
	}
	for _, want := range []ComponentType{
		CompCard, CompDisclosure, CompCarousel, CompGrid,
		CompSearchBar, CompCalendar, CompShareButton, CompOpenURLButton,
		CompMapView, CompLocationButton, CompQrScanner, CompTimerDisplay,
	} {
		if !seen[want] {
			t.Errorf("gallery-structure.json does not use %q", want)
		}
	}
}

func memoApp() App {
	app := Initial()
	app.Entities = []Entity{{ID: "memo", Name: "m", Fields: []Field{
		{ID: "note", Name: "n", Type: FieldString},
		{ID: "place", Name: "p", Type: FieldLocation},
		{ID: "date", Name: "d", Type: FieldDatetime},
		{ID: "elapsed", Name: "e", Type: FieldInteger},
	}}}
	app.Screens = append(app.Screens, Screen{ID: "form", Title: "f", Entity: "memo"})
	return app
}

func TestStructureSemanticErrors(t *testing.T) {
	yes := true
	app := memoApp()
	app.Screens[0].Components = []Component{
		{Type: CompGrid, Columns: 5, Children: []Component{{Type: CompDivider}}},         // range
		{Type: CompCard, Columns: 2, Children: []Component{{Type: CompDivider}}},         // columns only on grid
		{Type: CompCarousel, Expanded: &yes, Children: []Component{{Type: CompDivider}}}, // expanded only on disclosure
		{Type: CompSection, ItemWidth: "small", Children: []Component{{Type: CompDivider}}},
		{Type: CompGrid, Columns: 2}, // empty grid
		{Type: CompList, Entity: "memo", Item: &ListItem{Title: "note"}, SearchField: "q"}, // no searchBar
		{Type: CompCalendar, Entity: "memo", DateField: "note"},                            // not a date
		{Type: CompCalendar, Entity: "nope", DateField: "date"},
		{Type: CompMapView, Entity: "memo", LocationField: "note"}, // not a location
		{Type: CompLocationButton, Field: "place"},                 // needs screen.entity
		{Type: CompTimerDisplay, Mode: "countdown"},                // seconds
		{Type: CompTimerDisplay, Mode: "sundial"},
		{Type: CompOpenURLButton, Label: "x"}, // url
		{Type: CompShareButton},               // text
	}
	app.Screens[1].Components = []Component{
		{Type: CompLocationButton, Field: "note"},                  // wrong type
		{Type: CompQrScanner, Field: "elapsed"},                    // wrong type
		{Type: CompTimerDisplay, Mode: "stopwatch", Field: "note"}, // wrong type
		{Type: CompSearchBar, Field: "q"},
		{Type: CompSearchBar, Field: "q"}, // duplicate
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		"grid columns must be 2-4 (got 5)",
		"columns is only allowed on grid",
		"expanded is only allowed on disclosure",
		"itemWidth is only allowed on carousel",
		"grid needs at least one child",
		`searchField "q" has no matching searchBar`,
		`dateField "note" must be date or datetime`,
		`calendar entity "nope" does not exist`,
		`locationField "note" must be type location`,
		"locationButton requires screen.entity",
		"countdown timer needs seconds > 0",
		"timerDisplay mode must be stopwatch or countdown",
		"openUrlButton needs label and url",
		"shareButton needs text",
		`locationButton field "note" must be type location`,
		`qrScanner field "elapsed" must be string or text`,
		`timerDisplay field "note" must be integer or number`,
		`duplicate searchBar field "q"`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("problems missing %q:\n%s", want, joined)
		}
	}
}

func TestStructureValidPasses(t *testing.T) {
	open := true
	app := memoApp()
	app.Screens[0].Components = []Component{
		{Type: CompSearchBar, Field: "q", Area: "A1:F1"},
		{Type: CompList, Entity: "memo", Item: &ListItem{Title: "note"}, SearchField: "q", Area: "A2:F4"},
		{Type: CompGrid, Columns: 3, Area: "A5:F6", Children: []Component{{Type: CompText, Text: "a"}, {Type: CompText, Text: "b"}}},
		{Type: CompDisclosure, Title: "more", Expanded: &open, Children: []Component{{Type: CompText, Text: "c"}}},
		{Type: CompCarousel, ItemWidth: "large", Children: []Component{{Type: CompText, Text: "d"}}},
		{Type: CompCalendar, Entity: "memo", DateField: "date", OnDateTap: &Action{Type: ActionNavigate, Screen: "form"}},
		{Type: CompMapView, Entity: "memo", LocationField: "place", TitleField: "note", Zoom: "near"},
		{Type: CompTimerDisplay, Mode: "stopwatch"},
		{Type: CompShareButton, Text: "hi"},
		{Type: CompOpenURLButton, Label: "go", URL: "https://example.com"},
	}
	app.Screens[1].Components = []Component{
		{Type: CompLocationButton, Field: "place"},
		{Type: CompQrScanner, Field: "note"},
		{Type: CompTimerDisplay, Mode: "countdown", Seconds: 90, Field: "elapsed"},
		{Type: CompShareButton, Text: "{{note}}", Field: "note"},
	}
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
}
