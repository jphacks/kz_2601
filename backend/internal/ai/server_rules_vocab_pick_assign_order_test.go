package ai

import (
	"strings"
	"testing"
)

// The note on saves in turn reached the assign section in the prompt, once.
func TestServerRulesAssignOrderPrompt(t *testing.T) {
	l := serverRulesAssignOrderLine
	if strings.Count(serverRulesPrompt, l[1]) != 1 || !strings.Contains(serverRulesAssignSection, l[1]) {
		t.Fatal("order line not swapped")
	}
	if !strings.Contains(serverRulesPrompt, serverRulesAssignSection) {
		t.Fatal("assign section not in the prompt")
	}
}
