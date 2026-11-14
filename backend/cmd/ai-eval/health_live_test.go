package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckHealthLive(t *testing.T) {
	live := &dsl.App{Screens: []dsl.Screen{{Components: []dsl.Component{
		{Type: dsl.CompHealthStat, Metric: "steps", Label: "歩数", Live: true},
	}}}}
	plain := &dsl.App{Screens: []dsl.Screen{{Components: []dsl.Component{
		{Type: dsl.CompVStack, Children: []dsl.Component{{Type: dsl.CompHealthStat, Metric: "steps", Label: "歩数"}}},
	}}}}
	if miss := checkHealthLive(map[string]any{"healthLive": true}, live); miss != nil {
		t.Errorf("live app should pass: %v", miss)
	}
	if miss := checkHealthLive(map[string]any{"healthLive": false}, plain); miss != nil {
		t.Errorf("plain app should pass: %v", miss)
	}
	if miss := checkHealthLive(map[string]any{"healthLive": true}, plain); len(miss) != 1 {
		t.Errorf("plain app should miss healthLive=true: %v", miss)
	}
	if miss := checkHealthLive(map[string]any{"healthLive": false}, live); len(miss) != 1 {
		t.Errorf("live app should miss healthLive=false: %v", miss)
	}
	if miss := checkHealthLive(map[string]any{}, live); miss != nil {
		t.Errorf("no key: %v", miss)
	}
}
