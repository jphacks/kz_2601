package ai

import (
	"strings"
	"testing"
)

// #250 #251: shared entities show the poster with $author (no typed-in name
// field) and "only mine" is a component filter with value {"me":true}.
func TestPromptTeachesAuthorAndMe(t *testing.T) {
	for _, want := range []string{
		"名前の手入力欄を作らない",
		`"$author"`,
		`{"field":"$author","op":"eq","value":{"me":true}}`,
		`"$createdAt"`,
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
}
