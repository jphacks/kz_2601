package expreval

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestJSONEqualTable(t *testing.T) {
	obj := func() map[string]any {
		return map[string]any{"a": []any{1.0, "x", map[string]any{"b": nil}}, "c": true}
	}
	for _, tc := range []struct {
		name string
		a, b any
		want bool
	}{
		{"nil nil", nil, nil, true},
		{"nil vs zero", nil, 0.0, false},
		{"zero vs nil", 0.0, nil, false},
		{"float equal", 1.5, 1.5, true},
		{"float differ", 1.5, 2.0, false},
		{"float vs string", 1.0, "1", false},
		{"int is not a JSON number", 1, 1.0, false},
		{"normalized int equals float", normalizeJSON(1), 1.0, true},
		{"normalized int64 equals float", normalizeJSON(int64(3)), normalizeJSON(3.0), true},
		{"string equal", "あ", "あ", true},
		{"string differ", "a", "b", false},
		{"string vs bool", "true", true, false},
		{"bool equal", false, false, true},
		{"bool differ", true, false, false},
		{"bool vs nil", true, nil, false},
		{"array equal", []any{1.0, "x"}, []any{1.0, "x"}, true},
		{"array order", []any{1.0, "x"}, []any{"x", 1.0}, false},
		{"array length", []any{1.0}, []any{1.0, 1.0}, false},
		{"array vs object", []any{}, map[string]any{}, false},
		{"empty arrays", []any{}, []any{}, true},
		{"nested object equal", obj(), obj(), true},
		{"nested object deep differ", obj(), map[string]any{"a": []any{1.0, "x", map[string]any{"b": false}}, "c": true}, false},
		{"object missing key", map[string]any{"a": 1.0}, map[string]any{"b": 1.0}, false},
		{"object size", map[string]any{"a": 1.0}, map[string]any{"a": 1.0, "b": 2.0}, false},
		{"object vs array", map[string]any{}, []any{}, false},
		{"unknown type", struct{}{}, struct{}{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := jsonEqual(tc.a, tc.b); got != tc.want {
				t.Errorf("jsonEqual(%#v, %#v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestNormalizeJSONTable(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   any
		want any
	}{
		{"int", 7, 7.0},
		{"firestore int64", int64(9007199254740993), float64(9007199254740993)},
		{"negative int64", int64(-2), -2.0},
		{"float32", float32(0.5), 0.5},
		{"float64 stays", 2.25, 2.25},
		{"json.Number", json.Number("12.5"), 12.5},
		{"bad json.Number stays text", json.Number("1e999x"), "1e999x"},
		{"string stays", "1", "1"},
		{"nil stays", nil, nil},
		{"bool stays", true, true},
		{"array of ints", []any{1, int64(2), "x"}, []any{1.0, 2.0, "x"}},
		{"nested object", map[string]any{"n": int64(1), "a": []any{map[string]any{"m": 2}}}, map[string]any{"n": 1.0, "a": []any{map[string]any{"m": 2.0}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeJSON(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("normalizeJSON(%#v) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}
