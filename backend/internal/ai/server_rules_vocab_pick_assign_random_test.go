package ai

import (
	"strings"
	"testing"
)

// The random-vs-fewest bullet reached the assign section in the prompt, once.
func TestServerRulesAssignRandomPrompt(t *testing.T) {
	want := "「当番をランダムに 1 人選んで」「くじで決めて」は当番・担当でも order: random(fewest にしない)"
	if strings.Count(serverRulesPrompt, want) != 1 || !strings.Contains(serverRulesAssignSection, want) {
		t.Fatal("random bullet not in the assign section")
	}
	if !strings.Contains(serverRulesPrompt, serverRulesAssignSection) {
		t.Fatal("assign section not in the prompt")
	}
}
