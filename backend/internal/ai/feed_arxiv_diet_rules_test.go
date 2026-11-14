package ai

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestFeedArxivDietRules(t *testing.T) {
	for _, want := range []string{"feed{provider: arxiv, feedId:", "feed{provider: diet, query:"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	for _, ln := range strings.Split(feedArxivDietRules, "\n") {
		if !strings.Contains(ln, "」は feed") {
			t.Errorf("rule line is not dropped by the restriction: %q", ln)
		}
	}
	// Every category the rule names is on the allowlist and vice versa.
	for id := range dsl.ArxivCategories {
		if !strings.Contains(feedArxivDietRules, id) {
			t.Errorf("rule does not name arxiv category %s", id)
		}
	}
	var allowed []string
	for typ := range knownComponentTypes {
		if typ != "feed" {
			allowed = append(allowed, typ)
		}
	}
	got := buildRestrictedPrompt(systemPrompt, NewRestriction(allowed, nil))
	for _, gone := range []string{"provider: arxiv", "provider: diet"} {
		if strings.Contains(got, gone) {
			t.Errorf("restricted prompt still has %q", gone)
		}
	}
}
