package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestDeletable2753(t *testing.T) {
	del := dsl.Action{Type: dsl.ActionDeleteRecord, Entity: "task"}
	tests := []struct {
		name string
		app  *dsl.App
		want bool
	}{
		{"no screens", &dsl.App{}, false},
		{"toolbar delete button", &dsl.App{Screens: []dsl.Screen{{ID: "s", Toolbar: []dsl.ToolbarButton{{Action: del}}}}}, true},
		{"component delete action", &dsl.App{Screens: []dsl.Screen{{ID: "s", Components: []dsl.Component{{Type: dsl.CompButton, Action: &del}}}}}, true},
		{"child delete action", &dsl.App{Screens: []dsl.Screen{{ID: "s", Components: []dsl.Component{{Type: dsl.CompList, Children: []dsl.Component{{Type: dsl.CompButton, Action: &del}}}}}}}, true},
		{"list swipe to delete", &dsl.App{Screens: []dsl.Screen{{ID: "s", Components: []dsl.Component{{Type: dsl.CompList, Entity: "task", SwipeToDelete: true}}}}}, true},
		{"list without swipe", &dsl.App{Screens: []dsl.Screen{{ID: "s", Components: []dsl.Component{{Type: dsl.CompList, Entity: "task"}}}}}, false},
		{"swipe on another entity", &dsl.App{Screens: []dsl.Screen{{ID: "s", Components: []dsl.Component{{Type: dsl.CompList, Entity: "memo", SwipeToDelete: true}}}}}, false},
		{"delete of another entity", &dsl.App{Screens: []dsl.Screen{{ID: "s", Components: []dsl.Component{{Type: dsl.CompButton, Action: &dsl.Action{Type: dsl.ActionDeleteRecord, Entity: "memo"}}}}}}, false},
		{"button without action", &dsl.App{Screens: []dsl.Screen{{ID: "s", Components: []dsl.Component{{Type: dsl.CompButton}}}}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := deletable(tc.app, "task"); got != tc.want {
				t.Errorf("deletable = %v, want %v", got, tc.want)
			}
		})
	}
	if deletable(&dsl.App{}, "") {
		t.Error("empty entity on an empty app should not be deletable")
	}
}

func TestMentionsSubstitution2753(t *testing.T) {
	tests := []struct {
		summary string
		want    bool
	}{
		{"支払い機能は代替の方法で作りました", true},
		{"その機能はできません", true},
		{"通知はできないため別の形にしました", true},
		{"Payments were replaced with a manual list", true},
		{"This isn't possible, so I used a note", true},
		{"I CAN'T do that", true},
		{"家計簿を作りました", false},
		{"Added a budget screen", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := mentionsSubstitution(tc.summary); got != tc.want {
			t.Errorf("mentionsSubstitution(%q) = %v, want %v", tc.summary, got, tc.want)
		}
	}
}

func TestHasCalc2753(t *testing.T) {
	tests := []struct {
		name string
		m    Metrics
		want bool
	}{
		{"nothing", Metrics{}, false},
		{"only a ref field", Metrics{RefFields: 2}, false},
		{"computed", Metrics{Computed: []string{"task.total"}}, true},
		{"stat values", Metrics{StatValues: 1}, true},
		{"settlement", Metrics{Settlement: true}, true},
		{"progress target", Metrics{ProgressTo: true}, true},
	}
	for _, tc := range tests {
		if got := tc.m.HasCalc(); got != tc.want {
			t.Errorf("%s: HasCalc = %v, want %v", tc.name, got, tc.want)
		}
	}
}
