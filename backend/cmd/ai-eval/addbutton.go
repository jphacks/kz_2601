package main

import "github.com/jphacks/kz_2601/backend/pkg/dsl"

// dupAddToolbars lists "screen>toolbar label" for every toolbar ＋ that opens
// the same form as a button on its screen (#416: two add controls).
func dupAddToolbars(a *dsl.App) []string {
	var out []string
	for i := range a.Screens {
		s := &a.Screens[i]
		for _, j := range a.DuplicateAddToolbar(s) {
			out = append(out, s.ID+">"+s.Toolbar[j].Label)
		}
	}
	return out
}
