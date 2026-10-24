package ai

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func findKeyword(t *testing.T, v any, path string, bad map[string]bool, hits *[]string) {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		if arr, ok := v.([]any); ok {
			for i, e := range arr {
				findKeyword(t, e, path+"/"+string(rune('0'+i)), bad, hits)
			}
		}
		return
	}
	for k, sub := range m {
		if bad[k] && k != "format" {
			*hits = append(*hits, path+"/"+k)
		}
		// `format` is also a legitimate property name (Stat.format).
		if k == "format" {
			if _, isSchema := sub.(map[string]any); !isSchema {
				*hits = append(*hits, path+"/"+k)
			}
		}
		if k == "properties" || k == "$defs" {
			for name, child := range sub.(map[string]any) {
				findKeyword(t, child, path+"/"+k+"/"+name, bad, hits)
			}
			continue
		}
		findKeyword(t, sub, path+"/"+k, bad, hits)
	}
}

func TestGenerationSchemaHasNoUnsupportedKeywords(t *testing.T) {
	for name, raw := range map[string]json.RawMessage{"app": GenerationSchema(), "patch": PatchGenerationSchema()} {
		var v map[string]any
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, ok := v["$schema"]; ok {
			t.Errorf("%s: $schema should be dropped", name)
		}
		if _, ok := v["$ref"]; ok {
			t.Errorf("%s: root $ref must be inlined", name)
		}
		if v["type"] != "object" {
			t.Errorf("%s: root must be an object, got %v", name, v["type"])
		}
		var hits []string
		findKeyword(t, v, "", strippedKeywords, &hits)
		if len(hits) > 0 {
			t.Errorf("%s: unsupported keywords remain: %v", name, hits)
		}
	}
	var p map[string]any
	_ = json.Unmarshal(PatchGenerationSchema(), &p)
	props := p["properties"].(map[string]any)
	if props["patch"] == nil || props["summary"] == nil {
		t.Fatal("patch schema root must expose patch and summary")
	}
	// Leaf is flattened: one object whose `type` enum covers all 15 leaves,
	// and the Stat `format` property survives (only the keyword is stripped).
	leaf := p["$defs"].(map[string]any)["Leaf"].(map[string]any)
	lp := leaf["properties"].(map[string]any)
	if lp["format"] == nil || lp["text"] == nil || lp["entity"] == nil {
		t.Fatalf("flattened leaf misses properties: %v", lp)
	}
	if en := lp["type"].(map[string]any)["enum"].([]any); len(en) < 15 { // 15 = v1 baseline; batches append
		t.Fatalf("leaf type enum: %v", en)
	}
	if _, ok := lp["children"]; ok {
		t.Fatal("leaf must not have children")
	}
	comp := p["$defs"].(map[string]any)["Component"].(map[string]any)["properties"].(map[string]any)
	if comp["children"] == nil || len(comp["type"].(map[string]any)["enum"].([]any)) < 18 {
		t.Fatalf("component: %v", comp["type"])
	}
	for _, gone := range []string{"Container", "Text", "Stat"} {
		if _, ok := p["$defs"].(map[string]any)[gone]; ok {
			t.Fatalf("%s should be pruned", gone)
		}
	}
	// Schemas stay small enough for the API grammar compiler.
	if n := len(GenerationSchema()); n > 20000 {
		t.Fatalf("generation schema too large: %d bytes", n)
	}
}

// The relaxed schemas must still accept the canonical examples (they are a
// superset of the strict schema), otherwise Claude could never emit them.
func TestRelaxedSchemaAcceptsExamples(t *testing.T) {
	c := jsonschema.NewCompiler()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(GenerationSchema()))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AddResource("gen.json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("gen.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"initial", "kakeibo", "training", "walk"} {
		raw, err := os.ReadFile("../../../docs/dsl/examples/" + name + ".json")
		if err != nil {
			t.Skip(err)
		}
		v, _ := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err := s.Validate(v); err != nil {
			t.Errorf("%s rejected by relaxed schema: %v", name, err)
		}
	}
}

func TestExtractJSON(t *testing.T) {
	for _, in := range []string{`{"a":1}`, "```json\n{\"a\":1}\n```", "はい、こちらです:\n{\"a\":1}\n以上"} {
		if got := string(extractJSON(in)); got != `{"a":1}` {
			t.Fatalf("extractJSON(%q) = %q", in, got)
		}
	}
}

func TestStripAreas(t *testing.T) {
	raw, _ := os.ReadFile("examples/kakeibo.json")
	out, changed := stripAreas(raw)
	if !changed || strings.Contains(string(out), `"area"`) {
		t.Fatal("areas not stripped")
	}
	again, changed := stripAreas(out)
	if changed || !bytes.Equal(again, out) {
		t.Fatal("second strip should be a no-op")
	}
}

func TestFewShotMatchesDocsExample(t *testing.T) {
	docs, err := os.ReadFile("../../../docs/dsl/examples/kakeibo.json")
	if err != nil {
		t.Skip("docs example not available: ", err)
	}
	if string(docs) != fewShotKakeibo {
		t.Fatal("internal/ai/examples/kakeibo.json is out of sync with docs/dsl/examples/kakeibo.json")
	}
	if !strings.Contains(systemPrompt, `"name": "家計簿"`) || !strings.Contains(systemPrompt, "6 列") {
		t.Fatal("system prompt must embed the few-shot and the grid rules")
	}
}
