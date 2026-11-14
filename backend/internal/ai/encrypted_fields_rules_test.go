package ai

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// A current (non-review) build sends field.encrypted (iOS ClientCapabilities).
func init() { currentFeatures = append(currentFeatures, FeatureEncryptedFields) }

func TestEncryptedFieldsRuleReachesPrompt(t *testing.T) {
	if !strings.Contains(systemPrompt, encryptedFieldsRules) {
		t.Fatal("systemPrompt lacks the encrypted rule")
	}
	if !strings.Contains(string(GenerationSchema()), `"encrypted"`) {
		t.Fatal("generation schema lacks encrypted")
	}
}

// A build without the key never gets encrypted (the key goes, the field stays).
func TestEncryptedFieldsWithheldWithoutFeature(t *testing.T) {
	r := NewClientRestriction(allComponentTypes(), nil, featuresWithout(FeatureEncryptedFields))
	out, _, changed := enforceRestriction([]byte(featuresPrivateDoc), r)
	if !changed || strings.Contains(string(out), `"encrypted"`) || !strings.Contains(string(out), `"impression"`) {
		t.Fatalf("enforce: changed=%v out=%s", changed, out)
	}
	if _, err := dsl.Parse(out); err != nil {
		t.Fatal(err)
	}
}
