package ai

import (
	"encoding/json"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Claude structured outputs reject some JSON Schema keywords (pattern,
// format, numeric/length constraints). The "generation schema" is the
// canonical schema with those stripped; Go's dsl.Parse stays the strict gate.
var strippedKeywords = map[string]bool{
	"pattern": true, "format": true,
	"minLength": true, "maxLength": true,
	"minimum": true, "maximum": true, "exclusiveMinimum": true, "exclusiveMaximum": true, "multipleOf": true,
	"minItems": true, "maxItems": true, "uniqueItems": true,
	"minProperties": true, "maxProperties": true,
}

var (
	generationSchema      = mustRelax(dsl.SchemaJSON())
	patchGenerationSchema = mustRelax(dsl.PatchSchemaJSON())
)

// GenerationSchema is the relaxed AppDSL schema sent as output_config.format.
func GenerationSchema() json.RawMessage { return generationSchema }

// PatchGenerationSchema is the relaxed PatchDocument schema for edit turns.
func PatchGenerationSchema() json.RawMessage { return patchGenerationSchema }

func mustRelax(raw []byte) json.RawMessage {
	out, err := Relax(raw)
	if err != nil {
		panic("ai: relax schema: " + err.Error())
	}
	return out
}

// Relax derives the generation schema: strips unsupported keywords, drops
// $schema/$id, and inlines a root-level $ref (the API wants an object root).
func Relax(raw []byte) (json.RawMessage, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	delete(root, "$schema")
	delete(root, "$id")
	if ref, ok := root["$ref"].(string); ok {
		defs, _ := root["$defs"].(map[string]any)
		name := strings.TrimPrefix(ref, "#/$defs/")
		if target, ok := defs[name].(map[string]any); ok {
			inlined := map[string]any{}
			for k, v := range target {
				inlined[k] = v
			}
			for _, k := range []string{"title", "description", "$defs"} {
				if v, ok := root[k]; ok {
					inlined[k] = v
				}
			}
			root = inlined
		}
	}
	relaxSchema(root)
	return json.Marshal(root)
}

// relaxSchema walks one schema object. Keys under `properties`/`$defs` are
// names, not keywords, so they are recursed into but never stripped.
func relaxSchema(s map[string]any) {
	for k := range s {
		if strippedKeywords[k] {
			delete(s, k)
		}
	}
	for _, key := range []string{"properties", "$defs", "definitions", "patternProperties"} {
		if m, ok := s[key].(map[string]any); ok {
			for _, v := range m {
				if sub, ok := v.(map[string]any); ok {
					relaxSchema(sub)
				}
			}
		}
	}
	for _, key := range []string{"items", "not", "additionalItems", "contains", "propertyNames", "if", "then", "else"} {
		if sub, ok := s[key].(map[string]any); ok {
			relaxSchema(sub)
		}
	}
	for _, key := range []string{"anyOf", "oneOf", "allOf", "prefixItems"} {
		if arr, ok := s[key].([]any); ok {
			for _, v := range arr {
				if sub, ok := v.(map[string]any); ok {
					relaxSchema(sub)
				}
			}
		}
	}
}

// stripAreas removes every `area` from screen-level components so the
// layout falls back to "stack from the top" (deterministic, never overlaps).
func stripAreas(raw []byte) ([]byte, bool) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, false
	}
	screens, _ := doc["screens"].([]any)
	changed := false
	for _, s := range screens {
		sm, _ := s.(map[string]any)
		comps, _ := sm["components"].([]any)
		for _, c := range comps {
			cm, _ := c.(map[string]any)
			if _, ok := cm["area"]; ok {
				delete(cm, "area")
				changed = true
			}
			children, _ := cm["children"].([]any)
			for _, ch := range children {
				chm, _ := ch.(map[string]any)
				if _, ok := chm["area"]; ok {
					delete(chm, "area")
					changed = true
				}
			}
		}
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, false
	}
	return out, changed
}
