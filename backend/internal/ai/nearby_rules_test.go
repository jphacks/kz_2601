package ai

import (
	"strings"
	"testing"
)

// #425: passby / distance requests pick nearbyShare mode passby, and the
// generation schema carries the new props.
func TestNearbyPassbyRulesInPrompt(t *testing.T) {
	if !strings.Contains(systemPrompt, nearbyPassbyRules) {
		t.Fatal("system prompt lacks the passby rules")
	}
	for _, want := range []string{`mode:"passby"`, "distance:true", "fields"} {
		if !strings.Contains(nearbyPassbyRules, want) {
			t.Errorf("rules lack %q", want)
		}
	}
	schema := string(GenerationSchema())
	for _, want := range []string{`"passby"`, `"distance"`} {
		if !strings.Contains(schema, want) {
			t.Errorf("generation schema lacks %s", want)
		}
	}
}
