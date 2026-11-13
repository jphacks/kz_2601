package main

import (
	"reflect"
	"testing"
)

// TestMergeShape: how two occurrences of one key are merged into one Kotlin type.
func TestMergeShape_3193(t *testing.T) {
	g := &generator{defs: mustParse(t, `{
		"Expr":   {"anyOf": [{"type": "string"}, {"type": "number"}]},
		"IntOrX": {"anyOf": [7, {"type": "integer"}]},
		"NoNum":  {"anyOf": [{"type": "string"}, {"type": "boolean"}]},
		"Empty":  {}
	}`)}
	str := func() *shape { return &shape{kind: kString} }
	num := func() *shape { return &shape{kind: kNumber} }
	integer := func() *shape { return &shape{kind: kInt} }
	enum := func(vs ...string) *shape { return &shape{kind: kEnum, enum: &enumOcc{values: vs}} }
	list := func(e *shape) *shape { return &shape{kind: kList, elem: e} }
	class := func(n string) *shape { return &shape{kind: kClass, class: n} }
	sealed := func(n string) *shape { return &shape{kind: kSealed, class: n} }

	for _, tc := range []struct {
		name string
		a, b *shape
		want *shape // compared by kind/class/elem; enums by values
	}{
		{"same scalar", str(), str(), str()},
		{"same bool", &shape{kind: kBool}, &shape{kind: kBool}, &shape{kind: kBool}},
		{"integer then number", integer(), num(), num()},
		{"number then integer", num(), integer(), num()},
		{"string vs number", str(), num(), &shape{kind: kJSON}},
		{"string vs bool", str(), &shape{kind: kBool}, &shape{kind: kJSON}},
		{"string vs class", str(), class("A"), &shape{kind: kJSON}},
		{"number into a sealed with a number variant", num(), sealed("Expr"), sealed("Expr")},
		{"sealed with a number variant, then integer", sealed("Expr"), integer(), sealed("Expr")},
		{"integer variant counts as a number", integer(), sealed("IntOrX"), sealed("IntOrX")},
		{"sealed without a number variant", num(), sealed("NoNum"), &shape{kind: kJSON}},
		{"sealed that is not in defs", num(), sealed("Missing"), &shape{kind: kJSON}},
		{"sealed without anyOf", num(), sealed("Empty"), &shape{kind: kJSON}},
		{"string into a sealed", str(), sealed("Expr"), &shape{kind: kJSON}},
		{"same class", class("A"), class("A"), class("A")},
		{"different classes", class("A"), class("B"), &shape{kind: kJSON}},
		{"same sealed", sealed("Expr"), sealed("Expr"), sealed("Expr")},
		{"different sealed", sealed("Expr"), sealed("NoNum"), &shape{kind: kJSON}},
		{"class vs sealed", class("Expr"), sealed("Expr"), &shape{kind: kJSON}},
		{"enums are unioned", enum("a", "b"), enum("b", "c"), enum("a", "b", "c")},
		{"same enum values", enum("a"), enum("a"), enum("a")},
		{"list of the same", list(str()), list(str()), list(str())},
		{"list of integer then number", list(integer()), list(num()), list(num())},
		{"list of different kinds", list(str()), list(num()), &shape{kind: kJSON}},
		{"list of enums", list(enum("a")), list(enum("b")), list(enum("a", "b"))},
		{"list of already-JSON stays a list", list(&shape{kind: kJSON}), list(str()), list(&shape{kind: kJSON})},
		{"list vs scalar", list(str()), str(), &shape{kind: kJSON}},
		{"list of lists", list(list(integer())), list(list(num())), list(list(num()))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := g.mergeShape(tc.a, tc.b)
			if !sameShape(got, tc.want) {
				t.Fatalf("got %s, want %s", describe(got), describe(tc.want))
			}
		})
	}
}

func sameShape(a, b *shape) bool {
	if a.kind != b.kind || a.class != b.class {
		return false
	}
	if a.kind == kEnum && !reflect.DeepEqual(a.enum.values, b.enum.values) {
		return false
	}
	if a.kind == kList {
		return sameShape(a.elem, b.elem)
	}
	return true
}

func describe(s *shape) string {
	switch s.kind {
	case kList:
		return "list(" + describe(s.elem) + ")"
	case kEnum:
		return "enum" + "(" + joinStrings(s.enum.values) + ")"
	}
	return "kind" + string(rune('0'+int(s.kind))) + ":" + s.class
}

func joinStrings(vs []string) string {
	out := ""
	for i, v := range vs {
		if i > 0 {
			out += ","
		}
		out += v
	}
	return out
}
