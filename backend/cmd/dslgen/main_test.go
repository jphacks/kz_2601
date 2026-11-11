package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// TestGeneratedKotlinIsUpToDate fails when android/dsl/.../generated is stale
// against schema.json (same idea as `make schema-check`).
func TestGeneratedKotlinIsUpToDate(t *testing.T) {
	files, err := Generate(dsl.SchemaJSON())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join("..", "..", filepath.FromSlash(defaultOut)) // tests run in backend/cmd/dslgen
	onDisk, err := filepath.Glob(filepath.Join(dir, "*.kt"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range onDisk {
		got = append(got, filepath.Base(p))
	}
	var want []string
	for n := range files {
		want = append(want, n)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("generated files %v, want %v: run `make dsl-gen` in backend/", got, want)
	}
	for _, n := range want {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != files[n] {
			t.Errorf("%s is stale against pkg/dsl/schema.json: run `make dsl-gen` in backend/ and commit", n)
		}
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	a, err := Generate(dsl.SchemaJSON())
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate(dsl.SchemaJSON())
	if err != nil {
		t.Fatal(err)
	}
	for n := range a {
		if a[n] != b[n] {
			t.Errorf("%s differs between runs", n)
		}
	}
}

// Action built in code keeps its type as rawType (iOS Action.init sets rawType = type.rawValue).
func TestActionRawTypeDefaultsToWire(t *testing.T) {
	files, err := Generate(dsl.SchemaJSON())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(files["Models.kt"], "val rawType: String = type.wire,") {
		t.Error("Action.rawType should default to type.wire")
	}
}

// Container types agree with the Go catalog (dsl.ContainerTypes).
func TestContainerTypesMatchGoCatalog(t *testing.T) {
	g, err := newGenerator(dsl.SchemaJSON())
	if err != nil {
		t.Fatal(err)
	}
	var got, want []string
	for _, ct := range g.compTypes {
		if ct.container {
			got = append(got, ct.wire)
		}
	}
	for ct := range dsl.ContainerTypes {
		want = append(want, string(ct))
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("container types = %v, want %v", got, want)
	}
	if n := len(g.compTypes) - len(want); n != len(g.leaves) || n == 0 {
		t.Errorf("leaf types = %d, want %d", n, len(g.leaves))
	}
}

const tinySchema = `{
  "type": "object",
  "required": ["screens"],
  "properties": {
    "schemaVersion": {"type": "integer"},
    "screens": {"type": "array", "items": {"$ref": "#/$defs/Screen"}}
  },
  "$defs": {
    "Id": {"type": "string"},
    "Screen": {"type": "object", "required": ["id"], "properties": {
      "id": {"$ref": "#/$defs/Id"},
      "mode": {"type": "string", "enum": ["a", "b"]},
      "components": {"type": "array", "items": {"$ref": "#/$defs/Component"}},
      "meta": {"type": "object", "properties": {"if": {"$ref": "#/$defs/Expr"}}}
    }},
    "Component": {"anyOf": [{"$ref": "#/$defs/Container"}, {"$ref": "#/$defs/Leaf"}]},
    "Container": {"type": "object", "properties": {
      "type": {"type": "string", "enum": ["vstack"]},
      "title": {"type": "string", "x-for": {"vstack": "optional"}},
      "children": {"type": "array", "items": {"$ref": "#/$defs/Leaf"}}
    }},
    "Leaf": {"anyOf": [{"$ref": "#/$defs/Text"}, {"$ref": "#/$defs/Pic"}]},
    "Text": {"type": "object", "properties": {
      "type": {"const": "text"}, "size": {"type": "string", "enum": ["small"]}, "value": {"type": "string"},
      "max": {"type": "integer"}, "expr": {"$ref": "#/$defs/Expr"}
    }},
    "Pic": {"type": "object", "properties": {
      "type": {"const": "pic"}, "size": {"type": "string", "enum": ["large"]}, "value": {"type": "number"},
      "max": {"type": "number"}, "expr": {"type": "number"}
    }},
    "Expr": {"anyOf": [{"type": "number"}, {"$ref": "#/$defs/ExprRef"}]},
    "ExprRef": {"type": "object", "required": ["ref"], "properties": {"ref": {"type": "string"}}}
  }
}`

func TestGenerateShapes(t *testing.T) {
	files, err := Generate([]byte(tinySchema))
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join([]string{files["Models.kt"], files["Component.kt"], files["Enums.kt"], files["Expr.kt"], files["DslKeys.kt"]}, "\n")
	for _, want := range []string{
		"val id: String,",         // required → strict
		"val mode: Mode? = null,", // optional enum
		"val components: List<Component> = emptyList(),",
		"data class ScreenMeta(",  // inline object → Owner + Key
		"val `if`: Expr? = null,", // keyword → backticks
		"val size: Size? = null,", // enums unioned across parts
		"    SMALL(\"small\"),\n    LARGE(\"large\"),",
		"val value: JsonElement? = null,", // string vs number → JsonElement
		"val max: Double? = null,",        // integer + number → number
		"val expr: Expr? = null,",         // Expr + number → Expr
		"VSTACK(\"vstack\", true),",
		"PIC(\"pic\"),",
		"ComponentType.VSTACK to setOf(\"type\", \"title\", \"children\"),",
		"data class ExprRef(",
		") : Expr {",
		"element.keys.containsAll(listOf(\"ref\")) -> ExprRef.serializer()",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("output lacks %q", want)
		}
	}
}

func TestScreaming(t *testing.T) {
	for in, want := range map[string]string{
		"textField": "TEXT_FIELD", "vstack": "VSTACK", "h264": "H264", "2x": "V_2X", "a-b": "A_B", "qrCode": "QR_CODE",
	} {
		if got := screaming(in); got != want {
			t.Errorf("screaming(%q) = %q, want %q", in, got, want)
		}
	}
}
