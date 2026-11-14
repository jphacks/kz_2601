package ai

import (
	"strings"
	"testing"
)

// lookupBook: the rules reach the generation prompt and the schema-built
// catalog / chat index list the new action and its keys.
func TestBookRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## ISBN から本の情報を入れる(lookupBook)", "type: lookupBook", "isbnField", "qrScanner{field: isbn}"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	for _, want := range []string{"- lookupBook{", "bookLookup"} {
		if !strings.Contains(catalog, want) {
			t.Errorf("catalog lacks %q", want)
		}
	}
	if !strings.Contains(chatCapabilityIndex, "lookupBook") {
		t.Error("chat capability index lacks lookupBook")
	}
}
