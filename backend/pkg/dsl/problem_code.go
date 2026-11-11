package dsl

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// Problem codes (#1772): a stable label per validation problem, so logs can
// count why outputs fail without carrying the message (which quotes the DSL).
// Schema problems are "schema_" + the failing JSON Schema keyword in snake
// case (schema_required, schema_enum, schema_additional_properties ...).
const (
	CodeTooLarge      = "too_large"      // document over MaxDSLBytes
	CodeInvalidJSON   = "invalid_json"   // not JSON
	CodeDecode        = "decode"         // JSON does not decode into App
	CodePatchEmpty    = "patch_empty"    // PatchDocument without ops
	CodePatchTooMany  = "patch_too_many" // more than MaxPatchOps
	CodePatchInvalid  = "patch_invalid"  // not a JSON Patch
	CodePatchApply    = "patch_apply"    // patch does not apply to the document
	CodeSchemaUnknown = "schema_unknown_type"
	CodeMore          = "more"              // the "...and N more" line of a truncated list
	CodeUnknownComp   = "unknown_component" // unknown component / action type or op
	CodeBadRef        = "bad_ref"           // names an entity / field / screen ... that does not exist
	CodeMissingField  = "missing_field"     // something required is not set
	CodeDuplicate     = "duplicate"         // id / item listed twice
	CodeLimit         = "limit"             // too many / too long / out of range
	CodeWrongType     = "wrong_type"        // a field of the wrong type for this use
	CodeReadOnly      = "read_only"         // writes a computed field
	CodeHiddenField   = "hidden_field"      // exposes or writes a hidden field
	CodeEncrypted     = "encrypted_field"   // reads an encrypted field where it cannot
	CodeConflict      = "conflict"          // two settings that exclude each other
	CodeWrongPlace    = "wrong_place"       // a setting used where it does not apply
	CodeNotAllowed    = "not_allowed"       // a rule forbids this combination
	CodeBadValue      = "bad_value"         // a value outside what is accepted
	CodeOther         = "other"
)

var quotedRe = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)

// problemRules map a problem message (quoted parts removed, lower case) to a
// code; the first match wins, so specific phrases come before general ones.
var problemRules = []struct {
	code    string
	phrases []string
}{
	{CodeMore, []string{"...and "}},
	{CodeUnknownComp, []string{"unknown component type", "unknown action type", "unknown op", "unknown "}},
	{CodeEncrypted, []string{"encrypted"}},
	{CodeHiddenField, []string{"hidden"}},
	{CodeReadOnly, []string{"is computed", "read-only", "computed field", "not computed ones"}},
	{CodeDuplicate, []string{"twice", "duplicate", "already", "keep one", "used by another", "another name"}},
	{CodeBadRef, []string{"does not exist", "not in entity", "not in screen", "no such", "not found", "is not an entity", "is not a field", "refers to", "points to", "which no "}},
	{CodeLimit, []string{"too many", "too long", "longer than", "at most", "exceeds", "(max ", "1..", "0..", "must be between", "must be <=", "must be >=", "out of range", "nested deeper", "goes past"}},
	{CodeConflict, []string{"exclusive", "must differ", "both", "conflict", "the same field", "same as", "the same room", "are the same", "exactly one of"}},
	{CodeWrongType, []string{"(got ", "must be a ", "must be an ", "must be string", "is not a ", "is not an ", "field type", "is type ", "expects a", "compares a", "compares date", "compares string", "holds ids of", "; use a ", "is a %s, not", "not comparable", "is %s but the value", "is string;", "is a date field", "same type", "point it at", "make it type", "not money"}},
	{CodeWrongPlace, []string{"only for", "only on", "only in", "is only", "only with", "belongs in", "same side", "is for ", "only allowed for", "uses only", "not on a tap"}},
	{CodeMissingField, []string{"needs", "need entity", "need screen", "requires", "required", "is empty", "empty value", "is blank", "is missing", "at least", "has no", "maps no", "must have", "must set", "must include", "must keep", "set inputentity"}},
	{CodeNotAllowed, []string{"cannot", "can't", "could not", "not allowed", "must not", "may not", "may only", "do not", "does not", "remove ", "editableby", "is private", "is single", "control or invisible", "unmatched", "is reserved", "is addressed", "private", "single entity", "keep it empty", "not exchanged", "only op len"}},
	{CodeBadValue, []string{"must be", "invalid", "not one of", "is not", "always", "multiplies by 0", "takes value", "takes fields", "also the start", "(marker|bold)"}},
}

// ProblemCode is the stable code of one problem message (Validate and the
// checks it runs). Quoted parts (ids, names, values) are ignored, so the
// code depends on the rule, never on what the app contains.
func ProblemCode(problem string) string {
	s := strings.ToLower(quotedRe.ReplaceAllString(problem, `""`))
	switch {
	case strings.HasPrefix(s, "document exceeds"):
		return CodeTooLarge
	case strings.HasPrefix(s, "invalid json:"):
		return CodeInvalidJSON
	case strings.HasPrefix(s, "decode:"):
		return CodeDecode
	}
	for _, r := range problemRules {
		for _, ph := range r.phrases {
			if strings.Contains(s, ph) {
				return r.code
			}
		}
	}
	return CodeOther
}

// Codes returns one code per problem, in the order of Problems. Problems
// added without a code (a ValidationError built elsewhere) get ProblemCode.
func (e *ValidationError) Codes() []string {
	out := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		if i < len(e.codes) && e.codes[i] != "" {
			out[i] = e.codes[i]
		} else {
			out[i] = ProblemCode(p)
		}
	}
	return out
}

// problem builds a one-problem ValidationError with an explicit code.
func problem(code, msg string) *ValidationError {
	return &ValidationError{Problems: []string{msg}, codes: []string{code}}
}

// schemaCode is the code of one leaf schema error.
func schemaCode(v *jsonschema.ValidationError) string {
	switch v.ErrorKind.(type) {
	case *kind.Const, *kind.Enum:
		if n := len(v.InstanceLocation); n > 0 && v.InstanceLocation[n-1] == "type" {
			return CodeSchemaUnknown
		}
	}
	kp := v.ErrorKind.KeywordPath()
	name := ""
	if len(kp) > 0 {
		name = kp[len(kp)-1]
	} else { // keyword-less kinds (false schema, invalid value ...): the kind's type name
		name = strings.TrimPrefix(fmt.Sprintf("%T", v.ErrorKind), "*kind.")
	}
	return "schema_" + snake(name)
}

func snake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}
