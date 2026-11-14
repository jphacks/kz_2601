package ai

import (
	"slices"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// A current build sends component.bracket (iOS ClientCapabilities).
func init() { currentFeatures = append(currentFeatures, FeatureBracket) }

// The bracket rules reach the generation prompt and the catalog / chat index
// (both built from the schema) describe bracket.
func TestBracketRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## トーナメント表(bracket)", "bracket{entity: entrant, titleField: name, winsField: wins", "list・table で代替しない"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	if !strings.Contains(catalog, "\n- bracket: entity, titleField, winsField") {
		t.Error("catalog lacks the bracket line with its required keys")
	}
	if !strings.Contains(chatCapabilityIndex, "bracket") {
		t.Error("chat capability index lacks bracket")
	}
}

// The rule's example shape is a valid, lint-clean app.
func TestBracketRuleExampleIsValid(t *testing.T) {
	doc := `{"schemaVersion":1,"app":{"name":"卓球大会","icon":"trophy","voiceEntry":"floating"},"initialScreen":"home",
"entities":[{"id":"entrant","name":"出場者","titleField":"name","fields":[
 {"id":"name","name":"名前","type":"string","required":true},
 {"id":"wins","name":"勝ち上がり","type":"integer","defaultValue":0},
 {"id":"seed","name":"シード","type":"integer"}]}],
"screens":[
 {"id":"home","title":"トーナメント","components":[
  {"type":"bracket","area":"A1:F7","entity":"entrant","titleField":"name","winsField":"wins","seedField":"seed"},
  {"type":"list","area":"A8:F10","entity":"entrant","item":{"title":"name"},"onItemTap":{"type":"navigate","screen":"form","presentation":"sheet"}},
  {"type":"button","area":"D11:F12","label":"出場者を追加","action":{"type":"navigate","screen":"form","presentation":"sheet"}}]},
 {"id":"form","title":"出場者","entity":"entrant","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"entrant"}}],"components":[
  {"type":"textField","area":"A1:F2","field":"name"},
  {"type":"numberField","area":"A3:F4","field":"seed"}]}]}`
	app, err := dsl.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if probs := app.Lint(); len(probs) > 0 {
		t.Errorf("lint: %v", probs)
	}
}

// An old build that lists bracket in allowedComponents but not the feature
// key still never gets it (and a build without the type at all neither).
func TestBracketWithheldFromOldBuilds(t *testing.T) {
	r := NewClientRestriction(allComponentTypes(), nil, featuresWithout(FeatureBracket))
	if !r.NoFeature(FeatureBracket) {
		t.Fatal("feature key missing must turn bracket off")
	}
	var allowed []string
	for typ := range knownComponentTypes {
		if typ != string(dsl.CompBracket) {
			allowed = append(allowed, typ)
		}
	}
	old := NewClientRestriction(allowed, nil, currentFeatures)
	if !slices.Contains(old.Disallowed(), string(dsl.CompBracket)) {
		t.Fatalf("a build without the type: disallowed = %v", old.Disallowed())
	}
}

// The draw freezes at the first result, so the rule must say late entrants
// stay out and name startedField (the stored start time).
func TestBracketRuleStatesFreeze(t *testing.T) {
	for _, want := range []string{"後から足した人は表に入らない", "startedField: started"} {
		if !strings.Contains(bracketRules, want) {
			t.Errorf("bracket rule must contain %q", want)
		}
	}
}
