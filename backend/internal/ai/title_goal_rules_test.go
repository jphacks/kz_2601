package ai

import (
	"context"
	"strings"
	"testing"
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
