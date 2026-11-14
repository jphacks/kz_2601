package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

func mustParse(t *testing.T, s string) *object {
	t.Helper()
	o, err := parseOrdered([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// TestObjectGet: get never panics (nil receiver, missing key) and returns the
// parsed value as is; the typed accessors built on it fall back to zero values.
func TestObjectGet_3193(t *testing.T) {
	o := mustParse(t, `{"s":"x","n":3,"b":true,"nil":null,"o":{"k":"v"},"a":["p",2,"q"],"e":""}`)
	inner := o.obj("o")

	for _, tc := range []struct {
		name string
		o    *object
		key  string
		want any
	}{
		{"nil object", nil, "s", nil},
		{"missing key", o, "zzz", nil},
		{"empty key", o, "", nil},
		{"string", o, "s", "x"},
		{"empty string", o, "e", ""},
		{"number keeps its text (UseNumber)", o, "n", json.Number("3")},
		{"bool", o, "b", true},
		{"null", o, "nil", nil},
		{"nested object", o, "o", inner},
		{"key of the nested object only", o, "k", nil},
		{"nested lookup", inner, "k", "v"},
		{"array", o, "a", []any{"p", json.Number("2"), "q"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.o.get(tc.key); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("get(%q) = %#v, want %#v", tc.key, got, tc.want)
			}
		})
	}

	// has tells null from missing; the typed accessors give zero values on a
	// different type, a missing key and a nil receiver.
	if !o.has("nil") || o.has("zzz") || (*object)(nil).has("s") {
		t.Error("has: null counts as present, missing and nil receiver do not")
	}
	if o.str("s") != "x" || o.str("n") != "" || o.str("zzz") != "" || (*object)(nil).str("s") != "" {
		t.Error("str")
	}
	if o.obj("o") == nil || o.obj("s") != nil || (*object)(nil).obj("o") != nil {
		t.Error("obj")
	}
	if len(o.arr("a")) != 3 || o.arr("s") != nil || (*object)(nil).arr("a") != nil {
		t.Error("arr")
	}
	// strings keeps only the string items, in order
	if got := o.strings("a"); !reflect.DeepEqual(got, []string{"p", "q"}) {
		t.Errorf("strings = %v", got)
	}
	if got := o.strings("s"); got != nil {
		t.Errorf("strings of a non-array = %v", got)
	}
	// keys keep the schema order
	if want := []string{"s", "n", "b", "nil", "o", "a", "e"}; !reflect.DeepEqual(o.keys, want) {
		t.Errorf("keys = %v", o.keys)
	}
}

func TestParseOrderedErrors_3193(t *testing.T) {
	for name, in := range map[string]string{
		"not an object": `[1,2]`,
		"scalar":        `"x"`,
		"truncated":     `{"a":`,
		"empty":         ``,
	} {
		if _, err := parseOrdered([]byte(in)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
