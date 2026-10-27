package main

import "github.com/jphacks/kz_2601/backend/pkg/dsl"

// prefillRefs lists "screen>target.field" for every navigate with prefillRef
// (#256: a child form opened from its parent).
func prefillRefs(a *dsl.App) []string {
	var out []string
	add := func(from string, act *dsl.Action) {
		if act != nil && act.Type == dsl.ActionNavigate && act.PrefillRef != "" {
			out = append(out, from+">"+act.Screen+"."+act.PrefillRef)
		}
	}
	for _, s := range a.Screens {
		for i := range s.Toolbar {
			add(s.ID, &s.Toolbar[i].Action)
		}
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				add(s.ID, x.Action)
				add(s.ID, x.OnItemTap)
			}
		}
	}
	return out
}
