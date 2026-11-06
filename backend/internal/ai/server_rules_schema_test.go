package ai

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #340 / #343: serverRules is in the canonical schema but not in the main
// generation schema (the server-rules pass has its own schema).
func TestGenerationSchemaOmitsServerRules(t *testing.T) {
	var root map[string]any
	if err := json.Unmarshal(dsl.SchemaJSON(), &root); err != nil {
		t.Fatal(err)
	}
	if _, ok := root["properties"].(map[string]any)["serverRules"]; !ok {
		t.Fatal("canonical schema lost serverRules")
	}
	delete(root["properties"].(map[string]any), "serverRules")
	defs := root["$defs"].(map[string]any)
	for _, name := range serverRuleDefs {
		if _, ok := defs[name]; !ok {
			t.Fatalf("$defs/%s missing from the canonical schema", name)
		}
		delete(defs, name)
	}
	without, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	want, err := Relax(without)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(GenerationSchema(), want) {
		t.Fatal("generation schema changed by the server-rule defs")
	}
	if bytes.Contains(GenerationSchema(), []byte("serverRules")) || bytes.Contains(GenerationSchema(), []byte("RuleTrigger")) {
		t.Fatal("generation schema mentions server rules")
	}
}
