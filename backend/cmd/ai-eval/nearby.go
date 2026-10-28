package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Passby (#425) expect keys, checked by checkNearby:
//
//	passby (bool)         some nearbyShare has mode "passby" with explicit fields
//	passbyDistance (bool) some passby nearbyShare has distance:true
func checkNearby(exp map[string]any, a *dsl.App) []string {
	if a == nil {
		return nil
	}
	var passby, distance []string
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if x.Type != dsl.CompNearbyShare || x.Mode != dsl.NearbyModePassby || len(x.Fields) == 0 {
					continue
				}
				passby = append(passby, s.ID+":"+x.Entity)
				if x.Distance {
					distance = append(distance, s.ID+":"+x.Entity)
				}
			}
		}
	}
	var miss []string
	if want, ok := exp["passby"].(bool); ok && (len(passby) > 0) != want {
		miss = append(miss, fmt.Sprintf("passby=%v", passby))
	}
	if want, ok := exp["passbyDistance"].(bool); ok && (len(distance) > 0) != want {
		miss = append(miss, fmt.Sprintf("passbyDistance=%v", distance))
	}
	return miss
}
