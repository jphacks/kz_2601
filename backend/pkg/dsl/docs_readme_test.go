package dsl

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

const dslReadme = "../../../docs/dsl/README.md"

// schemaTypeNames returns every name a schema "type" property can take: the
// const of each component (Text, Stat, ...), the enum of the containers, and
// Action.type / Field.type.
func schemaTypeNames(t *testing.T) []string {
	t.Helper()
	type typeProp struct {
		Const string   `json:"const"`
		Enum  []string `json:"enum"`
	}
	var schema struct {
		Defs map[string]struct {
			Properties struct {
				Type *typeProp `json:"type"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(SchemaJSON(), &schema); err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	for _, def := range schema.Defs {
		p := def.Properties.Type
		if p == nil {
			continue
		}
		if p.Const != "" {
			set[p.Const] = true
		}
		for _, e := range p.Enum {
			set[e] = true
		}
	}
	for _, need := range []string{"Action", "Field"} { // guard: these two must be present
		if p := schema.Defs[need].Properties.Type; p == nil || len(p.Enum) == 0 {
			t.Fatalf("schema: %s.type has no enum", need)
		}
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// TestDSLReadmeListsAllTypes_2968: every component / container type, action
// type and field type in schema.json appears in docs/dsl/README.md as `name`,
// so a new type cannot ship without a line in the doc.
func TestDSLReadmeListsAllTypes_2968(t *testing.T) {
	b, err := os.ReadFile(dslReadme)
	if err != nil {
		t.Fatal(err)
	}
	readme := string(b)
	names := schemaTypeNames(t)
	if len(names) < 50 {
		t.Fatalf("only %d type names found in the schema; the walk is broken", len(names))
	}
	var missing []string
	for _, n := range names {
		if !strings.Contains(readme, "`"+n+"`") {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		t.Errorf("types in schema.json but not written as `name` in %s: %v", dslReadme, missing)
	}
}
