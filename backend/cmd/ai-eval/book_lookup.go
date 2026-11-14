package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// lookupBook expect keys, checked by checkBookLookup:
//
//	lookupBook (bool)  a button / toolbar runs lookupBook and maps the title
//
// Cases: internal/ai/eval/book_cases.json and jphacks_coverage_cases.json
// cov_book_lending (-cases).
func init() {
	expectKeys["lookupBook"] = expectBool
}

func checkBookLookup(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["lookupBook"].(bool)
	if !ok {
		return nil
	}
	n := 0
	use := func(act *dsl.Action) {
		if act != nil && act.Type == dsl.ActionLookupBook && act.BookLookup != nil && act.BookLookup.TitleField != "" {
			n++
		}
	}
	for _, s := range a.Screens {
		for i := range s.Toolbar {
			use(&s.Toolbar[i].Action)
		}
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				use(x.Action)
			}
		}
	}
	if (n > 0) != want {
		return []string{fmt.Sprintf("lookupBook=%d", n)}
	}
	return nil
}
