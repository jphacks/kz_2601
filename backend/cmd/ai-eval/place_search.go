package main

import (
	"fmt"
	"slices"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// placeSearch expect keys, checked by checkPlaceSearch:
//
//	placeSearch (string)      some placeSearch fills the name or the place and searches by
//	                          category | query | free (neither) | any
//	locationAddress (bool)    some locationButton has addressField (reverse geocoding)
//
// Cases: internal/ai/eval/place_search_cases.json (-cases).
func init() {
	expectKeys["placeSearch"] = expectString
	expectKeys["locationAddress"] = expectBool
}

var placeSearchKinds = []string{"category", "query", "free", "any"}

// placeSearchUses lists how each placeSearch that fills a name / place searches.
func placeSearchUses(a *dsl.App) (kinds []string, addressButtons int) {
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				switch {
				case x.Type == dsl.CompLocationButton && x.AddressField != "":
					addressButtons++
				case x.Type != dsl.CompPlaceSearch || (x.NameField == "" && x.LocationField == ""):
				case x.Category != "":
					kinds = append(kinds, "category")
				case x.Query != "":
					kinds = append(kinds, "query")
				default:
					kinds = append(kinds, "free")
				}
			}
		}
	}
	return kinds, addressButtons
}

func checkPlaceSearch(exp map[string]any, a *dsl.App) []string {
	want, hasKind := exp["placeSearch"].(string)
	addr, hasAddr := exp["locationAddress"].(bool)
	if !hasKind && !hasAddr {
		return nil
	}
	kinds, buttons := placeSearchUses(a)
	var miss []string
	if hasKind {
		switch {
		case !slices.Contains(placeSearchKinds, want):
			miss = append(miss, fmt.Sprintf("placeSearch=%q is not one of %v", want, placeSearchKinds))
		case len(kinds) == 0 || (want != "any" && !slices.Contains(kinds, want)):
			miss = append(miss, fmt.Sprintf("placeSearch=%v (want %s)", kinds, want))
		}
	}
	if hasAddr && (buttons > 0) != addr {
		miss = append(miss, fmt.Sprintf("locationAddress=%d", buttons))
	}
	return miss
}
