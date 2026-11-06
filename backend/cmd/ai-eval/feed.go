package main

import (
	"fmt"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Feed (#348) expect keys, checked by checkFeed:
//   - feed (string): some feed reads this provider (weather|holidays|rss);
//     "none" = no feed at all (e.g. an arbitrary URL that must not become one).
//   - feedRegion (string): some weather feed uses this forecast office code.
//
// Cases: internal/ai/eval/feed_cases.json (-cases) and feed_chat_cases.json
// (-chat -cases).

func checkFeed(exp map[string]any, a *dsl.App) []string {
	want, okP := exp["feed"].(string)
	region, okR := exp["feedRegion"].(string)
	if !okP && !okR {
		return nil
	}
	var providers, regions []string
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if x.Type == dsl.CompFeed {
					providers = append(providers, x.Provider)
					if x.Provider == dsl.FeedWeather {
						regions = append(regions, x.Region)
					}
				}
			}
		}
	}
	var miss []string
	if okP {
		got := want == "none" && len(providers) == 0
		for _, p := range providers {
			got = got || p == want
		}
		if !got {
			miss = append(miss, fmt.Sprintf("feed=%s (got [%s])", want, strings.Join(providers, ",")))
		}
	}
	if okR {
		got := false
		for _, r := range regions {
			got = got || r == region
		}
		if !got {
			miss = append(miss, fmt.Sprintf("feedRegion=%s (got [%s])", region, strings.Join(regions, ",")))
		}
	}
	return miss
}
