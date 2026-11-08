package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// TestPromptSnippetsValid extracts every inline JSON object the prompts teach
// ({"op":...}, filters, components, entities, server rules, whole apps) and
// checks it with the real validators in pkg/dsl, so a rule cannot teach a
// shape the validator rejects. Snippets that are templates rather than JSON
// (<entityId>, "...", "field"?:) are skipped and counted.
func TestPromptSnippetsValid(t *testing.T) {
	base := []struct{ name, text string }{
		{"systemPrompt", systemPrompt},
		{"editRules", editRules},
		{"serverRulesPrompt", serverRulesPrompt},
		{"logicSystemPrompt", logicSystemPrompt},
		{"chatSystemPrompt", chatSystemPrompt},
		{"chatEditSystemPrompt", chatEditSystemPrompt},
		{"chatCreateSystemPrompt", chatCreateSystemPrompt},
	}
	prompts := slices.Clone(base)
	// restrictPrompt swaps lines for older / review builds (settings via
	// agg latest, Safari instead of webView, ...); those replacements teach
	// snippets of their own. Already-seen snippets are not counted again.
	all := slices.Sorted(maps.Keys(knownComponentTypes))
	noWeb := slices.DeleteFunc(slices.Clone(all), func(s string) bool { return s == "webView" || s == "chatThread" })
	for _, r := range []struct {
		name string
		r    Restriction
	}{
		{"oldClient", NewClientRestriction(all, nil, nil)},
		{"reviewBuild", NewClientRestriction(noWeb, []string{FeatureSharedData, FeatureFieldHealth, FeatureCameraCapture}, nil)},
	} {
		ctx := withRestriction(context.Background(), r.r)
		for _, p := range base {
			prompts = append(prompts, struct{ name, text string }{r.name + "/" + p.name, restrictPrompt(ctx, p.text)})
		}
	}
	v := newSnippetValidator(t)
	seen := map[string]bool{}
	var found, validated, skipped int
	skips := map[string]int{}
	classes := map[string]int{}
	var failures []string
	for _, p := range prompts {
		for _, s := range scanJSONObjects(p.text) {
			if seen[s] {
				continue
			}
			seen[s] = true
			found++
			class, problems, skip := v.check(s)
			if skip != "" {
				skipped++
				skips[skip]++
				t.Logf("skip (%s): %s", skip, short(s))
				continue
			}
			validated++
			classes[class]++
			if len(problems) == 0 {
				continue
			}
			if reason, ok := snippetAllowlist[s]; ok {
				t.Logf("allowlisted (%s): %s", reason, short(s))
				continue
			}
			failures = append(failures, fmt.Sprintf("%s [%s] %s\n    %s", p.name, class, short(s), strings.Join(problems, "\n    ")))
		}
	}
	t.Logf("snippets found=%d validated=%d skipped=%d failed=%d; classes=%v; skips=%v", found, validated, skipped, len(failures), classes, skips)
	for _, f := range failures {
		t.Errorf("prompt teaches invalid DSL: %s", f)
	}
	// Guard the scanner itself: the prompts teach well over a hundred snippets.
	if found < 150 || validated < 100 {
		t.Errorf("scanner found too few snippets (found=%d validated=%d); did the extraction break?", found, validated)
	}
	for s := range snippetAllowlist {
		if !seen[s] {
			t.Errorf("allowlist entry no longer in any prompt; remove it: %s", short(s))
		}
	}
}

// snippetAllowlist: snippets that fail on purpose, with the reason.
var snippetAllowlist = map[string]string{}

// scanJSONObjects returns each top-level {"... object in s, matched by
// balanced braces outside string literals. An object never spans lines in the
// prompts, so an unterminated one is returned up to the newline (and then
// fails to parse).
func scanJSONObjects(s string) []string {
	var out []string
	for i := 0; i+1 < len(s); i++ {
		if s[i] != '{' || s[i+1] != '"' {
			continue
		}
		depth, inStr, esc, end := 0, false, false, -1
	scan:
		for j := i; j < len(s); j++ {
			c := s[j]
			switch {
			case inStr && esc:
				esc = false
			case inStr && c == '\\':
				esc = true
			case inStr:
				inStr = c != '"'
			case c == '"':
				inStr = true
			case c == '{':
				depth++
			case c == '}':
				depth--
				if depth == 0 {
					end = j + 1
					break scan
				}
			case c == '\n':
				end = j
				break scan
			}
		}
		if end < 0 {
			end = len(s)
		}
		out = append(out, s[i:end])
		i = end - 1
	}
	return out
}

func short(s string) string {
	if r := []rune(s); len(r) > 160 {
		return string(r[:160]) + "…"
	}
	return s
}

// Template markers: <name> stands for an id, "..." / … elides keys,
// "x"?: marks an optional key, bare words (Expr, X) stand for values.
var (
	anglePlaceholder = regexp.MustCompile(`<[^<>"\n]{1,40}>`)
	areaPlaceholder  = regexp.MustCompile(`"area":"<[^<>"\n]{1,40}>"`)
	elision          = regexp.MustCompile(`\.\.\.|…|"\?:`)
	bareProse        = regexp.MustCompile(`:\s*[^\x00-\x7F]`) // {"total": 同じ agg}
)

type snippetValidator struct {
	t          *testing.T
	compiler   *jsonschema.Compiler
	schemas    map[string]*jsonschema.Schema
	components map[string]bool
	actions    map[string]bool
}

func newSnippetValidator(t *testing.T) *snippetValidator {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(dsl.SchemaJSON()))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("dsl.schema.json", doc); err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Defs map[string]struct {
			AnyOf []struct {
				Ref string `json:"$ref"`
			} `json:"anyOf"`
			Properties map[string]struct {
				Const any   `json:"const"`
				Enum  []any `json:"enum"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(dsl.SchemaJSON(), &schema); err != nil {
		t.Fatal(err)
	}
	v := &snippetValidator{t: t, compiler: c, schemas: map[string]*jsonschema.Schema{}, components: map[string]bool{}, actions: map[string]bool{}}
	var collect func(def string)
	collect = func(def string) {
		d := schema.Defs[def]
		for _, r := range d.AnyOf {
			collect(strings.TrimPrefix(r.Ref, "#/$defs/"))
		}
		if tp, ok := d.Properties["type"]; ok {
			for _, name := range append([]any{tp.Const}, tp.Enum...) {
				if name, ok := name.(string); ok && name != "" {
					v.components[name] = true
				}
			}
		}
	}
	collect("Component")
	for _, name := range schema.Defs["Action"].Properties["type"].Enum {
		if name, ok := name.(string); ok {
			v.actions[name] = true
		}
	}
	if len(v.components) < 20 || len(v.actions) < 3 {
		t.Fatalf("component/action types not found in the schema (%d/%d)", len(v.components), len(v.actions))
	}
	return v
}

// schema validates raw against $defs/<def> of the embedded DSL schema.
func (v *snippetValidator) schema(def string, raw []byte) []string {
	s, ok := v.schemas[def]
	if !ok {
		var err error
		if s, err = v.compiler.Compile("dsl.schema.json#/$defs/" + def); err != nil {
			v.t.Fatalf("compile $defs/%s: %v", def, err)
		}
		v.schemas[def] = s
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return []string{err.Error()}
	}
	if err := s.Validate(inst); err != nil {
		return []string{fmt.Sprintf("$defs/%s: %v", def, strings.ReplaceAll(err.Error(), "\n", " "))}
	}
	return nil
}

// expr parses raw as an Expr (strict keys), checks the schema and the
// standalone shape rules (op names, arity, agg / filter ops).
func (v *snippetValidator) expr(raw []byte) []string {
	var e dsl.Expr
	if err := json.Unmarshal(raw, &e); err != nil {
		return []string{err.Error()}
	}
	return append(v.schema("Expr", raw), dsl.ExprShapeProblems(&e)...)
}

// check classifies one snippet and validates it. skip != "" says why it was
// not validated.
func (v *snippetValidator) check(s string) (class string, problems []string, skip string) {
	raw := []byte(s)
	if json.Valid(raw) && anglePlaceholder.MatchString(s) {
		// <fieldId> inside a string literal is only a name; the shape around
		// it is still checked with a valid id in its place.
		// An area keeps its A1:F1 pattern.
		raw = areaPlaceholder.ReplaceAll(raw, []byte(`"area":"A1:F1"`))
		raw = anglePlaceholder.ReplaceAllFunc(raw, func([]byte) []byte { return []byte("x") })
	}
	defer func() {
		switch class {
		case "", "expression", "values", "set-value", "filter", "app", "op-label", "typed", "unclassified", "invalid-json":
			// checked whole already (or not at all)
		default:
			problems = append(problems, nestedExprProblems(raw)...)
		}
	}()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		if anglePlaceholder.MatchString(s) || elision.MatchString(s) || strings.Contains(s, ":X") || strings.Contains(s, "Expr") || bareProse.MatchString(s) {
			return "", nil, "template, not JSON"
		}
		return "invalid-json", []string{err.Error()}, ""
	}
	if hasAlternatives(m) {
		return "", nil, `alternatives "a|b"`
	}
	has := func(keys ...string) bool {
		for _, k := range keys {
			if _, ok := m[k]; !ok {
				return false
			}
		}
		return true
	}
	onlyKeys := func(keys ...string) bool {
		for k := range m {
			if !slices.Contains(keys, k) {
				return false
			}
		}
		return true
	}
	var typ string
	_ = json.Unmarshal(m["type"], &typ)
	// The prompts ask Claude to leave out implied values (filter value null,
	// sort direction desc, ...); fillDefaults / fillNode put them back before
	// dsl.Parse, so the snippet is checked the same way.
	filled := fillSnippet(raw, has("field", "op"), has("field", "direction"))
	switch {
	case has("$defs"):
		return "", nil, "the schema itself"
	case has("schemaVersion") || has("app", "screens"):
		_, err := dsl.Parse(fillDefaults(raw))
		return "app", errProblems(err), ""
	case has("app"):
		return "", nil, "app fragment"
	case has("summary", "patch"):
		return "patch", v.schema("PatchDocument", raw), ""
	case has("op", "path"):
		var op dsl.PatchOp
		if err := json.Unmarshal(raw, &op); err != nil {
			return "patch-op", []string{err.Error()}, ""
		}
		op = fillPatchDefaults([]dsl.PatchOp{op}, nil)[0]
		if v.components[valueType(op.Value)] {
			// a new component: check it as a Component (PatchOp.value is anyOf
			// every shape, so its error would not say what is wrong).
			return "patch-op", v.schema("Component", op.Value), ""
		}
		filledOp, _ := json.Marshal(op)
		return "patch-op", v.schema("PatchOp", filledOp), ""
	case has("serverRules"):
		var out struct {
			Rules []json.RawMessage `json:"serverRules"`
		}
		_ = json.Unmarshal(raw, &out)
		for _, r := range out.Rules {
			problems = append(problems, v.schema("ServerRule", r)...)
		}
		return "server-rules", problems, ""
	case has("id", "on", "do"):
		return "server-rule", v.schema("ServerRule", raw), ""
	case len(m) == 1 && has("op"):
		// {"op":"add"} names an op in the catalog list: the name must exist.
		var name string
		_ = json.Unmarshal(m["op"], &name)
		if _, ok := dsl.Ops[name]; !ok && !slices.Contains(dsl.FilterOps, name) {
			return "op-label", []string{fmt.Sprintf("unknown op %q", name)}, ""
		}
		return "", nil, "op name label (catalog list)"
	case has("op", "entity", "field", "equals"):
		return "server-lookup", v.schema("ServerRule/properties/lookup", raw), ""
	case has("entity", "fields"):
		return "server-similar", v.schema("ServerRule/properties/similar", raw), ""
	case has("order"):
		return "server-pick", v.schema("ServerRule/properties/pick", raw), ""
	case has("kind"):
		return "server-recipient", v.schema("RuleRecipient", raw), ""
	case has("ref") || has("agg") || has("setting") || has("if") || (has("op") && onlyKeys("op", "args")):
		return "expression", v.expr(filled), ""
	case has("field", "op"):
		return "filter", append(v.schema("Filter", filled), v.filterShape(filled)...), ""
	case has("field", "direction"):
		return "sort", v.schema("Sort", filled), ""
	case has("field", "value"):
		// server rule updateRecord / createRecord entry: value is an Expr.
		return "set-value", v.exprValue(m["value"]), ""
	case has("me") || has("myFollows"):
		return "filter-value", v.schema("Filter", []byte(`{"field":"$author","op":"eq","value":`+string(raw)+`}`)), ""
	case typ != "" && v.components[typ]:
		return "component", v.schema("Component", filled), ""
	case typ != "" && v.actions[typ]:
		return "action", v.schema("Action", filled), ""
	case has("id", "fields"):
		return "entity", v.schema("Entity", filled), ""
	case has("id", "type"):
		return "field", v.schema("Field", filled), ""
	case has("id", "icon", "label"):
		return "mark", v.schema("Mark", raw), ""
	case has("field") && onlyKeys("field", "title", "aggregate"):
		return "table-column", v.schema("TableColumn", raw), ""
	case allExprs(m):
		// server rule values map {"<field>": Expr}.
		for _, k := range slices.Sorted(maps.Keys(m)) {
			problems = append(problems, v.expr(m[k])...)
		}
		return "values", problems, ""
	case typ != "":
		return "typed", []string{fmt.Sprintf("type %q is neither a component nor an action type", typ)}, ""
	}
	// Not skipped: a snippet the classifier does not know would otherwise
	// never be validated. Teach the classifier or allowlist it.
	return "unclassified", []string{"no check for keys " + strings.Join(slices.Sorted(maps.Keys(m)), ",")}, ""
}

// nestedExprProblems shape-checks every expression inside a component,
// entity, rule, ... (the schema does not know op arity or agg names).
func nestedExprProblems(raw []byte) []string {
	var doc any
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	var out []string
	var walk func(n any, top bool)
	walk = func(n any, top bool) {
		switch n := n.(type) {
		case []any:
			for _, c := range n {
				walk(c, false)
			}
		case map[string]any:
			if !top && looksLikeExpr(n) {
				b, _ := json.Marshal(n)
				var e dsl.Expr
				if err := json.Unmarshal(b, &e); err != nil {
					out = append(out, err.Error())
				} else {
					out = append(out, dsl.ExprShapeProblems(&e)...)
				}
				return
			}
			for _, k := range slices.Sorted(maps.Keys(n)) {
				walk(n[k], false)
			}
		}
	}
	walk(doc, true)
	return out
}

func looksLikeExpr(m map[string]any) bool {
	_, op := m["op"]
	_, args := m["args"]
	_, agg := m["agg"]
	_, ent := m["entity"]
	_, cond := m["if"]
	_, then := m["then"]
	return (op && args && len(m) == 2) || (agg && ent) || (cond && then)
}

// fillSnippet applies the defaults the generation path fills in.
func fillSnippet(raw []byte, isFilter, isSort bool) []byte {
	var doc any
	if json.Unmarshal(raw, &doc) != nil {
		return raw
	}
	fillNode(doc, nil)
	switch m, _ := doc.(map[string]any); {
	case isFilter:
		fillFilters(m)
	case isSort:
		if _, ok := m["direction"]; !ok {
			m["direction"] = "desc"
		}
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	return out
}

func valueType(raw json.RawMessage) string {
	var c struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(raw, &c)
	return c.Type
}

// allExprs: every value is an object that decodes as an Expr.
func allExprs(m map[string]json.RawMessage) bool {
	for _, r := range m {
		var e dsl.Expr
		if t := bytes.TrimSpace(r); len(t) == 0 || t[0] != '{' || json.Unmarshal(r, &e) != nil {
			return false
		}
	}
	return len(m) > 0
}

// filterShape checks a filter's op / value through an agg around it.
// A component's own filter may name $author / $marks.* or take {"me":true}
// (agg may not), so those parts are swapped for plain ones first.
func (v *snippetValidator) filterShape(raw []byte) []string {
	var f map[string]any
	if json.Unmarshal(raw, &f) != nil {
		return nil
	}
	if s, _ := f["field"].(string); strings.HasPrefix(s, "$") {
		f["field"] = "x"
	}
	if val, ok := f["value"].(map[string]any); ok && (val["me"] != nil || val["myFollows"] != nil) {
		f["value"] = "x"
	}
	raw, _ = json.Marshal(f)
	var e dsl.Expr
	if err := json.Unmarshal([]byte(`{"agg":"count","entity":"x","filter":`+string(raw)+`}`), &e); err != nil {
		return []string{err.Error()}
	}
	return dsl.ExprShapeProblems(&e)
}

func (v *snippetValidator) exprValue(raw json.RawMessage) []string {
	if t := bytes.TrimSpace(raw); len(t) == 0 || t[0] != '{' {
		return nil
	}
	return v.expr(raw)
}

// hasAlternatives: a string value like "count|sum|avg" lists choices.
func hasAlternatives(m map[string]json.RawMessage) bool {
	for _, r := range m {
		var s string
		if json.Unmarshal(r, &s) == nil && strings.Contains(s, "|") {
			return true
		}
	}
	return false
}

func errProblems(err error) []string {
	if err == nil {
		return nil
	}
	if ve, ok := err.(*dsl.ValidationError); ok {
		return ve.Problems
	}
	return []string{err.Error()}
}

// TestSnippetValidatorCatches keeps the checker honest: wrong shapes fail,
// templates are skipped, implied defaults pass.
func TestSnippetValidatorCatches(t *testing.T) {
	v := newSnippetValidator(t)
	for _, tc := range []struct {
		src        string
		fail, skip bool
	}{
		{`{"op":"addDays","args":[{"ref":"due"},-3]}`, false, false},
		{`{"op":"addDays","args":[{"ref":"due"}]}`, true, false},
		{`{"op":"subtract","args":[1,2]}`, true, false},
		{`{"agg":"total","entity":"e","field":"a"}`, true, false},
		{`{"field":"date","op":"thisMonth"}`, false, false},
		{`{"field":"date","op":"between","value":1}`, true, false},
		{`{"type":"stat","label":"合計","entity":"e","field":"a","aggregat":"sum"}`, true, false},
		{`{"id":"<親>","type":"ref","refEntity":"<親の entity>"}`, true, false},
		{`{"op":"<name>","args":[Expr,...]}`, false, true},
		{`{"agg":"count",...}`, false, true},
		{`{"op":"add"}`, false, true},
		// #764 follow-up: these used to be skipped or passed.
		{`{"op":"subtract"}`, true, false},
		{`{"type":"navigat","screen":"x"}`, true, false},
		{`{"type":"stats","label":"x"}`, true, false},
		{`{"foo":1,"bar":2}`, true, false},
		{`{"type":"stat","label":"x","value":{"op":"addDays","args":[1]}}`, true, false},
		{`{"op":"add","path":"/screens/0/components/-","value":{"type":"stat","label":"x","value":{"agg":"total","entity":"e","field":"a"}}}`, true, false},
		{`{"entity":"e","order":"random","filter":[{"field":"a","op":"eq","value":{"op":"not","args":[]}}]}`, true, false},
		{`{"type":"stat","label":"x","value":{"agg":"sum","entity":"e"}}`, true, false},
		{`{"type":"stat","label":"自分","entity":"e","aggregate":"count","filter":{"field":"$author","op":"eq","value":{"me":true}}}`, false, false},
	} {
		_, problems, skip := v.check(tc.src)
		if (skip != "") != tc.skip || (len(problems) > 0) != tc.fail {
			t.Errorf("%s: skip=%q problems=%v, want skip=%v fail=%v", tc.src, skip, problems, tc.skip, tc.fail)
		}
	}
}
