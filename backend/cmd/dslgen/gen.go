package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"
)

// Package-level knobs. Everything else is derived from the schema.
const (
	kotlinPackage = "jp.kz2601.metamon.dsl"
	rootClass     = "AppDSL"
	componentName = "Component"
)

// sealedDefs are anyOf defs emitted as a sealed interface (primitives and
// objects told apart by their required keys). Other anyOf defs become JsonElement.
var sealedDefs = map[string]bool{"Expr": true}

// defaultOverrides keep the iOS fallbacks for required scalars
// (ios/Packages/MetamonDSL AppDocument.swift). Missing → this value instead of
// dropping the parent.
var defaultOverrides = map[string]string{
	"AppMeta.name": `"Tailor It"`,
	"Screen.title": `""`,
}

// enumFallbacks: enum name → wire value used for unknown / missing values.
// Enums not listed fall back to UNKNOWN.
var enumFallbacks = map[string]string{
	"VoiceEntry": "floating",
}

var kotlinReservedTypes = map[string]bool{
	"Any": true, "Array": true, "Boolean": true, "Char": true, "Double": true, "Float": true,
	"Int": true, "List": true, "Long": true, "Map": true, "Nothing": true, "Number": true,
	"Pair": true, "Result": true, "Set": true, "String": true, "Unit": true,
	"JsonElement": true, "JsonObject": true,
}

var kotlinKeywords = map[string]bool{
	"as": true, "break": true, "class": true, "continue": true, "do": true, "else": true,
	"false": true, "for": true, "fun": true, "if": true, "in": true, "interface": true,
	"is": true, "null": true, "object": true, "package": true, "return": true, "super": true,
	"this": true, "throw": true, "true": true, "try": true, "typealias": true, "typeof": true,
	"val": true, "var": true, "when": true, "while": true,
}

type kind int

const (
	kString kind = iota
	kInt
	kNumber
	kBool
	kEnum
	kClass
	kComponent
	kComponentType
	kSealed
	kList
	kJSON
	kJSONObject
)

type shape struct {
	kind  kind
	class string   // kClass / kSealed
	enum  *enumOcc // kEnum
	elem  *shape   // kList
}

// enumOcc is one string enum in the schema. Its Kotlin name is decided after
// every class is built (see nameEnums).
type enumOcc struct {
	owner, key string
	fixedName  string // a $defs string enum keeps its def name
	values     []string
	name       string
}

type prop struct {
	key      string
	shape    *shape
	required bool
}

type class struct {
	name      string
	props     []*prop
	implement string // sealed interface this class belongs to
}

type sealedVariant struct {
	primitive string // "string" / "number" / "integer" / "boolean"; empty for objects
	class     string
	required  []string
}

type sealedDef struct {
	name     string
	variants []sealedVariant
}

type generator struct {
	root      *object
	defs      *object
	leaves    []string
	leafSet   map[string]bool
	classes   []*class
	byName    map[string]*class
	building  map[string]bool
	sealed    []*sealedDef
	sealedSet map[string]*sealedDef
	compTypes []compType
	enums     []*enumOcc // unique, in emit order
	lenient   map[string]bool
}

type compType struct {
	wire      string
	container bool
	keys      []string
}

func newGenerator(schema []byte) (*generator, error) {
	root, err := parseOrdered(schema)
	if err != nil {
		return nil, err
	}
	g := &generator{
		root:      root,
		defs:      root.obj("$defs"),
		leafSet:   map[string]bool{},
		byName:    map[string]*class{},
		building:  map[string]bool{},
		sealedSet: map[string]*sealedDef{},
		lenient:   map[string]bool{},
	}
	if g.defs == nil {
		return nil, fmt.Errorf("schema has no $defs")
	}
	leaf := g.defs.obj("Leaf")
	if leaf == nil {
		return nil, fmt.Errorf("schema has no $defs.Leaf")
	}
	for _, v := range leaf.arr("anyOf") {
		if o, ok := v.(*object); ok {
			n := refName(o.str("$ref"))
			g.leaves = append(g.leaves, n)
			g.leafSet[n] = true
		}
	}
	if err := g.build(); err != nil {
		return nil, err
	}
	return g, nil
}

func refName(ref string) string { return strings.TrimPrefix(ref, "#/$defs/") }

func (g *generator) build() error {
	if err := g.buildClass(rootClass, g.root, ""); err != nil {
		return err
	}
	if err := g.buildComponent(); err != nil {
		return err
	}
	for _, name := range g.defs.keys {
		def := g.defs.obj(name)
		if def == nil || g.leafSet[name] || name == "Container" {
			continue
		}
		switch {
		case sealedDefs[name]:
			if err := g.buildSealed(name); err != nil {
				return err
			}
		case def.str("type") == "object" && def.has("properties"):
			if err := g.buildClass(name, def, ""); err != nil {
				return err
			}
		}
	}
	wires := make([]string, len(g.compTypes))
	for i, t := range g.compTypes {
		wires[i] = t.wire
	}
	if err := uniqueEntries("ComponentType", wires); err != nil {
		return err
	}
	return g.nameEnums()
}

func (g *generator) buildClass(name string, def *object, implement string) error {
	if g.byName[name] != nil || g.building[name] {
		return nil
	}
	if kotlinReservedTypes[name] {
		return fmt.Errorf("class name %q clashes with a Kotlin type", name)
	}
	g.building[name] = true
	c := &class{name: name, implement: implement}
	g.byName[name] = c
	g.classes = append(g.classes, c)
	required := def.strings("required")
	props := def.obj("properties")
	if props == nil {
		return nil
	}
	for _, k := range props.keys {
		s, err := g.shapeOf(name, k, props.obj(k))
		if err != nil {
			return fmt.Errorf("%s.%s: %w", name, k, err)
		}
		c.props = append(c.props, &prop{key: k, shape: s, required: slices.Contains(required, k)})
	}
	return nil
}

func (g *generator) shapeOf(owner, key string, p *object) (*shape, error) {
	if p == nil {
		return &shape{kind: kJSON}, nil
	}
	if ref := p.str("$ref"); ref != "" {
		return g.refShape(refName(ref))
	}
	if p.has("anyOf") || p.has("oneOf") {
		return &shape{kind: kJSON}, nil
	}
	t, ok := p.vals["type"].(string)
	if !ok {
		return &shape{kind: kJSON}, nil
	}
	switch t {
	case "string":
		if p.has("enum") {
			vals := p.strings("enum")
			if slices.Contains(vals, "") {
				return nil, fmt.Errorf("empty enum value")
			}
			return &shape{kind: kEnum, enum: &enumOcc{owner: owner, key: key, values: vals}}, nil
		}
		return &shape{kind: kString}, nil
	case "integer":
		return &shape{kind: kInt}, nil
	case "number":
		return &shape{kind: kNumber}, nil
	case "boolean":
		return &shape{kind: kBool}, nil
	case "array":
		elem, err := g.shapeOf(owner, key, p.obj("items"))
		if err != nil {
			return nil, err
		}
		return &shape{kind: kList, elem: elem}, nil
	case "object":
		if p.has("properties") {
			name := owner + pascal(key)
			if g.defs.has(name) || g.byName[name] != nil {
				return nil, fmt.Errorf("inline object name %s clashes with another class", name)
			}
			if err := g.buildClass(name, p, ""); err != nil {
				return nil, err
			}
			return &shape{kind: kClass, class: name}, nil
		}
		return &shape{kind: kJSONObject}, nil
	}
	return &shape{kind: kJSON}, nil
}

func (g *generator) refShape(name string) (*shape, error) {
	def := g.defs.obj(name)
	if def == nil {
		return nil, fmt.Errorf("unknown $ref %q", name)
	}
	switch {
	case name == componentName || name == "Leaf" || name == "Container" || g.leafSet[name]:
		return &shape{kind: kComponent}, nil
	case sealedDefs[name]:
		return &shape{kind: kSealed, class: name}, nil
	case def.has("anyOf") || def.has("oneOf"):
		return &shape{kind: kJSON}, nil
	}
	switch def.str("type") {
	case "string":
		if def.has("enum") {
			return &shape{kind: kEnum, enum: &enumOcc{fixedName: name, values: def.strings("enum")}}, nil
		}
		return &shape{kind: kString}, nil
	case "integer":
		return &shape{kind: kInt}, nil
	case "number":
		return &shape{kind: kNumber}, nil
	case "boolean":
		return &shape{kind: kBool}, nil
	case "object":
		if def.has("properties") {
			if err := g.buildClass(name, def, ""); err != nil {
				return nil, err
			}
			return &shape{kind: kClass, class: name}, nil
		}
		return &shape{kind: kJSONObject}, nil
	}
	return &shape{kind: kJSON}, nil
}

// buildComponent flattens Container + every Leaf into one class (#351 §7):
// a key shared by several parts keeps its type; enums are unioned; a key
// whose type differs between parts becomes JsonElement.
func (g *generator) buildComponent() error {
	c := &class{name: componentName}
	g.byName[componentName] = c
	g.classes = append(g.classes, c)
	c.props = append(c.props, &prop{key: "type", shape: &shape{kind: kComponentType}})
	index := map[string]*prop{}

	add := func(owner, key string, p *object) error {
		s, err := g.shapeOf(owner, key, p)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", owner, key, err)
		}
		if old, ok := index[key]; ok {
			old.shape = mergeShape(old.shape, s)
			return nil
		}
		np := &prop{key: key, shape: cloneForComponent(s, key)}
		index[key] = np
		c.props = append(c.props, np)
		return nil
	}

	container := g.defs.obj("Container")
	cprops := container.obj("properties")
	var containerKeys []string
	for _, k := range cprops.keys {
		if k == "type" {
			continue
		}
		containerKeys = append(containerKeys, k)
		if err := add("Container", k, cprops.obj(k)); err != nil {
			return err
		}
	}
	for _, wire := range cprops.obj("type").strings("enum") {
		keys := []string{"type"}
		for _, k := range containerKeys {
			if xf := cprops.obj(k).obj("x-for"); xf == nil || xf.has(wire) {
				keys = append(keys, k)
			}
		}
		g.compTypes = append(g.compTypes, compType{wire: wire, container: true, keys: keys})
	}
	for _, leaf := range g.leaves {
		def := g.defs.obj(leaf)
		props := def.obj("properties")
		wire := props.obj("type").str("const")
		if wire == "" {
			return fmt.Errorf("leaf %s has no type const", leaf)
		}
		keys := []string{"type"}
		for _, k := range props.keys {
			if k == "type" {
				continue
			}
			keys = append(keys, k)
			if err := add(leaf, k, props.obj(k)); err != nil {
				return err
			}
		}
		g.compTypes = append(g.compTypes, compType{wire: wire, keys: keys})
	}
	return nil
}

// cloneForComponent gives a component key its own enum (owned by Component)
// so unions do not leak into other classes.
func cloneForComponent(s *shape, key string) *shape {
	switch s.kind {
	case kEnum:
		return &shape{kind: kEnum, enum: &enumOcc{owner: componentName, key: key, values: slices.Clone(s.enum.values)}}
	case kList:
		return &shape{kind: kList, elem: cloneForComponent(s.elem, key)}
	}
	return s
}

func mergeShape(a, b *shape) *shape {
	if a.kind != b.kind {
		return &shape{kind: kJSON}
	}
	switch a.kind {
	case kEnum:
		for _, v := range b.enum.values {
			if !slices.Contains(a.enum.values, v) {
				a.enum.values = append(a.enum.values, v)
			}
		}
		return a
	case kList:
		m := mergeShape(a.elem, b.elem)
		if m.kind == kJSON && a.elem.kind != kJSON {
			return &shape{kind: kJSON}
		}
		a.elem = m
		return a
	case kClass, kSealed:
		if a.class != b.class {
			return &shape{kind: kJSON}
		}
	}
	return a
}

func (g *generator) buildSealed(name string) error {
	if g.sealedSet[name] != nil {
		return nil
	}
	sd := &sealedDef{name: name}
	g.sealedSet[name] = sd
	g.sealed = append(g.sealed, sd)
	for _, v := range g.defs.obj(name).arr("anyOf") {
		o, ok := v.(*object)
		if !ok {
			continue
		}
		if ref := o.str("$ref"); ref != "" {
			vn := refName(ref)
			def := g.defs.obj(vn)
			if def == nil || def.str("type") != "object" {
				return fmt.Errorf("%s: variant %s is not an object", name, vn)
			}
			if err := g.buildClass(vn, def, name); err != nil {
				return err
			}
			g.byName[vn].implement = name
			sd.variants = append(sd.variants, sealedVariant{class: vn, required: def.strings("required")})
			continue
		}
		switch t := o.str("type"); t {
		case "string", "number", "integer", "boolean":
			sd.variants = append(sd.variants, sealedVariant{primitive: t, class: name + pascal(t)})
		default:
			return fmt.Errorf("%s: unsupported variant %q", name, t)
		}
	}
	return nil
}

// nameEnums decides Kotlin names. A key whose enums agree everywhere gets
// the plain name (voiceEntry → VoiceEntry); otherwise owner + key
// (Action.type → ActionType).
func (g *generator) nameEnums() error {
	var occs []*enumOcc
	var walk func(s *shape)
	walk = func(s *shape) {
		switch s.kind {
		case kEnum:
			occs = append(occs, s.enum)
		case kList:
			walk(s.elem)
		}
	}
	for _, c := range g.classes {
		for _, p := range c.props {
			walk(p.shape)
		}
	}
	byKey := map[string][]*enumOcc{}
	for _, o := range occs {
		if o.fixedName == "" {
			byKey[o.key] = append(byKey[o.key], o)
		}
	}
	taken := func(n string) bool { return g.byName[n] != nil || g.sealedSet[n] != nil || kotlinReservedTypes[n] }
	for _, o := range occs {
		if o.fixedName != "" {
			o.name = o.fixedName
			continue
		}
		plain := pascal(o.key)
		same := true
		for _, other := range byKey[o.key] {
			if !sameSet(other.values, o.values) {
				same = false
				break
			}
		}
		if same && !taken(plain) {
			o.name = plain
		} else {
			o.name = o.owner + pascal(o.key)
		}
	}
	seen := map[string]*enumOcc{}
	for _, o := range occs {
		if prev, ok := seen[o.name]; ok {
			if !slices.Equal(prev.values, o.values) && !sameSet(prev.values, o.values) {
				return fmt.Errorf("enum name %s used for different values (%s.%s, %s.%s)", o.name, prev.owner, prev.key, o.owner, o.key)
			}
			continue
		}
		if taken(o.name) || o.name == "ComponentType" {
			return fmt.Errorf("enum name %s clashes with a class", o.name)
		}
		if err := uniqueEntries(o.name, o.values); err != nil {
			return err
		}
		seen[o.name] = o
		g.enums = append(g.enums, o)
	}
	sort.SliceStable(g.enums, func(i, j int) bool { return g.enums[i].name < g.enums[j].name })
	return nil
}

func uniqueEntries(enum string, values []string) error {
	seen := map[string]string{}
	for _, v := range values {
		n := screaming(v)
		if prev, ok := seen[n]; ok {
			return fmt.Errorf("enum %s: %q and %q map to the same entry %s", enum, prev, v, n)
		}
		seen[n] = v
	}
	return nil
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, v := range a {
		if !slices.Contains(b, v) {
			return false
		}
	}
	return true
}

// ---- naming ----------------------------------------------------------------

func pascal(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	var b strings.Builder
	for _, p := range parts {
		r := []rune(p)
		r[0] = unicode.ToUpper(r[0])
		b.WriteString(string(r))
	}
	return b.String()
}

// screaming turns a wire value into an enum entry name (textField → TEXT_FIELD).
func screaming(v string) string {
	var b strings.Builder
	prevLower := false
	for _, r := range v {
		switch {
		case unicode.IsUpper(r):
			if prevLower {
				b.WriteByte('_')
			}
			b.WriteRune(r)
			prevLower = false
		case unicode.IsLower(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToUpper(r))
			prevLower = true
		default:
			b.WriteByte('_')
			prevLower = false
		}
	}
	out := strings.Trim(b.String(), "_")
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	if out == "" || unicode.IsDigit(rune(out[0])) {
		out = "V_" + out
	}
	return out
}

func kotlinIdent(key string) string {
	if kotlinKeywords[key] {
		return "`" + key + "`"
	}
	return key
}
