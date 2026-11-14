package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Current builds (the review build too) send openUrlButton.app (iOS ClientCapabilities).
func init() { currentFeatures = append(currentFeatures, FeatureOpenApp) }

func TestOpenAppRuleReachesPrompt(t *testing.T) {
	if !strings.Contains(systemPrompt, openAppRules) || !strings.Contains(systemPrompt, paypayNoLinkRule) {
		t.Fatal("systemPrompt lacks the open-app rule / the PayPay no-link rule")
	}
	if strings.Contains(systemPrompt, paypayNoLinkRuleOff) {
		t.Fatal("the full prompt must not keep #413's no-button sentence")
	}
	if !strings.Contains(systemPrompt, paypaySchemeRule) || strings.Contains(systemPrompt, paypaySchemeRuleOff) {
		t.Fatal("the full prompt must send apps to openUrlButton app, not #413's https-only sentence")
	}
	sch := string(GenerationSchema())
	for _, want := range []string{`"app"`, `"paypay"`, `"instagram"`} {
		if !strings.Contains(sch, want) {
			t.Fatalf("generation schema lacks %s", want)
		}
	}
	for _, want := range []string{"PayPay", "LINE", "Slack", "Discord", "shareTo", "作らない"} {
		if !strings.Contains(openAppRules, want) {
			t.Fatalf("rule lacks %q", want)
		}
	}
}

// Without the key: #413's sentence comes back, the schema loses app / to and
// shareTo x / mail, a PayPay button without url is dropped and shareTo x
// falls back to the share sheet.
func TestOpenAppWithheldWithoutFeature(t *testing.T) {
	r := NewClientRestriction(allComponentTypes(), nil, featuresWithout(FeatureOpenApp))
	p := restrictPrompt(withRestriction(context.Background(), r), systemPrompt)
	if !strings.Contains(p, paypayNoLinkRuleOff) || strings.Contains(p, paypayNoLinkRule) {
		t.Fatal("restricted prompt must restore #413's no-link rule")
	}
	if !strings.Contains(p, paypaySchemeRuleOff) || strings.Contains(p, paypaySchemeRule) {
		t.Fatal("restricted prompt must restore #413's https-only sentence")
	}
	sch := string(restrictSchemaJSON(dsl.SchemaJSON(), r))
	if strings.Contains(sch, `"instagram"`) || strings.Contains(sch, `"mail",`) {
		t.Fatal("restricted schema keeps app / shareTo mail")
	}
	out, _, changed := enforceRestriction([]byte(featuresDoc), r)
	s := string(out)
	if !changed || strings.Contains(s, `"paypay"`) || strings.Contains(s, `"shareTo"`) || !strings.Contains(s, `"shareButton"`) {
		t.Fatalf("enforce: changed=%v out=%s", changed, out)
	}
	if _, err := dsl.Parse(out); err != nil {
		t.Fatal(err)
	}
}
