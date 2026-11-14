package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckFeed(t *testing.T) {
	app := func(cs ...dsl.Component) *dsl.App {
		a := dsl.Initial()
		a.Screens[0].Components = []dsl.Component{{Type: dsl.CompCard, Children: cs}}
		return &a
	}
	weather := dsl.Component{Type: dsl.CompFeed, Provider: dsl.FeedWeather, Region: "170000"}
	holidays := dsl.Component{Type: dsl.CompFeed, Provider: dsl.FeedHolidays}
	for name, tc := range map[string]struct {
		exp  map[string]any
		app  *dsl.App
		pass bool
	}{
		"weather":        {map[string]any{"feed": "weather"}, app(weather), true},
		"region":         {map[string]any{"feed": "weather", "feedRegion": "170000"}, app(weather), true},
		"wrong region":   {map[string]any{"feedRegion": "130000"}, app(weather), false},
		"wrong provider": {map[string]any{"feed": "rss"}, app(weather, holidays), false},
		"none ok":        {map[string]any{"feed": "none"}, app(dsl.Component{Type: dsl.CompText, Text: "x"}), true},
		"none but feed":  {map[string]any{"feed": "none"}, app(holidays), false},
		"no keys":        {map[string]any{}, app(), true},
	} {
		if got := len(checkFeed(tc.exp, tc.app)) == 0; got != tc.pass {
			t.Errorf("%s: pass=%v want %v", name, got, tc.pass)
		}
	}
}
