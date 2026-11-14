package ai

import (
	"strings"
	"testing"
)

// placeSearch: the rules reach the generation prompt and the schema-built
// catalog / chat index list the new leaf and the locationButton key.
func TestPlaceSearchRulesReachPrompts(t *testing.T) {
	for _, want := range []string{
		"## 近くの場所を探す(placeSearch)",
		"placeSearch{nameField: name, locationField: place, addressField: address}",
		"locationButton{field: place, addressField: address}",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	for _, want := range []string{"placeSearch", "addressField", "movieTheater"} {
		if !strings.Contains(catalog, want) {
			t.Errorf("catalog lacks %q", want)
		}
	}
	if !strings.Contains(chatCapabilityIndex, "placeSearch") {
		t.Error("chat capability index lacks placeSearch")
	}
}
