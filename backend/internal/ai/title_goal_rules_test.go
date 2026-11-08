package ai

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestTitleGoalRules(t *testing.T) {
	if !strings.Contains(systemPrompt, titleGoalSingleLine) {
		t.Fatal("create prompt lacks the goal line")
	}
	// builds without single read the goal record with latest only
	old := NewClientRestriction(allComponentTypes(), nil, []string{FeatureRecordRetention, FeatureNotificationWhen, FeatureMatching})
	if !old.NoSingle() {
		t.Fatal("want a build without singleEntity")
	}
	got := withoutNote(restrictPrompt(withRestriction(context.Background(), old), systemPrompt))
	if strings.Contains(got, titleGoalSingleLine) || !strings.Contains(got, titleGoalLatestLine) {
		t.Error("no-single prompt still tells the model to use \"single\": true for goals")
	}
}

// The goal snippet must validate on a goal / budget entity without a date
// field (goal{title,target_minutes}, budget{amount}): "by":"date" did not.
func TestTitleGoalSnippetWithoutDateField(t *testing.T) {
	snip := regexp.MustCompile(`\{"agg":"latest"[^}]*\}`)
	app := func(value string) []byte {
		return []byte(`{"schemaVersion":1,"app":{"name":"t","voiceEntry":"floating"},"initialScreen":"home",
"entities":[{"id":"budget","name":"予算","fields":[{"id":"amount","name":"予算額","type":"currency"}]}],
"screens":[{"id":"home","title":"ホーム","components":[{"type":"stat","label":"予算","value":` + value + `}]}]}`)
	}
	for name, line := range map[string]string{"single": titleGoalSingleLine, "latest": titleGoalLatestLine} {
		got := snip.FindString(line)
		if got == "" {
			t.Fatalf("%s: no latest snippet in %q", name, line)
		}
		if _, err := dsl.Parse(app(got)); err != nil {
			t.Errorf("%s: snippet %s does not validate without a date field: %v", name, got, err)
		}
	}
	// the old snippet fails there, so the check above is not vacuous
	if _, err := dsl.Parse(app(`{"agg":"latest","entity":"budget","field":"amount","by":"date"}`)); err == nil || !strings.Contains(err.Error(), `latest by "date" not in entity "budget"`) {
		t.Errorf(`"by":"date" on an entity without date must fail on by: %v`, err)
	}
}
