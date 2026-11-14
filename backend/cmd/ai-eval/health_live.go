package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// healthStat live (#3694) expect key, checked by checkHealthLive:
//
//	healthLive (bool) some healthStat has live: true (false = none has it)
//
// Cases: internal/ai/eval/health_live_cases.json (-cases).

func healthLiveCount(a *dsl.App) int {
	n := 0
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if x.Type == dsl.CompHealthStat && x.Live {
					n++
				}
			}
		}
	}
	return n
}

func checkHealthLive(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["healthLive"].(bool)
	if !ok {
		return nil
	}
	if n := healthLiveCount(a); (n > 0) != want {
		return []string{fmt.Sprintf("healthLive=%d", n)}
	}
	return nil
}
