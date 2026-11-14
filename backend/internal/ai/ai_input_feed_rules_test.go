package ai

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestAIInputFeedRulesInPrompt(t *testing.T) {
	if !strings.Contains(systemPrompt, "inputFeeds: [その feed の id]") {
		t.Fatal("inputFeeds rules are not in the system prompt")
	}
	if !strings.Contains(string(GenerationSchema()), `"inputFeeds"`) {
		t.Fatal("generation schema lacks inputFeeds")
	}
}

// A build without feed drops every inputFeeds line with the feed rules.
func TestAIInputFeedRulesDroppedWithoutFeed(t *testing.T) {
	for _, ln := range strings.Split(aiInputFeedRules, "\n") {
		if !strings.Contains(ln, "」は feed") {
			t.Errorf("rule line is not dropped by the restriction: %q", ln)
		}
	}
	var allowed []string
	for typ := range knownComponentTypes {
		if typ != "feed" {
			allowed = append(allowed, typ)
		}
	}
	if got := buildRestrictedPrompt(systemPrompt, NewRestriction(allowed, nil)); strings.Contains(got, "inputFeeds: [") {
		t.Error("restricted prompt still has the inputFeeds rules")
	}
}

// The prompt's example (weather + pressure advice) is valid DSL.
func TestAIInputFeedRulesExampleValid(t *testing.T) {
	raw := `{"schemaVersion":1,"app":{"name":"体調","voiceEntry":"none"},"initialScreen":"home","entities":[],
"screens":[{"id":"home","title":"体調","components":[
 {"type":"feed","id":"wx","provider":"weather","region":"170000","title":"石川県の天気","limit":3,"area":"A1:F5"},
 {"type":"feed","id":"hpa","provider":"pressure","region":"170000","title":"石川県の気圧","limit":8,"area":"A6:F12"},
 {"type":"aiButton","label":"体調のアドバイス","task":"generate","prompt":"天気と気圧の変化から今日の体調の注意点と過ごし方","shape":"list","count":3,"inputFeeds":["wx","hpa"],"area":"A13:F18"}]}]}`
	if _, err := dsl.Parse([]byte(raw)); err != nil {
		t.Fatalf("example rejected: %v", err)
	}
}
