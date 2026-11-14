package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #425: passby / distance requests pick nearbyShare mode passby, and the
// generation schema carries the new props.
func TestNearbyPassbyRulesInPrompt(t *testing.T) {
	if !strings.Contains(systemPrompt, nearbyPassbyRules) {
		t.Fatal("system prompt lacks the passby rules")
	}
	for _, want := range []string{`mode:"passby"`, "distance:true", "fields"} {
		if !strings.Contains(nearbyPassbyRules, want) {
			t.Errorf("rules lack %q", want)
		}
	}
	schema := string(GenerationSchema())
	for _, want := range []string{`"passby"`, `"distance"`} {
		if !strings.Contains(schema, want) {
			t.Errorf("generation schema lacks %s", want)
		}
	}
}

const passbyDoc = `{"schemaVersion":1,"app":{"name":"すれ違い","icon":"person.2","voiceEntry":"floating"},"initialScreen":"home",
"entities":[{"id":"profile","name":"プロフィール","titleField":"name","fields":[{"id":"name","name":"名前","type":"string","required":true}]}],
"screens":[{"id":"home","title":"すれ違い","entity":"profile","components":[
{"type":"nearbyShare","area":"A1:F6","label":"すれ違い通信","entity":"profile","mode":"passby","fields":["name"],"distance":true},
{"type":"nearbyShare","area":"A7:F8","label":"送る","entity":"profile"}]}]}`

// #1955: disabledFeatures nearbyShare.passby (Mac) keeps nearbyShare send but
// never makes passby; without the value nothing changes.
func TestNearbyPassbyDisabled(t *testing.T) {
	all := allComponentTypes()
	r := NewClientRestriction(all, []string{FeatureNearbyPassby}, currentFeatures)
	if !r.Active() || !r.NoFeature(FeatureNearbyPassby) {
		t.Fatalf("passby not off: active=%v off=%v", r.Active(), r.off)
	}
	if NewRestriction(nil, []string{" " + FeatureNearbyPassby}).NoFeature(FeatureNearbyPassby) != true {
		t.Fatal("disabledFeatures alone (no allowedComponents) must turn passby off")
	}
	p := restrictPrompt(withRestriction(context.Background(), r), systemPrompt)
	if strings.Contains(withoutNote(p), nearbyPassbyRules) || strings.Contains(withoutNote(p), "passby") {
		t.Fatal("restricted prompt still teaches passby")
	}
	if !strings.Contains(p, "この端末ではすれ違い通信は使えない") {
		t.Fatal("note missing")
	}
	raw := restrictSchemaJSON(dsl.SchemaJSON(), r)
	if strings.Contains(string(raw), "passby") {
		t.Fatal("restricted schema still names passby")
	}
	for _, p := range []string{"NearbyShare.fields", "NearbyShare.limit", "NearbyShare.distance"} {
		if schemaHas(t, raw, p) {
			t.Errorf("restricted schema still has %s", p)
		}
	}
	if !schemaHas(t, raw, "NearbyShare.mode") || !strings.Contains(string(raw), `"nearbyShare"`) {
		t.Fatal("nearbyShare send must stay")
	}

	doc := []byte(passbyDoc)
	if v := restrictionViolations(doc, nil, r); len(v) != 1 || !strings.Contains(v[0], "passby") {
		t.Fatalf("violations = %v", v)
	}
	if v := restrictionViolations(doc, doc, r); len(v) != 0 {
		t.Fatalf("existing passby is not new: %v", v)
	}
	out, notes, changed := enforceRestriction(doc, r)
	if !changed || len(notes) != 1 || strings.Contains(string(out), "passby") || !strings.Contains(string(out), `"送る"`) {
		t.Fatalf("enforce: changed=%v notes=%v out=%s", changed, notes, out)
	}
	if _, err := dsl.Parse(out); err != nil {
		t.Fatalf("enforced doc invalid: %v", err)
	}
}

func TestNearbyPassbyKeptWithoutDeclaration(t *testing.T) {
	for _, r := range []Restriction{
		NewClientRestriction(allComponentTypes(), nil, currentFeatures),
		NewClientRestriction(nil, nil, nil),
		NewClientRestriction(allComponentTypes(), []string{FeatureSharedData}, currentFeatures),
	} {
		if r.NoFeature(FeatureNearbyPassby) {
			t.Fatal("passby off without the declaration")
		}
	}
	if NewClientRestriction(allComponentTypes(), nil, currentFeatures).Active() {
		t.Fatal("a current iOS build must stay unrestricted")
	}
}
