package dsl

import (
	"strings"
	"testing"
)

func TestLocationTrackerSemanticErrors(t *testing.T) {
	app := memoApp()
	app.Entities[0].Fields = append(app.Entities[0].Fields, Field{ID: "must", Name: "x", Type: FieldString, Required: true})
	app.Screens[0].Components = []Component{
		{Type: CompLocationTracker, Entity: "memo", LocationField: "place", DateField: "date"}, // ok except "must"
		{Type: CompLocationTracker, Entity: "memo", LocationField: "note"},                     // not a location
		{Type: CompLocationTracker, Entity: "memo", LocationField: "place", DateField: "note"}, // not a datetime
		{Type: CompLocationTracker, Entity: "nope", LocationField: "place"},
		{Type: CompMapView, Entity: "memo", LocationField: "place", Kind: "route", DateField: "date"}, // ok
		{Type: CompMapView, Entity: "memo", LocationField: "place", Kind: "line"},
		{Type: CompMapView, Entity: "memo", LocationField: "place", DateField: "date"},                   // dateField without route
		{Type: CompStat, Label: "距離", Entity: "memo", Aggregate: "distance", Field: "place", By: "date"}, // ok
		{Type: CompStat, Label: "距離", Entity: "memo", Aggregate: "distance", Field: "elapsed"},
		{Type: CompStat, Label: "時間", Entity: "memo", Aggregate: "duration", Field: "place", By: "note"},
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		`required field "must" of entity "memo" needs a defaultValue`,
		`locationTracker locationField "note" must be a stored location field`,
		`locationTracker dateField "note" must be a stored datetime field`,
		`locationTracker entity "nope" does not exist`,
		"mapView kind must be pins or route",
		"mapView dateField is only for kind route",
		`stat aggregate distance field "elapsed" must be the location field`,
		`duration by "note" must be a date or datetime field`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

func TestLocationTrackerValid(t *testing.T) {
	app := memoApp()
	app.Screens[0].Components = []Component{
		{Type: CompLocationTracker, Entity: "memo", LocationField: "place", DateField: "date", Seconds: 1800},
		{Type: CompMapView, Entity: "memo", LocationField: "place", Kind: "route", DateField: "date"},
		{Type: CompStat, Label: "距離", Entity: "memo", Aggregate: "distance", Field: "place", By: "date"},
		{Type: CompStat, Label: "時間", Entity: "memo", Aggregate: "duration", Field: "place", By: "date"},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}
