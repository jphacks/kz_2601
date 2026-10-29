package ai

import (
	"strings"
	"testing"
)

// #417: the generate rules reach the generation prompt, and the catalog
// (built from the schema) lists the generate task and its shape keys.
func TestAIGenerateRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## AI に決まった形で書かせるボタン(#417)", "aiButton{task: generate", "fields + items"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	for _, want := range []string{"|generate", "shape(line|list|fields)", "count", "items", "generate は「〜の案を出してくれるボタン」"} {
		if !strings.Contains(catalog, want) {
			t.Errorf("catalog lacks %q", want)
		}
	}
}
