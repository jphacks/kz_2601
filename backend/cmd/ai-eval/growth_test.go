package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckGrowth(t *testing.T) {
	tree := &dsl.App{Screens: []dsl.Screen{{Components: []dsl.Component{
		{Type: dsl.CompGrowth, Label: "木", Metric: "steps", Live: true},
	}}}}
	flower := &dsl.App{Screens: []dsl.Screen{{Components: []dsl.Component{
		{Type: dsl.CompCard, Children: []dsl.Component{{Type: dsl.CompGrowth, Label: "花", Kind: "flower", Entity: "x"}}},
	}}}}
	plain := &dsl.App{Screens: []dsl.Screen{{Components: []dsl.Component{
		{Type: dsl.CompHealthStat, Metric: "steps", Label: "歩数", Live: true},
	}}}}
	if miss := checkGrowth(map[string]any{"growth": "tree", "growthSteps": true}, tree); miss != nil {
		t.Errorf("tree should pass: %v", miss)
	}
	if miss := checkGrowth(map[string]any{"growth": "flower", "growthSteps": false}, flower); miss != nil {
		t.Errorf("flower should pass: %v", miss)
	}
	if miss := checkGrowth(map[string]any{"growth": "tree", "growthSteps": true}, plain); len(miss) != 2 {
		t.Errorf("healthStat only should miss both: %v", miss)
	}
	if miss := checkGrowth(map[string]any{"growth": "flower"}, tree); len(miss) != 1 {
		t.Errorf("wrong kind should miss: %v", miss)
	}
	if miss := checkGrowth(map[string]any{}, tree); miss != nil {
		t.Errorf("no key: %v", miss)
	}
}
