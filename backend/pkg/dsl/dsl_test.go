package dsl

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const docsDir = "../../../docs/backend"

// The embedded schema must be byte-identical to the canonical doc copy.
func TestSchemaInSyncWithDocs(t *testing.T) {
	want, err := os.ReadFile(filepath.Join(docsDir, "dsl.schema.json"))
	if err != nil {
		t.Skip("docs schema not found:", err)
	}
	if !bytes.Equal(want, schemaJSON) {
		t.Fatal("backend/pkg/dsl/schema.json differs from docs/backend/dsl.schema.json; run `make schema-sync`")
	}
}

// Every example must pass schema+semantic validation and round-trip through
// the Go types without losing a single field.
func TestExamplesRoundTrip(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(docsDir, "examples", "*.json"))
	if len(files) == 0 {
		t.Skip("no examples found")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			app, err := Parse(raw)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			out, err := json.Marshal(app)
			if err != nil {
				t.Fatal(err)
			}
			var a, b any
			if err := json.Unmarshal(raw, &a); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(out, &b); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(a, b) {
				t.Fatalf("round-trip mismatch\nwant: %s\n got: %s", raw, out)
			}
			// re-validate the marshaled form too (omitempty must not drop required keys)
			if _, err := Parse(out); err != nil {
				t.Fatalf("re-Parse after marshal: %v", err)
			}
		})
	}
}

func TestInitialIsValid(t *testing.T) {
	raw, err := json.Marshal(Initial())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(raw); err != nil {
		t.Fatal(err)
	}
}

func TestSemanticErrors(t *testing.T) {
	app := Initial()
	app.InitialScreen = "nope"
	app.Screens[0].Components = append(app.Screens[0].Components, Component{
		Type: CompButton, Label: "go", Action: &Action{Type: ActionNavigate, Screen: "missing"},
	})
	app.Screens[0].Components = append(app.Screens[0].Components, Component{
		Type: CompVStack, Children: []Component{{Type: CompHStack, Children: []Component{{Type: CompDivider}}}},
	})
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	if len(ve.Problems) < 3 {
		t.Fatalf("expected >=3 problems, got %v", ve.Problems)
	}
}

func TestSchemaRejectsUnknownComponent(t *testing.T) {
	raw := []byte(`{"schemaVersion":"0.1","app":{"name":"x","voiceEntry":"none"},"initialScreen":"home","entities":[],
	"screens":[{"id":"home","title":"t","components":[{"type":"webview","url":"https://example.com"}]}]}`)
	if _, err := Parse(raw); err == nil {
		t.Fatal("expected schema error for unknown component type")
	}
}
