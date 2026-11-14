package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// text.highlight expect key: textHighlight (string) = some text marks words
// from words (fixed) | field (a field of the open record) | any.
// Case: internal/ai/eval/jphacks_coverage_cases.json cov_error_query (E_2202).
func init() {
	expectKeys["textHighlight"] = expectString
}

// textHighlightSources lists "words" / "field" for every text with highlight.
func textHighlightSources(a *dsl.App) []string {
	var out []string
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				switch h := x.Highlight; {
				case x.Type != dsl.CompText || h == nil:
				case h.Field != "":
					out = append(out, "field")
				case len(h.Words) > 0:
					out = append(out, "words")
				}
			}
		}
	}
	return out
}

func checkTextHighlight(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["textHighlight"].(string)
	if !ok {
		return nil
	}
	if want != "words" && want != "field" && want != "any" {
		return []string{"textHighlight must be words|field|any: " + want}
	}
	got := textHighlightSources(a)
	for _, g := range got {
		if want == "any" || want == g {
			return nil
		}
	}
	return []string{fmt.Sprintf("textHighlight=%v (want %s)", got, want)}
}
