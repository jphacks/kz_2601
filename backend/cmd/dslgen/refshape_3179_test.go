package main

import (
	"strings"
	"testing"
)

// defsGenerator_3179 は $defs だけを持つ小さな generator(build は回さない)。
func defsGenerator_3179(t *testing.T, defs string) *generator {
	t.Helper()
	root, err := parseOrdered([]byte(`{"$defs": ` + defs + `}`))
	if err != nil {
		t.Fatal(err)
	}
	return &generator{
		root:      root,
		defs:      root.obj("$defs"),
		leafSet:   map[string]bool{"Slider": true},
		byName:    map[string]*class{},
		building:  map[string]bool{},
		sealedSet: map[string]*sealedDef{},
		lenient:   map[string]bool{},
	}
}

const refShapeDefs_3179 = `{
  "Component": {"type": "object"},
  "Leaf": {"anyOf": []},
  "Container": {"type": "object"},
  "Slider": {"type": "object", "properties": {"min": {"type": "number"}}},
  "Expr": {"anyOf": [{"type": "number"}]},
  "Either": {"oneOf": [{"type": "string"}, {"type": "number"}]},
  "Title": {"type": "string"},
  "Mode": {"type": "string", "enum": ["a", "b"]},
  "Count": {"type": "integer"},
  "Ratio": {"type": "number"},
  "Flag": {"type": "boolean"},
  "Meta": {"type": "object", "properties": {"id": {"type": "string"}}},
  "Bag": {"type": "object"},
  "Listy": {"type": "array", "items": {"type": "string"}},
  "Untyped": {}
}`

func TestRefShape_3179(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		def       string
		wantKind  kind
		wantClass string
		wantEnum  string // fixedName
	}{
		{"Component", "Component", kComponent, "", ""},
		{"Leaf", "Leaf", kComponent, "", ""},
		{"Container", "Container", kComponent, "", ""},
		{"leaf の 1 つ", "Slider", kComponent, "", ""},
		{"sealed の Expr", "Expr", kSealed, "Expr", ""},
		{"anyOf は JsonElement", "Either", kJSON, "", ""},
		{"文字", "Title", kString, "", ""},
		{"文字の enum は def の名前を持つ", "Mode", kEnum, "", "Mode"},
		{"整数", "Count", kInt, "", ""},
		{"数", "Ratio", kNumber, "", ""},
		{"真偽", "Flag", kBool, "", ""},
		{"properties のある object は class", "Meta", kClass, "Meta", ""},
		{"properties の無い object は JsonObject", "Bag", kJSONObject, "", ""},
		{"array の def は JsonElement", "Listy", kJSON, "", ""},
		{"型の無い def は JsonElement", "Untyped", kJSON, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := defsGenerator_3179(t, refShapeDefs_3179)
			s, err := g.refShape(tc.def)
			if err != nil {
				t.Fatal(err)
			}
			if s.kind != tc.wantKind || s.class != tc.wantClass {
				t.Errorf("refShape(%q) = kind %d class %q, want kind %d class %q", tc.def, s.kind, s.class, tc.wantKind, tc.wantClass)
			}
			if tc.wantKind == kEnum {
				if s.enum == nil || s.enum.fixedName != tc.wantEnum || strings.Join(s.enum.values, ",") != "a,b" {
					t.Errorf("enum = %+v", s.enum)
				}
			}
			if tc.wantKind == kClass && g.byName[tc.wantClass] == nil {
				t.Errorf("class %q was not built", tc.wantClass)
			}
		})
	}

	t.Run("無い $ref はエラー", func(t *testing.T) {
		t.Parallel()
		g := defsGenerator_3179(t, refShapeDefs_3179)
		if _, err := g.refShape("Nope"); err == nil || !strings.Contains(err.Error(), "unknown $ref") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("Kotlin の型と同じ名前の class はエラー", func(t *testing.T) {
		t.Parallel()
		g := defsGenerator_3179(t, `{"List": {"type": "object", "properties": {"x": {"type": "string"}}}}`)
		if _, err := g.refShape("List"); err == nil || !strings.Contains(err.Error(), "clashes") {
			t.Errorf("err = %v", err)
		}
	})
}
