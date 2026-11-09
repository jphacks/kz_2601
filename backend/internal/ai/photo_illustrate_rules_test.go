package ai

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// A current (non-review) build sends photoField.illustrate (iOS ClientCapabilities).
func init() { currentFeatures = append(currentFeatures, FeatureIllustrate) }

func TestIllustrateRuleReachesPrompt(t *testing.T) {
	if !strings.Contains(systemPrompt, photoIllustrateRules) {
		t.Fatal("systemPrompt lacks the illustrate rule")
	}
	if !strings.Contains(string(GenerationSchema()), `"illustrate"`) {
		t.Fatal("generation schema lacks illustrate")
	}
	if !strings.Contains(photoIllustrateRules, "端末内") {
		t.Fatal("the rule must say the picture is made on the device")
	}
}

// The review build omits the key, so it never gets illustrate (removed, the
// photo field stays).
func TestIllustrateWithheldWithoutFeature(t *testing.T) {
	r := NewClientRestriction(allComponentTypes(), nil, featuresWithout(FeatureIllustrate))
	if !r.NoFeature(FeatureIllustrate) {
		t.Fatal("feature key missing must turn illustrate off")
	}
	out, _, changed := enforceRestriction([]byte(featuresDoc), r)
	if !changed || strings.Contains(string(out), `"illustrate"`) || !strings.Contains(string(out), `"photoField"`) {
		t.Fatalf("enforce: changed=%v out=%s", changed, out)
	}
	if _, err := dsl.Parse(out); err != nil {
		t.Fatal(err)
	}
}
