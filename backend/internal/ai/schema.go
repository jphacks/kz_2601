package ai

import (
	"encoding/json"
	"sort"
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
	compact(root)
	return json.Marshal(root)
}

// compact shrinks the grammar Claude has to compile: the 15-way Leaf anyOf
// (expanded once per reference) exceeds the API's grammar size limit. Leaf
// and Component become one flattened object (type enum + every property
// optional) and PatchValue becomes primitives | flat node | arrays. Per-type
// required props and combinations are enforced afterwards by dsl.Parse.
func compact(root map[string]any) {
	defs, _ := root["$defs"].(map[string]any)
	if defs == nil {
		return
	}
	leafNames := refNames(defs["Leaf"])
	if len(leafNames) == 0 {
		return
	}
	leafFlat := mergeObjects(defs, leafNames)
	leafFlat["description"] = "Leaf component. `type` decides which other properties apply (see catalog); do not set unrelated properties."
	defs["Leaf"] = leafFlat

	compFlat := mergeObjects(defs, append([]string{"Container"}, leafNames...))
	compFlat["properties"].(map[string]any)["children"] = map[string]any{
		"type": "array", "items": map[string]any{"$ref": "#/$defs/Leaf"},
		"description": "vstack/hstack/section only. Children stack vertically/horizontally and must not carry `area`.",
	}
	compFlat["description"] = "Screen-level component: a container (vstack/hstack/section with children) or a leaf."
	defs["Component"] = compFlat
	delete(defs, "Container")
	for _, n := range leafNames {
		delete(defs, n)
	}

	if _, ok := defs["PatchValue"]; ok {
		node := mergeObjects(defs, []string{"AppMeta", "Tab", "Theme", "Notification", "Entity", "Field", "Screen", "ToolbarButton", "Action", "Component", "ListItem", "Sort", "Filter"})
		node["description"] = "A DSL sub-document (entity, field, screen, component, action, tab, notification, ...). Set only the properties of that node kind."
		defs["PatchNode"] = node
		defs["PatchValue"] = map[string]any{
			"description": "Primitive, DSL sub-document, or array of either.",
			"anyOf": []any{
				map[string]any{"type": "string"}, map[string]any{"type": "number"}, map[string]any{"type": "boolean"}, map[string]any{"type": "null"},
				map[string]any{"$ref": "#/$defs/PatchNode"},
				map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/PatchNode"}},
			},
		}
	}
	pruneDefs(root)
}

// refNames lists the $defs names referenced by a schema's anyOf.
func refNames(s any) []string {
	m, _ := s.(map[string]any)
	arr, _ := m["anyOf"].([]any)
	var out []string
	for _, v := range arr {
		if sub, ok := v.(map[string]any); ok {
			if ref, ok := sub["$ref"].(string); ok {
				out = append(out, strings.TrimPrefix(ref, "#/$defs/"))
			}
		}
	}
	return out
}

// mergeObjects unions the properties of several object defs. Colliding
// property schemas with enum/const are unioned; otherwise the first wins.
// `required` is reduced to the keys required by every input (usually `type`).
func mergeObjects(defs map[string]any, names []string) map[string]any {
	props := map[string]any{}
	var order []string
	reqCount := map[string]int{}
	for _, n := range names {
		d, _ := defs[n].(map[string]any)
		p, _ := d["properties"].(map[string]any)
		for _, k := range sortedKeys(p) {
			ps, _ := p[k].(map[string]any)
			if cur, ok := props[k].(map[string]any); ok {
				props[k] = mergeProp(cur, ps)
			} else {
				props[k] = mergeProp(nil, ps)
				order = append(order, k)
			}
		}
		if req, ok := d["required"].([]any); ok {
			for _, r := range req {
				reqCount[r.(string)]++
			}
		}
	}
	var required []any
	for _, k := range order {
		if reqCount[k] == len(names) {
			required = append(required, k)
		}
	}
	out := map[string]any{"type": "object", "additionalProperties": false, "properties": props}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

// mergeProp copies b (and unions enum/const into a if present).
func mergeProp(a, b map[string]any) map[string]any {
	cp := map[string]any{}
	for k, v := range b {
		cp[k] = v
	}
	if c, ok := cp["const"]; ok {
		delete(cp, "const")
		cp["type"] = "string"
		cp["enum"] = []any{c}
	}
	if a == nil {
		return cp
	}
	ae, aok := a["enum"].([]any)
	be, bok := cp["enum"].([]any)
	if aok && bok {
		seen := map[any]bool{}
		var union []any
		for _, v := range append(append([]any{}, ae...), be...) {
			if !seen[v] {
				seen[v] = true
				union = append(union, v)
			}
		}
		a["enum"] = union
		return a
	}
	if aok != bok { // enum vs free string: free string wins
		delete(a, "enum")
		if _, ok := a["type"]; !ok {
			a["type"] = "string"
		}
	}
	return a
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// pruneDefs drops $defs not reachable from the root.
func pruneDefs(root map[string]any) {
	defs, _ := root["$defs"].(map[string]any)
	seen := map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			if ref, ok := t["$ref"].(string); ok {
				n := strings.TrimPrefix(ref, "#/$defs/")
				if !seen[n] {
					seen[n] = true
					walk(defs[n])
				}
			}
			for k, sub := range t {
				if k != "$defs" {
					walk(sub)
				}
			}
		case []any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(root)
	for n := range defs {
		if !seen[n] {
			delete(defs, n)
		}
	}
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
