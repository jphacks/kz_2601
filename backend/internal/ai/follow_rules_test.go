package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #270: the follow recipe reaches the generation and chat prompts.
func TestFollowRulesReachPrompts(t *testing.T) {
	if !strings.Contains(systemPrompt, followRules) {
		t.Error("systemPrompt lacks followRules")
	}
	for name, p := range map[string]string{"edit": chatEditSystemPrompt, "create": chatCreateSystemPrompt} {
		if !strings.Contains(p, followChatRule) {
			t.Errorf("%s chat prompt lacks followChatRule", name)
		}
	}
	if !strings.HasSuffix(chatCreateSystemPrompt, chatCapabilityIndex) {
		t.Fatal("chat create prompt no longer ends with the capability index")
	}
	// the recipe's filter and actions are what the schema and Validate accept
	for _, want := range []string{`{"myFollows":true}`, `"type":"follow"`, `"type":"unfollow"`} {
		if !strings.Contains(followRules, want) {
			t.Errorf("followRules lacks %s", want)
		}
	}
	for _, a := range []string{string(dsl.ActionFollow), string(dsl.ActionUnfollow)} {
		if !strings.Contains(chatCapabilityIndex, a+"(") {
			t.Errorf("capability index lacks action %s", a)
		}
	}
}

// Builds without shared data (the review build) get no follow rules.
func TestFollowRulesDroppedWithoutSharedData(t *testing.T) {
	ctx := withRestriction(context.Background(), NewRestriction(nil, []string{FeatureSharedData}))
	for name, p := range map[string]string{"create": systemPrompt, "chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt} {
		got := restrictPrompt(ctx, p)
		if strings.Contains(got, followRules) || strings.Contains(got, followChatRule) {
			t.Errorf("%s: follow rules left for a build without shared data", name)
		}
	}
}
