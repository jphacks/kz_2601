package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Music (#412) expect keys, checked by checkMusic:
//   - musicRequests (bool): some musicPlayer has allowRequests (friends can
//     send song requests from their phones).
//   - musicAutoAccept (bool): some musicPlayer has autoAccept.
//
// Cases: internal/ai/eval/music_cases.json (-cases) and
// music_chat_cases.json (-chat -cases).

func musicPlayers(a *dsl.App) []dsl.Component {
	var out []dsl.Component
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if x.Type == dsl.CompMusicPlayer {
					out = append(out, x)
				}
			}
		}
	}
	return out
}

func checkMusic(exp map[string]any, a *dsl.App) []string {
	var miss []string
	players := musicPlayers(a)
	for key, get := range map[string]func(dsl.Component) bool{
		"musicRequests":   func(c dsl.Component) bool { return c.AllowRequests },
		"musicAutoAccept": func(c dsl.Component) bool { return c.AutoAccept },
	} {
		want, ok := exp[key].(bool)
		if !ok {
			continue
		}
		got := false
		for _, c := range players {
			got = got || get(c)
		}
		if got != want {
			miss = append(miss, fmt.Sprintf("%s=%v (players=%d)", key, got, len(players)))
		}
	}
	return miss
}
