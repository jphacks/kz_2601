package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckLiveActivity(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "dsl", "examples", "ramen_timer.json"))
	if err != nil {
		t.Fatal(err)
	}
	ramen, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if miss := checkLiveActivity(map[string]any{"liveTimer": true, "activityButtons": true}, ramen); miss != nil {
		t.Fatalf("ramen: %v", miss)
	}
	if miss := checkLiveActivity(map[string]any{"liveTimer": false, "activityButtons": false}, ramen); len(miss) != 2 {
		t.Fatalf("false expects should both miss: %v", miss)
	}
	if miss := checkLiveActivity(map[string]any{}, ramen); miss != nil {
		t.Fatalf("no expect key: %v", miss)
	}

	// start without end is not a usable pair; a timer without the flag does not count
	half := &dsl.App{Screens: []dsl.Screen{{ID: "home",
		Toolbar: []dsl.ToolbarButton{{Label: "開始", Action: dsl.Action{Type: dsl.ActionStartActivity, Activity: &dsl.Activity{Title: "運転中"}}}},
		Components: []dsl.Component{{Type: dsl.CompTimerDisplay, Mode: "stopwatch"}},
	}}}
	if miss := checkLiveActivity(map[string]any{"liveTimer": false, "activityButtons": false}, half); miss != nil {
		t.Fatalf("half: %v", miss)
	}
}
