package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Route guide (#411) expect keys: routeDest ("entity" | "record" | "address" |
// "any": some routeGuide takes its destination that way; entity = chosen among
// an entity's records, record = the screen's record, address = fixed) and
// routeMode ("automobile" | "walking": some routeGuide starts in that mode,
// "" counts as automobile). Cases: internal/ai/eval/route_cases.json (-cases).

var routeDests = []string{"entity", "record", "address", "any"}

var routeModes = []string{"automobile", "walking"}

// routeGuides lists "dest:mode" of every routeGuide in the app.
func routeGuides(a *dsl.App) []string {
	var out []string
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if x.Type != dsl.CompRouteGuide {
					continue
				}
				dest := "record"
				switch {
				case x.Address != "":
					dest = "address"
				case x.Entity != "" && x.Entity != s.Entity:
					dest = "entity"
				}
				mode := x.Mode
				if mode == "" {
					mode = "automobile"
				}
				out = append(out, dest+":"+mode)
			}
		}
	}
	return out
}

func checkRoute(exp map[string]any, a *dsl.App) []string {
	dest, hasDest := exp["routeDest"].(string)
	mode, hasMode := exp["routeMode"].(string)
	if !hasDest && !hasMode {
		return nil
	}
	got := routeGuides(a)
	ok := slices.ContainsFunc(got, func(g string) bool {
		d, m, _ := strings.Cut(g, ":")
		return (!hasDest || dest == "any" || d == dest) && (!hasMode || m == mode)
	})
	if !ok {
		return []string{fmt.Sprintf("routeGuide=%v (want dest=%s mode=%s)", got, dest, mode)}
	}
	return nil
}
