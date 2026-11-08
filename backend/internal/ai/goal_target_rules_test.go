package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// The goal rule is in the prompt with a whole, valid stat component, and a
// build without single reads the goal with agg latest instead.
func TestGoalTargetRules_InPrompt(t *testing.T) {
	for _, want := range []string{"## 変えられる目標と進み具合のバー", goalRateSetting} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	for _, s := range []string{goalRateSetting, goalRateLatest} {
		var c map[string]any
		if err := json.Unmarshal([]byte(s), &c); err != nil {
			t.Fatalf("snippet is not JSON: %v\n%s", err, s)
		}
		if c["type"] != "stat" || c["format"] != "percent" || c["value"] == nil {
			t.Errorf("snippet = %v", c)
		}
	}
	all := make([]string, 0, len(knownComponentTypes))
	for k := range knownComponentTypes {
		all = append(all, k)
	}
	old := NewClientRestriction(all, nil, nil)
	if !old.NoSingle() {
		t.Fatal("restriction without single")
	}
	got := restrictPrompt(withRestriction(context.Background(), old), systemPrompt)
	if strings.Contains(got, goalRateSetting) || !strings.Contains(got, goalRateLatest) {
		t.Error("a build without single still reads the goal with setting")
	}
}
