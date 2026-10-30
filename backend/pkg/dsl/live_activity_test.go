package dsl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func activityApp(act Action) App {
	app := Initial()
	app.Entities = []Entity{{ID: "bowl", Name: "記録", Fields: []Field{{ID: "name", Name: "名前", Type: FieldString}}}}
	app.Screens[0].Components = []Component{
		{Type: CompStat, ID: "month", Entity: "bowl", Aggregate: "count", Label: "杯数"},
		{Type: CompText, ID: "note", Text: "x"},
		{Type: CompButton, ID: "go", Label: "並ぶ", Action: &act},
	}
	return app
}

func TestActivityValid(t *testing.T) {
	for _, act := range []Action{
		{Type: ActionStartActivity, Activity: &Activity{Title: "並び中"}},
		{Type: ActionStartActivity, Activity: &Activity{Title: "並び中", Subtitle: "s", Icon: "person.3", Minutes: MaxActivityMinutes, Stat: "month"}},
		{Type: ActionEndActivity},
	} {
		app := activityApp(act)
		if err := app.Validate(); err != nil {
			t.Errorf("%+v rejected: %v", act.Activity, err)
		}
	}
}

func TestActivityErrors(t *testing.T) {
	for want, act := range map[string]Action{
		"needs activity{title}":      {Type: ActionStartActivity},
		"activity.title is empty":    {Type: ActionStartActivity, Activity: &Activity{}},
		"minutes must be 1-480":      {Type: ActionStartActivity, Activity: &Activity{Title: "t", Minutes: 481}},
		`stat "nope" is not a`:       {Type: ActionStartActivity, Activity: &Activity{Title: "t", Stat: "nope"}},
		`stat "note" is a text`:      {Type: ActionStartActivity, Activity: &Activity{Title: "t", Stat: "note"}},
		"unknown action type \"liv": {Type: "liveActivity"},
	} {
		app := activityApp(act)
		err := app.Validate()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q, got %v", want, err)
		}
	}
}

// The example parses, validates and keeps liveActivity / activity on round trip.
func TestRamenTimerExample(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "dsl", "examples", "ramen_timer.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !app.Screens[0].Components[0].LiveActivity {
		t.Error("timerDisplay.liveActivity lost")
	}
	act := app.Screens[0].Components[2].Action
	if act == nil || act.Type != ActionStartActivity || act.Activity == nil || act.Activity.Stat != "month" || act.Activity.Minutes != 30 {
		t.Fatalf("startActivity lost: %+v", act)
	}
	out, _ := json.Marshal(app)
	if !strings.Contains(string(out), `"liveActivity":true`) || !strings.Contains(string(out), `"activity":{"title":"行列に並び中"`) {
		t.Errorf("round trip lost keys: %s", out)
	}
}
