package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// growth expect keys, checked by checkGrowth:
//
//	growth      (string) some growth has this kind (tree / flower / plant; "" in the DSL = tree)
//	growthSteps (bool)   some growth reads device steps live (metric steps + live: true)
//	growthOpen  (bool)   some growth counts steps from when it appeared (since open)
//
// Cases: internal/ai/eval/growth_cases.json (-cases).

func growthParts(a *dsl.App) []dsl.Component {
	var out []dsl.Component
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if x.Type == dsl.CompGrowth {
					out = append(out, x)
				}
			}
		}
	}
	return out
}

func checkGrowth(exp map[string]any, a *dsl.App) []string {
	var miss []string
	parts := growthParts(a)
	if want, ok := exp["growth"].(string); ok {
		found := false
		for _, c := range parts {
			kind := c.Kind
			if kind == "" {
				kind = "tree"
			}
			found = found || kind == want
		}
		if !found {
			miss = append(miss, fmt.Sprintf("growth=%d(no %s)", len(parts), want))
		}
	}
	if want, ok := exp["growthSteps"].(bool); ok {
		n := 0
		for _, c := range parts {
			if c.Metric == "steps" && c.Live {
				n++
			}
		}
		if (n > 0) != want {
			miss = append(miss, fmt.Sprintf("growthSteps=%d", n))
		}
	}
	if want, ok := exp["growthOpen"].(bool); ok {
		n := 0
		for _, c := range parts {
			if c.Metric == "steps" && c.Since == "open" {
				n++
			}
		}
		if (n > 0) != want {
			miss = append(miss, fmt.Sprintf("growthOpen=%d", n))
		}
	}
	return miss
}
