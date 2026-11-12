package dsl

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestProblemCode(t *testing.T) {
	for msg, want := range map[string]string{
		`screen "home" components[0]: unknown component type "chart3d"`: CodeUnknownComp,
		`screen "home" components[0]: entity "ghost" does not exist`:    CodeBadRef,
		`screen "home": aiButton output "x" not in entity "e"`:          CodeBadRef,
		`screen "home": aiButton requires screen.entity`:                CodeMissingField,
		`app.name is empty`:                                                                CodeMissingField,
		`too many screens (13 > 12)`:                                                       CodeLimit,
		`entity "e" field "a" is listed twice`:                                             CodeDuplicate,
		`screen "s": aiButton output "total" is computed (read-only)`:                      CodeReadOnly,
		`screen "s": aiButton input and output must differ`:                                CodeConflict,
		`screen "s": aiButton options are only for task classify`:                          CodeWrongPlace,
		`screen "s": aiButton classify output "x" must be enum, string or text (got date)`: CodeWrongType,
		`...and 4 more`:                     CodeMore,
		`document exceeds 65536 bytes`:      CodeTooLarge,
		`decode: json: unknown field "zzz"`: CodeDecode,
		`something nobody wrote a rule for`: CodeOther,
	} {
		if got := ProblemCode(msg); got != want {
			t.Errorf("ProblemCode(%q) = %s, want %s", msg, got, want)
		}
	}
}

// Quoted parts never change the code: an id or name that happens to contain
// a rule phrase is still the same rule.
func TestProblemCode_IgnoresQuoted(t *testing.T) {
	a := ProblemCode(`entity "a" field "b" is listed twice`)
	b := ProblemCode(`entity "does not exist" field "unknown component type" is listed twice`)
	if a != b || a != CodeDuplicate {
		t.Fatalf("%s vs %s", a, b)
	}
}

func codesOf(t *testing.T, err error) []string {
	t.Helper()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("not a ValidationError: %v", err)
	}
	c := ve.Codes()
	if len(c) != len(ve.Problems) {
		t.Fatalf("codes %v for problems %v", c, ve.Problems)
	}
	return c
}

func TestValidationErrorCodes(t *testing.T) {
	if c := codesOf(t, ValidateSchema([]byte("{"))); c[0] != CodeInvalidJSON {
		t.Errorf("invalid json: %v", c)
	}
	_, err := Parse(make([]byte, MaxDSLBytes+1))
	if c := codesOf(t, err); c[0] != CodeTooLarge {
		t.Errorf("too large: %v", c)
	}
	c := codesOf(t, ValidateSchema([]byte(`{"schemaVersion":1}`)))
	for _, x := range c {
		if !strings.HasPrefix(x, "schema_") {
			t.Errorf("schema problem code %q", x)
		}
	}
	if !slices.Contains(c, "schema_required") {
		t.Errorf("missing schema_required: %v", c)
	}
	_, _, err = ApplyPatch([]byte(`{}`), nil)
	if c := codesOf(t, err); c[0] != CodePatchEmpty {
		t.Errorf("empty patch: %v", c)
	}
	// built elsewhere without codes: derived from the message
	ve := &ValidationError{Problems: []string{`x: entity "e" does not exist`}}
	if c := ve.Codes(); c[0] != CodeBadRef {
		t.Errorf("fallback: %v", c)
	}
}

var (
	placeholderVerbs   = regexp.MustCompile(`%[a-zA-Z]`)
	placeholderLetters = regexp.MustCompile(`[a-zA-Z]{3,}`)
)

// Every add("...") message of the checkers gets a code, and few fall to
// "other" (a new rule phrase belongs in problemRules).
func TestProblemCode_CoversCheckers(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`\badd\(("(?:[^"\\]|\\.)*")`)
	counts := map[string]int{}
	var other []string
	total := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			format, err := strconv.Unquote(m[1])
			if err != nil {
				continue
			}
			if !placeholderLetters.MatchString(placeholderVerbs.ReplaceAllString(format, "")) {
				continue // "%s: %s" passes on a message classified on its own
			}
			code := ProblemCode(format)
			counts[code]++
			total++
			if code == CodeOther {
				other = append(other, format)
			}
		}
	}
	if total < 500 {
		t.Fatalf("found only %d checker messages; the scan is broken", total)
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("%-20s %d", k, counts[k])
	}
	if share := float64(len(other)) / float64(total); share > 0.03 {
		t.Errorf("%d/%d checker messages are %q:\n%s", len(other), total, CodeOther, strings.Join(other, "\n"))
	}
}
