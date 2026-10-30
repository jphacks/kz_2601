package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckRoute(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "dsl", "examples", "drive.json"))
	if err != nil {
		t.Fatal(err)
	}
	drive, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := routeGuides(drive); !slices.Equal(got, []string{"entity:automobile", "record:automobile"}) {
		t.Fatalf("routeGuides = %v", got)
	}
	for _, exp := range []map[string]any{
		{"routeDest": "entity"}, {"routeDest": "record"}, {"routeDest": "any", "routeMode": "automobile"}, {},
	} {
		if miss := checkRoute(exp, drive); miss != nil {
			t.Errorf("%v: %v", exp, miss)
		}
	}
	for _, exp := range []map[string]any{{"routeDest": "address"}, {"routeMode": "walking"}} {
		if miss := checkRoute(exp, drive); miss == nil {
			t.Errorf("%v passed on the drive example", exp)
		}
	}

	walk := &dsl.App{Screens: []dsl.Screen{{ID: "home", Components: []dsl.Component{
		{Type: dsl.CompCard, Children: []dsl.Component{{Type: dsl.CompRouteGuide, Address: "金沢駅", Mode: "walking"}}},
	}}}}
	if miss := checkRoute(map[string]any{"routeDest": "address", "routeMode": "walking"}, walk); miss != nil {
		t.Errorf("address in a container: %v", miss)
	}
	// dest and mode must hold for the same routeGuide
	mixed := &dsl.App{Screens: []dsl.Screen{{ID: "home", Components: []dsl.Component{
		{Type: dsl.CompRouteGuide, Address: "金沢駅"},
		{Type: dsl.CompRouteGuide, Entity: "spot", LocationField: "place", Mode: "walking"},
	}}}}
	if miss := checkRoute(map[string]any{"routeDest": "address", "routeMode": "walking"}, mixed); miss == nil {
		t.Error("dest and mode from different routeGuides passed")
	}
}
