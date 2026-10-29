package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckDashcam(t *testing.T) {
	app := func(c dsl.Component) *dsl.App {
		return &dsl.App{Screens: []dsl.Screen{{ID: "home", Components: []dsl.Component{c}}}}
	}
	auto := app(dsl.Component{Type: dsl.CompDashcam, AutoStart: []string{"landscape", "still"}})
	manual := app(dsl.Component{Type: dsl.CompDashcam})
	none := app(dsl.Component{Type: dsl.CompVideoField})

	if miss := checkDashcam(map[string]any{"dashcamAuto": true}, auto); miss != nil {
		t.Fatalf("auto start should pass: %v", miss)
	}
	if miss := checkDashcam(map[string]any{"dashcamAuto": true}, manual); miss == nil {
		t.Fatal("manual-only dashcam must miss dashcamAuto=true")
	}
	if miss := checkDashcam(map[string]any{"dashcamAuto": true}, none); miss == nil {
		t.Fatal("no dashcam must miss dashcamAuto=true")
	}
	if miss := checkDashcam(map[string]any{"dashcamAuto": false}, manual); miss != nil {
		t.Fatalf("manual dashcam should pass dashcamAuto=false: %v", miss)
	}
	if miss := checkDashcam(map[string]any{}, none); miss != nil {
		t.Fatalf("no key = not checked: %v", miss)
	}
}
