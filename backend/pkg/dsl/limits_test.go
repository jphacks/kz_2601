package dsl

import (
	"encoding/json"
	"strings"
	"testing"
)

// Extreme AI output: huge area rows and deep nesting are rejected before
// clients render them (iOS also clamps rows to 48 and stops drawing deep trees).

func TestParseAreaRejectsHugeRows(t *testing.T) {
	for _, bad := range []string{"A1:F49", "A1:F100000000", "A1:F99999999999999999999999", "A99999999999999999999999:F1"} {
		if _, err := ParseArea(bad); err == nil {
			t.Errorf("ParseArea(%q) should fail", bad)
		}
	}
	if _, err := ParseArea("A1:F48"); err != nil {
		t.Errorf("A1:F48 should pass: %v", err)
	}
}

func TestValidateRejectsNestedContainers(t *testing.T) {
	app := Initial()
	app.Screens[0].Components = []Component{{Type: CompVStack, ID: "outer", Children: []Component{
		{Type: CompVStack, ID: "inner", Children: []Component{{Type: CompText, Text: "x"}}},
	}}}
	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), "containers cannot be nested") {
		t.Fatalf("want nested container error, got %v", err)
	}
}

func TestParseRejectsDeepNesting(t *testing.T) {
	raw, err := json.Marshal(Initial())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(raw); err != nil {
		t.Fatalf("baseline should parse: %v", err)
	}
	inner := `{"type":"text","text":"x"}`
	for range 300 {
		inner = `{"type":"vstack","children":[` + inner + `]}`
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var comp any
	if err := json.Unmarshal([]byte(inner), &comp); err != nil {
		t.Fatal(err)
	}
	doc["screens"].([]any)[0].(map[string]any)["components"] = []any{comp}
	deep, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(deep); err == nil {
		t.Fatal("300-level nesting should be rejected")
	}
}
