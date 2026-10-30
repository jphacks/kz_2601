package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #425: checkNearby tells passby / distance apart from the send button.
func TestCheckNearby(t *testing.T) {
	app := func(c dsl.Component) *dsl.App {
		return &dsl.App{Screens: []dsl.Screen{{ID: "home", Components: []dsl.Component{c}}}}
	}
	send := app(dsl.Component{Type: dsl.CompNearbyShare, Entity: "p"})
	passby := app(dsl.Component{Type: dsl.CompNearbyShare, Entity: "p", Mode: dsl.NearbyModePassby, Fields: []string{"name"}})
	dist := app(dsl.Component{Type: dsl.CompNearbyShare, Entity: "p", Mode: dsl.NearbyModePassby, Fields: []string{"name"}, Distance: true})

	both := map[string]any{"passby": true, "passbyDistance": true}
	if miss := checkNearby(both, dist); len(miss) != 0 {
		t.Fatal(miss)
	}
	if miss := checkNearby(both, passby); len(miss) != 1 {
		t.Fatalf("passby without distance: %v", miss)
	}
	if miss := checkNearby(both, send); len(miss) != 2 {
		t.Fatalf("send button: %v", miss)
	}
	if miss := checkNearby(map[string]any{"passby": false}, send); len(miss) != 0 {
		t.Fatal(miss)
	}
	if miss := checkNearby(map[string]any{}, dist); len(miss) != 0 {
		t.Fatal(miss)
	}
}
