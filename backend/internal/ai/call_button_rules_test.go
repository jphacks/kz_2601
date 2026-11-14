package ai

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// A current build sends component.callButton (iOS ClientCapabilities).
func init() { currentFeatures = append(currentFeatures, FeatureCallButton) }

// A current build: the one sentence, the catalog line, no decline.
func TestCallButtonRulesReachPrompts(t *testing.T) {
	if !strings.Contains(systemPrompt, callButtonRules) || strings.Contains(systemPrompt, callDeclineRule) {
		t.Error("systemPrompt: callButton sentence in, decline out")
	}
	if !strings.Contains(catalog, "\n- callButton:") || !strings.Contains(chatCapabilityIndex, "callButton") {
		t.Error("catalog / chat index lack callButton")
	}
	for name, p := range map[string]string{"edit": chatEditSystemPrompt, "create": chatCreateSystemPrompt} {
		if strings.Contains(p, callLimitItem) {
			t.Errorf("%s chat prompt still says calls are impossible", name)
		}
	}
	if strings.Count(callButtonRules, "。") != 1 {
		t.Error("the callButton rule is one sentence")
	}
}

// A build without the feature (older builds, the review build) gets the
// decline in the generation prompt and the limit in the chat prompts.
func TestCallButtonWithheldFromOldBuilds(t *testing.T) {
	r := NewClientRestriction(allComponentTypes(), nil, featuresWithout(FeatureCallButton))
	if !r.NoFeature(FeatureCallButton) {
		t.Fatal("feature key missing must turn callButton off")
	}
	ctx := withRestriction(context.Background(), r)
	gen := restrictPrompt(ctx, systemPrompt)
	if strings.Contains(gen, callButtonRules) || !strings.Contains(gen, callDeclineRule) {
		t.Error("generate prompt: decline must replace the callButton sentence")
	}
	for name, p := range map[string]string{"edit": chatEditSystemPrompt, "create": chatCreateSystemPrompt} {
		if got := restrictPrompt(ctx, p); !strings.Contains(got, callLimitItem) {
			t.Errorf("%s chat prompt lacks the call limit", name)
		}
	}
	// a build without the type at all
	var allowed []string
	for typ := range knownComponentTypes {
		if typ != string(dsl.CompCallButton) {
			allowed = append(allowed, typ)
		}
	}
	old := NewClientRestriction(allowed, nil, currentFeatures)
	if !slices.Contains(old.Disallowed(), string(dsl.CompCallButton)) {
		t.Fatalf("a build without the type: disallowed = %v", old.Disallowed())
	}
}

// CALLS_ENABLED off on the server: no build gets callButton, even one that
// sends no allowedComponents.
func TestCallButtonOffWhileCallsDisabled(t *testing.T) {
	s := &Service{NoCalls: true, TextImage: true}
	if r := s.clientRestriction(nil, nil, nil); !r.NoFeature(FeatureCallButton) {
		t.Error("NoCalls must turn callButton off")
	}
	s.NoCalls = false
	if r := s.clientRestriction(allComponentTypes(), nil, currentFeatures); r.NoFeature(FeatureCallButton) {
		t.Error("calls on + current build: callButton stays")
	}
	if !CallsEnabled(func(string) string { return "true" }) || CallsEnabled(func(string) string { return "" }) {
		t.Error("CallsEnabled")
	}
}

// The strip removes callButton from a document of a build without it.
func TestCallButtonStrip(t *testing.T) {
	doc := map[string]any{"screens": []any{map[string]any{"id": "home", "components": []any{
		map[string]any{"type": "text", "text": "hi"},
		map[string]any{"type": "callButton", "label": "話す"},
	}}}}
	var f *clientFeature
	for i := range clientFeatures {
		if clientFeatures[i].key == FeatureCallButton {
			f = &clientFeatures[i]
		}
	}
	if f == nil {
		t.Fatal("no clientFeature")
	}
	if len(f.places(doc)) != 1 || f.strip(doc) != 1 || len(f.places(doc)) != 0 {
		t.Fatal("strip did not remove the callButton")
	}
}

// The call omission: an app with a callButton is the call; without one the
// decline sentence is needed.
func TestCallOmissionWithCallButton(t *testing.T) {
	app, err := dsl.Parse([]byte(`{"schemaVersion":1,"app":{"name":"連絡","icon":"phone","voiceEntry":"floating"},"initialScreen":"home",
"entities":[{"id":"post","name":"連絡","scope":"shared","titleField":"body","fields":[{"id":"body","name":"本文","type":"string","required":true}]}],
"screens":[{"id":"home","title":"連絡","components":[{"type":"chatThread","area":"A1:F8","entity":"post","textField":"body"},{"type":"callButton","area":"C11:F12","video":true}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := omissionIDs(omissions("家族とビデオ通話できるアプリ", app, "", true)); len(got) != 0 {
		t.Errorf("with a callButton: %v", got)
	}
	if got := omissionIDs(omissions("家族とビデオ通話できるアプリ", &dsl.App{}, "", true)); !slices.Equal(got, []string{"call"}) {
		t.Errorf("without: %v", got)
	}
}
