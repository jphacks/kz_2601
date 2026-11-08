package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
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

// The no-single snippet validates against a goal entity with no date field
// (goal{title,target_minutes}, the common shape): it must not name a "by".
func TestGoalTargetRules_LatestWithoutDateField(t *testing.T) {
	raw := `{"schemaVersion":1,"app":{"name":"勉強","voiceEntry":"floating"},"initialScreen":"home",
"entities":[
 {"id":"study","name":"勉強","fields":[{"id":"minutes","name":"分","type":"integer"},{"id":"date","name":"日付","type":"date","defaultValue":"now"}]},
 {"id":"goal","name":"目標","fields":[{"id":"title","name":"名前","type":"string"},{"id":"target_minutes","name":"目標(分)","type":"integer"}]}],
"screens":[{"id":"home","title":"ホーム","components":[` + goalRateLatest + `]}]}`
	if _, err := dsl.Parse([]byte(raw)); err != nil {
		t.Fatalf("goalRateLatest rejected for a goal entity without a date field: %v", err)
	}
}
