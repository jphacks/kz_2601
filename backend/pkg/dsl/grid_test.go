package dsl

import (
	"strings"
	"testing"
)

func TestParseArea(t *testing.T) {
	ok := map[string]Area{
		"A1:C3":   {0, 1, 2, 3},
		"A1:F12":  {0, 1, 5, 12},
		"D11:F12": {3, 11, 5, 12},
		"B2:B2":   {1, 2, 1, 2},
	}
	for s, want := range ok {
		got, err := ParseArea(s)
		if err != nil || got != want {
			t.Errorf("ParseArea(%q) = %v, %v; want %v", s, got, err, want)
		}
		if got.String() != s {
			t.Errorf("String() = %q, want %q", got.String(), s)
		}
	}
	for _, bad := range []string{"", "G1:G2", "A0:B1", "C3:A1", "A1:A0", "a1:c3", "A1", "A1:C99"} {
		if _, err := ParseArea(bad); err == nil {
			t.Errorf("ParseArea(%q) should fail", bad)
		}
	}
}

func TestGridOverlapIsReported(t *testing.T) {
	app := Initial()
	app.Screens[0].Components = []Component{
		{Type: CompText, ID: "a", Area: "A1:C3", Text: "a"},
		{Type: CompText, ID: "b", Area: "C3:F6", Text: "b"}, // shares C3 with a
		{Type: CompText, ID: "c", Area: "G1:G2", Text: "c"}, // bad column
		{Type: CompText, ID: "d", Area: "A0:B1", Text: "d"}, // bad row
		{Type: CompHStack, ID: "e", Area: "A7:F8", Children: []Component{{Type: CompDivider, Area: "A1:A1"}}},
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		`(id "b") area C3:F6 overlaps`,
		`(id "a") area A1:C3`,
		`area "G1:G2" must look like`,
		`area "A0:B1" must look like`,
		`children of hstack cannot have area`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("problems missing %q:\n%s", want, joined)
		}
	}
}

func stat(id, area string) Component {
	return Component{Type: CompStat, ID: id, Area: area, Label: id, Entity: "expense", Aggregate: "count"}
}

// #81: card / section / vstack children may use a local grid.
func TestLocalGridValid(t *testing.T) {
	app := Initial()
	app.Screens[0].Components = []Component{
		{Type: CompCard, ID: "c", Area: "A1:F3", Children: []Component{stat("a", "A1:C2"), stat("b", "D1:F2"), stat("w", "A3:F3")}},
		// stacked container: rows are bounded only by MaxGridRows
		{Type: CompVStack, ID: "v", Children: []Component{stat("x", "A1:C1"), stat("y", "D1:F1")}},
	}
	app.Entities = []Entity{{ID: "expense", Name: "e", Fields: []Field{{ID: "t", Name: "t", Type: FieldString}}}}
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLocalGridProblems(t *testing.T) {
	cases := map[string]struct {
		c    Component
		want string
	}{
		"partial": {Component{Type: CompCard, Area: "A1:F3", Children: []Component{stat("a", "A1:C1"), stat("b", "")}},
			"only 1 of 2 children have area"},
		"overlap": {Component{Type: CompSection, Area: "A1:F3", Children: []Component{stat("a", "A1:D1"), stat("b", "D1:F1")}},
			`(id "b") area D1:F1 overlaps`},
		"rows": {Component{Type: CompCard, Area: "A4:F5", Children: []Component{stat("a", "A1:C2"), stat("b", "D2:F3")}},
			`goes past row 2; inside a container rows count from 1`},
		"format": {Component{Type: CompVStack, Area: "A1:F3", Children: []Component{stat("a", "A1")}},
			`area "A1" must look like`},
		"grid": {Component{Type: CompGrid, Area: "A1:F3", Children: []Component{stat("a", "A1:C1")}},
			`children of grid cannot have area`},
		"depth": {Component{Type: CompCard, Area: "A1:F3", Children: []Component{{Type: CompVStack, Area: "A1:F1", Children: []Component{stat("a", "A1:C1")}}}},
			`area is allowed only one level inside a container`},
	}
	for name, tc := range cases {
		app := Initial()
		app.Entities = []Entity{{ID: "expense", Name: "e", Fields: []Field{{ID: "t", Name: "t", Type: FieldString}}}}
		app.Screens[0].Components = []Component{tc.c}
		err := app.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want %q, got %v", name, tc.want, err)
		}
	}
}

func TestGridValidLayoutPasses(t *testing.T) {
	app := Initial()
	app.Screens[0].Components = []Component{
		{Type: CompText, ID: "a", Area: "A1:C3", Text: "a"},
		{Type: CompText, ID: "b", Area: "D1:F3", Text: "b"},
		{Type: CompText, ID: "c", Area: "A4:F12", Text: "c"},
		{Type: CompText, ID: "d", Text: "no area -> stacked below"},
		{Type: CompText, ID: "e", Area: "A13:F20", Text: "scrolls"},
	}
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestTabsValidation(t *testing.T) {
	app := Initial()
	app.App.Tabs = []Tab{{Screen: "home", Title: "a"}}
	if err := app.Validate(); err == nil || !strings.Contains(err.Error(), "tabs") {
		t.Fatalf("expected tabs error, got %v", err)
	}
	app.App.Tabs = []Tab{{Screen: "home", Title: "a"}, {Screen: "nope", Title: "b"}}
	if err := app.Validate(); err == nil || !strings.Contains(err.Error(), `"nope"`) {
		t.Fatalf("expected missing screen error, got %v", err)
	}
}
