package ai

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// catalog is the Markdown component/action catalog embedded in the system
// prompt. It is derived from the canonical schema so that adding a component
// to dsl.schema.json (with description / x-usage) is enough for Claude to
// learn it; catalog_test.go fails when a type lacks a description.
var catalog = mustCatalog(dsl.SchemaJSON())

func mustCatalog(raw []byte) string {
	out, err := BuildCatalog(raw)
	if err != nil {
		panic("ai: build catalog: " + err.Error())
	}
	return out
}

// Schema metadata keys read by the catalog (ignored by JSON Schema validators
// and stripped from the generation schema by Relax).
const (
	keyUsage     = "x-usage"            // layout idiom for a component def
	keyEnumDescs = "x-enumDescriptions" // per-enum-value description on a `type` property
	keyFor       = "x-for"              // {enumValue: "required"|"optional"}: property applies only to those `type`s
)

// commonProps are present on every component and listed once, not per type.
var commonProps = map[string]bool{"type": true, "id": true, "area": true}

// BuildCatalog renders the component and action catalog from the canonical
// AppDSL schema: one line per type ("type: 必須 / 任意 — description" and
// "使い方: x-usage"), the helper objects they reference (ListItem/Sort/Filter/
// Notification), then one line per action type.
func BuildCatalog(schema []byte) (string, error) {
	var root map[string]any
	if err := json.Unmarshal(schema, &root); err != nil {
		return "", err
	}
	defs, _ := root["$defs"].(map[string]any)
	if defs == nil {
		return "", fmt.Errorf("schema has no $defs")
	}
	var b strings.Builder
	helpers := map[string]bool{}

	b.WriteString("全部品共通の任意キー: id(画面内一意)、area(Screen 直下のみ)。\n")
	b.WriteString("### Container(children は Leaf のみ)\n")
	container, _ := defs["Container"].(map[string]any)
	if container == nil {
		return "", fmt.Errorf("schema has no $defs/Container")
	}
	for _, t := range enumValues(container, "type") {
		writeTypeLine(&b, defs, t, container, enumDesc(container, t), "", helpers)
	}
	if usage, _ := container[keyUsage].(string); usage != "" {
		fmt.Fprintf(&b, "  使い方: %s\n", usage)
	}

	b.WriteString("### Leaf\n")
	leafNames := refNames(defs["Leaf"])
	if len(leafNames) == 0 {
		return "", fmt.Errorf("schema has no $defs/Leaf anyOf")
	}
	for _, name := range leafNames {
		def, _ := defs[name].(map[string]any)
		t := constType(def)
		if t == "" {
			return "", fmt.Errorf("$defs/%s has no properties.type.const", name)
		}
		desc, _ := def["description"].(string)
		usage, _ := def[keyUsage].(string)
		writeTypeLine(&b, defs, t, def, desc, usage, helpers)
	}

	b.WriteString("### アクション(button.action / toolbar.action / list.onItemTap)\n")
	action, _ := defs["Action"].(map[string]any)
	if action == nil {
		return "", fmt.Errorf("schema has no $defs/Action")
	}
	for _, t := range enumValues(action, "type") {
		fmt.Fprintf(&b, "- %s{%s}", t, propList(defs, action, t, true, helpers))
		if d := enumDesc(action, t); d != "" {
			b.WriteString(" — " + d)
		}
		b.WriteString("\n")
	}
	if common := propList(defs, action, "*", false, nil); common != "" {
		fmt.Fprintf(&b, "- どの action にも付けられる任意キー: %s\n", common)
	}

	b.WriteString("### 補助オブジェクト\n")
	for _, name := range sortedBoolKeys(helpers) {
		def, _ := defs[name].(map[string]any)
		desc, _ := def["description"].(string)
		fmt.Fprintf(&b, "- %s{%s}", helperLabel(name), propList(defs, def, "", false, nil))
		if desc != "" {
			b.WriteString(" — " + desc)
		}
		b.WriteString("\n")
	}

	b.WriteString("### Entity の scope(レコードを誰と共有するか・省略時 private)\n")
	entity, _ := defs["Entity"].(map[string]any)
	scopes := enumValues(entity, "scope")
	if len(scopes) == 0 {
		return "", fmt.Errorf("schema has no $defs/Entity.properties.scope enum")
	}
	for _, v := range scopes {
		fmt.Fprintf(&b, "- scope: %s", v)
		if d := enumDescOf(entity, "scope", v); d != "" {
			b.WriteString(" — " + d)
		}
		b.WriteString("\n")
	}
	if props, _ := entity["properties"].(map[string]any); props != nil {
		if sp, _ := props["scope"].(map[string]any); sp != nil {
			if usage, _ := sp[keyUsage].(string); usage != "" {
				fmt.Fprintf(&b, "  使い方: %s\n", usage)
			}
		}
	}
	if err := writeExprCatalog(&b, defs); err != nil {
		return "", err
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// writeExprCatalog renders the calculation-expression section: the five node
// shapes, one line per op (from $defs/ExprOp x-enumDescriptions) and the
// recipes ($defs/Expr x-recipes).
func writeExprCatalog(b *strings.Builder, defs map[string]any) error {
	expr, _ := defs["Expr"].(map[string]any)
	if expr == nil {
		return fmt.Errorf("schema has no $defs/Expr")
	}
	desc, _ := expr["description"].(string)
	fmt.Fprintf(b, "### 計算式 Expr(Field.computed と stat.value・agg の filter.value に書ける)— %s\n", desc)
	if usage, _ := expr[keyUsage].(string); usage != "" {
		fmt.Fprintf(b, "  使い方: %s\n", usage)
	}
	descOf := func(name string) string {
		d, _ := defs[name].(map[string]any)
		s, _ := d["description"].(string)
		return s
	}
	b.WriteString("- リテラル: 12 / \"文字\" / true\n")
	fmt.Fprintf(b, "- {\"ref\":\"<fieldId>\"} — %s\n", descOf("ExprRef"))
	fmt.Fprintf(b, "- {\"agg\":\"count|sum|avg|min|max\",\"entity\":\"<entityId>\",\"field\"?:\"<fieldId>\",\"filter\"?:filter|[filter]} — %s\n", descOf("ExprAgg"))
	fmt.Fprintf(b, "- {\"if\":Expr,\"then\":Expr,\"else\":Expr} — %s\n", descOf("ExprIf"))
	op, _ := defs["ExprOp"].(map[string]any)
	ops := enumValues(op, "op")
	if len(ops) == 0 {
		return fmt.Errorf("schema has no $defs/ExprOp.properties.op enum")
	}
	fmt.Fprintf(b, "- {\"op\":\"<name>\",\"args\":[Expr,...]} — %s 一覧:\n", descOf("ExprOp"))
	for _, o := range ops {
		fmt.Fprintf(b, "- {\"op\":\"%s\"} %s\n", o, enumDescOf(op, "op", o))
	}
	recipes, _ := expr["x-recipes"].([]any)
	if len(recipes) > 0 {
		b.WriteString("#### 計算レシピ(よくある計算の正解形。そのまま使ってよい)\n")
		for _, r := range recipes {
			fmt.Fprintf(b, "- %v\n", r)
		}
	}
	return nil
}

// writeTypeLine renders "- type: req / opt — desc" + "  使い方: usage".
func writeTypeLine(b *strings.Builder, defs map[string]any, t string, def map[string]any, desc, usage string, helpers map[string]bool) {
	fmt.Fprintf(b, "- %s: %s", t, propList(defs, def, t, false, helpers))
	if desc != "" {
		b.WriteString(" — " + desc)
	}
	b.WriteString("\n")
	if usage != "" {
		fmt.Fprintf(b, "  使い方: %s\n", usage)
	}
}

// propList renders "req1, req2 / opt1, opt2(enum|values)" for one def.
// forType filters properties by x-for: "" = no filtering, "*" = only
// properties without x-for (shared by all types), otherwise properties that
// list forType plus, unless skipCommon, those without x-for. helpers collects
// referenced helper defs.
func propList(defs map[string]any, def map[string]any, forType string, skipCommon bool, helpers map[string]bool) string {
	props, _ := def["properties"].(map[string]any)
	required := map[string]bool{}
	if req, ok := def["required"].([]any); ok {
		for _, r := range req {
			required[r.(string)] = true
		}
	}
	var reqs, opts []string
	for _, k := range propertyOrder(def) {
		if commonProps[k] && forType != "" && forType != "*" {
			continue
		}
		if k == "type" {
			continue
		}
		ps, _ := props[k].(map[string]any)
		xfor, hasFor := ps[keyFor].(map[string]any)
		isReq := required[k]
		switch {
		case forType == "*":
			if hasFor {
				continue
			}
		case forType != "" && hasFor:
			mode, ok := xfor[forType].(string)
			if !ok {
				continue
			}
			isReq = mode == "required"
		case forType != "" && skipCommon:
			continue
		}
		label := k + propSuffix(defs, ps, helpers)
		if isReq {
			reqs = append(reqs, label)
		} else {
			opts = append(opts, label)
		}
	}
	switch {
	case len(reqs) == 0 && len(opts) == 0:
		if forType == "" || forType == "*" {
			return ""
		}
		return "(追加キー無し)"
	case len(opts) == 0:
		return strings.Join(reqs, ", ")
	case len(reqs) == 0:
		if forType == "*" {
			return strings.Join(opts, ", ")
		}
		return "/ " + strings.Join(opts, ", ")
	default:
		return strings.Join(reqs, ", ") + " / " + strings.Join(opts, ", ")
	}
}

// propSuffix renders "(a|b|c)" for enums and "{req keys}" for object refs.
func propSuffix(defs map[string]any, ps map[string]any, helpers map[string]bool) string {
	if en, ok := ps["enum"].([]any); ok {
		vals := make([]string, 0, len(en))
		for _, v := range en {
			vals = append(vals, fmt.Sprint(v))
		}
		return "(" + strings.Join(vals, "|") + ")"
	}
	ref, _ := ps["$ref"].(string)
	if ref == "" {
		return ""
	}
	name := strings.TrimPrefix(ref, "#/$defs/")
	target, _ := defs[name].(map[string]any)
	if target == nil || target["type"] != "object" || name == "Action" {
		return ""
	}
	if helpers != nil {
		helpers[name] = true
	}
	var reqs []string
	if req, ok := target["required"].([]any); ok {
		for _, r := range req {
			reqs = append(reqs, r.(string))
		}
	}
	if len(reqs) == 0 {
		return ""
	}
	return "{" + strings.Join(reqs, ",") + "}"
}

// helperLabel maps a helper def to the property name it appears under.
func helperLabel(name string) string {
	switch name {
	case "ListItem":
		return "item"
	default:
		return strings.ToLower(name[:1]) + name[1:]
	}
}

// propertyOrder returns property names: required first (schema order), then
// the rest alphabetically (encoding/json loses object key order).
func propertyOrder(def map[string]any) []string {
	props, _ := def["properties"].(map[string]any)
	var order []string
	seen := map[string]bool{}
	if req, ok := def["required"].([]any); ok {
		for _, r := range req {
			k := r.(string)
			if _, ok := props[k]; ok && !seen[k] {
				order = append(order, k)
				seen[k] = true
			}
		}
	}
	rest := make([]string, 0, len(props))
	for k := range props {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(order, rest...)
}

func enumValues(def map[string]any, prop string) []string {
	props, _ := def["properties"].(map[string]any)
	ps, _ := props[prop].(map[string]any)
	en, _ := ps["enum"].([]any)
	out := make([]string, 0, len(en))
	for _, v := range en {
		out = append(out, fmt.Sprint(v))
	}
	return out
}

func enumDesc(def map[string]any, value string) string { return enumDescOf(def, "type", value) }

// enumDescOf reads properties.<prop>.x-enumDescriptions[value].
func enumDescOf(def map[string]any, prop, value string) string {
	props, _ := def["properties"].(map[string]any)
	ps, _ := props[prop].(map[string]any)
	m, _ := ps[keyEnumDescs].(map[string]any)
	s, _ := m[value].(string)
	return s
}

func constType(def map[string]any) string {
	props, _ := def["properties"].(map[string]any)
	ps, _ := props["type"].(map[string]any)
	s, _ := ps["const"].(string)
	return s
}

func sortedBoolKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
