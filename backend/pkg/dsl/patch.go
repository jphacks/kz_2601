package dsl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// PatchOp is one RFC 6902 operation (mirrors $defs/PatchOp).
type PatchOp struct {
	Op    string          `json:"op"` // add|remove|replace|move|copy|test
	Path  string          `json:"path"`
	From  string          `json:"from,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

// PatchDocument is the structured-output root for edit turns
// (mirrors $defs/PatchDocument). The ai service receives this from Claude
// and forwards `patch` to appconfig: PATCH /apps/{id}/dsl.
type PatchDocument struct {
	Patch   []PatchOp `json:"patch"`
	Summary string    `json:"summary"`
}

// MaxPatchOps bounds a single edit turn.
const MaxPatchOps = 50

// ApplyPatch applies ops to the current DSL JSON, then runs the full
// Parse (schema + semantic) on the result. It returns the new document bytes
// (canonical, compact) and the typed App. Any failure leaves `current`
// untouched; errors are *ValidationError so the ai can re-prompt with them.
func ApplyPatch(current []byte, ops []PatchOp) ([]byte, *App, error) {
	if len(ops) == 0 {
		return nil, nil, &ValidationError{Problems: []string{"patch is empty"}}
	}
	if len(ops) > MaxPatchOps {
		return nil, nil, &ValidationError{Problems: []string{fmt.Sprintf("too many patch ops (%d > %d)", len(ops), MaxPatchOps)}}
	}
	raw, err := json.Marshal(ops)
	if err != nil {
		return nil, nil, &ValidationError{Problems: []string{"encode patch: " + err.Error()}}
	}
	p, err := jsonpatch.DecodePatch(raw)
	if err != nil {
		return nil, nil, &ValidationError{Problems: []string{"invalid JSON Patch: " + err.Error()}}
	}
	opts := jsonpatch.NewApplyOptions()
	opts.EnsurePathExistsOnAdd = false
	opts.AllowMissingPathOnRemove = false
	next, err := p.ApplyWithOptions(current, opts)
	if err != nil {
		return nil, nil, &ValidationError{Problems: []string{"apply patch: " + err.Error()}}
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, next); err != nil {
		return nil, nil, &ValidationError{Problems: []string{"compact: " + err.Error()}}
	}
	app, err := Parse(buf.Bytes())
	if err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), app, nil
}

// ---- PatchDocument schema (edit-turn structured output) -------------------

// patchSchemaJSON is derived from the embedded schema: same $defs, root = PatchDocument.
var patchSchemaJSON = func() []byte {
	var root map[string]any
	if err := json.Unmarshal(schemaJSON, &root); err != nil {
		panic("dsl: schema: " + err.Error())
	}
	derived := map[string]any{
		"$schema":     root["$schema"],
		"$id":         "https://github.com/jphacks/kz_2601/docs/dsl/dsl-patch.schema.json",
		"title":       "AppDSLPatch",
		"description": "Structured-output schema for edit turns: {patch: RFC6902 ops, summary}.",
		"$ref":        "#/$defs/PatchDocument",
		"$defs":       root["$defs"],
	}
	b, err := json.Marshal(derived)
	if err != nil {
		panic("dsl: patch schema: " + err.Error())
	}
	return b
}()

// PatchSchemaJSON is what the ai service passes to Claude as
// output_config.format for edit turns (root = $defs/PatchDocument).
func PatchSchemaJSON() []byte { return patchSchemaJSON }

var compiledPatch = func() *jsonschema.Schema {
	c := jsonschema.NewCompiler()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(patchSchemaJSON))
	if err != nil {
		panic("dsl: patch schema is not JSON: " + err.Error())
	}
	if err := c.AddResource("dsl-patch.schema.json", doc); err != nil {
		panic("dsl: add patch schema: " + err.Error())
	}
	s, err := c.Compile("dsl-patch.schema.json")
	if err != nil {
		panic("dsl: compile patch schema: " + err.Error())
	}
	return s
}()

// ValidatePatchDocument checks a raw PatchDocument against the schema.
func ValidatePatchDocument(raw []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return &ValidationError{Problems: []string{"invalid JSON: " + err.Error()}}
	}
	if err := compiledPatch.Validate(doc); err != nil {
		var ve *jsonschema.ValidationError
		if errors.As(err, &ve) {
			return &ValidationError{Problems: flatten(ve)}
		}
		return &ValidationError{Problems: []string{err.Error()}}
	}
	return nil
}

// Diff returns an RFC 6902 patch that transforms `from` into `to`
// (used for the version log and summary generation when the ai sends a
// full document, e.g. first generation or fallback).
func Diff(from, to []byte) ([]PatchOp, error) {
	// json-patch v5 has no diff; keep the dependency surface small and
	// record a single replace at the root. Fine-grained diff is a nice-to-have.
	if bytes.Equal(from, to) {
		return nil, nil
	}
	return []PatchOp{{Op: "replace", Path: "", Value: json.RawMessage(to)}}, nil
}
