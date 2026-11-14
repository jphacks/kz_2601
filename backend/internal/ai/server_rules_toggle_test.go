package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

// #347: "止めて" turns a rule off instead of removing it: the base line is
// swapped, the section is in the prompt and the pass's schema lets the
// model write enabled.
func TestServerRulesTogglePrompt(t *testing.T) {
	if strings.Contains(serverRulesPrompt, serverRulesToggleLine[0]) || !strings.Contains(serverRulesPrompt, serverRulesToggleLine[1]) {
		t.Fatal("toggle line not swapped")
	}
	if !strings.Contains(serverRulesPrompt, serverRulesToggleSection) {
		t.Fatal("toggle section not appended")
	}
	var s struct {
		Defs map[string]struct {
			Properties map[string]struct {
				Type any `json:"type"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(ServerRulesGenerationSchema(), &s); err != nil {
		t.Fatal(err)
	}
	if p, ok := s.Defs["ServerRule"].Properties["enabled"]; !ok || p.Type != "boolean" {
		t.Fatalf("ServerRule.enabled in the pass schema: %+v", s.Defs["ServerRule"].Properties)
	}
}
