package ai

import (
	"strings"
	"testing"
)

// #411: the route rules reach the generation prompt, the catalog / chat index
// (both built from the schema) describe routeGuide, and chat no longer lists
// navigation itself as impossible.
func TestRouteRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## 道案内・ナビ(#411)", "mode walking"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	if !strings.Contains(catalog, "\n- routeGuide:") {
		t.Error("catalog lacks the routeGuide line")
	}
	// The three destination shapes live in the catalog's 使い方.
	for _, want := range []string{"routeGuide{entity: spot, locationField: place, titleField: name}", `routeGuide{address: "金沢駅"}`} {
		if !strings.Contains(catalog, want) {
			t.Errorf("catalog lacks %q", want)
		}
	}
	if !strings.Contains(chatCapabilityIndex, "routeGuide") {
		t.Error("chat capability index lacks routeGuide (chat would answer 「ナビはできない」)")
	}
	for _, p := range []string{chatCreateSystemPrompt, chatEditSystemPrompt} {
		if strings.Contains(p, "経路案内・ナビ") {
			t.Error("chat prompt still lists navigation as impossible")
		}
	}
}

func TestRouteGuidePlainTerm(t *testing.T) {
	if got := plainTerms("routeGuide で案内します", LocaleJA); got != "道案内で案内します" {
		t.Errorf("plainTerms = %q", got)
	}
}
