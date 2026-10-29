package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckMusic(t *testing.T) {
	app := dsl.Initial()
	app.Screens[0].Components = []dsl.Component{{Type: dsl.CompSection, Children: []dsl.Component{
		{Type: dsl.CompMusicPlayer, AllowRequests: true},
	}}}
	if miss := checkMusic(map[string]any{"musicRequests": true, "musicAutoAccept": false}, &app); len(miss) != 0 {
		t.Errorf("player in a section with requests: %v", miss)
	}
	if miss := checkMusic(map[string]any{"musicAutoAccept": true}, &app); len(miss) != 1 {
		t.Errorf("autoAccept missing should miss once: %v", miss)
	}
	if miss := checkMusic(map[string]any{}, &app); len(miss) != 0 {
		t.Errorf("no music keys: %v", miss)
	}
	empty := dsl.Initial()
	if miss := checkMusic(map[string]any{"musicRequests": true}, &empty); len(miss) != 1 {
		t.Errorf("no player: %v", miss)
	}
}
