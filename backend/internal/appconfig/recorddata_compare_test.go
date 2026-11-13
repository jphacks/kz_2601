package appconfig

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCompareValues(t *testing.T) {
	cases := []struct {
		name string
		a, b any
		want int
		ok   bool
	}{
		{"float less", 1.0, 2.0, -1, true},
		{"float greater", 3.5, 2.0, 1, true},
		{"float equal", 2.0, 2.0, 0, true},
		{"int64 vs float64 equal", int64(2), 2.0, 0, true},
		{"int64 vs float64 less", int64(1), 1.5, -1, true},
		{"int vs json.Number", 3, json.Number("2"), 1, true},
		{"string less", "a", "b", -1, true},
		{"string equal", "x", "x", 0, true},
		{"string greater", "b", "a", 1, true},
		{"bool false < true", false, true, -1, true},
		{"bool true > false", true, false, 1, true},
		{"bool equal", true, true, 0, true},
		{"nil both", nil, nil, 0, true},
		{"nil sorts last (a)", nil, 1.0, 1, true},
		{"nil sorts last (b)", "x", nil, -1, true},
		{"number vs string", 1.0, "1", 0, false},
		{"string vs number", "1", 1.0, 0, false},
		{"bool vs string", true, "true", 0, false},
		{"unknown types", []any{1}, []any{1}, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := compareValues(c.a, c.b)
			if got != c.want || ok != c.ok {
				t.Fatalf("compareValues(%#v, %#v) = %d, %v; want %d, %v", c.a, c.b, got, ok, c.want, c.ok)
			}
		})
	}
}

func TestParseFilterValue(t *testing.T) {
	cases := []struct {
		name    string
		typ     dsl.FieldType
		in      string
		want    any
		wantErr string
	}{
		{"integer", dsl.FieldInteger, "42", 42.0, ""},
		{"currency negative", dsl.FieldCurrency, "-100", -100.0, ""},
		{"number fraction", dsl.FieldNumber, "1.5", 1.5, ""},
		{"number bad", dsl.FieldNumber, "abc", nil, "must be numeric"},
		{"integer empty", dsl.FieldInteger, "", nil, "must be numeric"},
		{"boolean true", dsl.FieldBoolean, "true", true, ""},
		{"boolean 0", dsl.FieldBoolean, "0", false, ""},
		{"boolean bad", dsl.FieldBoolean, "yes", nil, "must be a boolean"},
		{"string kept", dsl.FieldString, "42", "42", ""},
		{"date kept", dsl.FieldDate, "2026-11-12", "2026-11-12", ""},
		{"enum kept", dsl.FieldEnum, "a", "a", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseFilterValue(&dsl.Field{ID: "f", Type: c.typ}, c.in)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, c.wantErr)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("got %#v, %v; want %#v", got, err, c.want)
			}
		})
	}
}

func TestAsFloat(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want float64
		ok   bool
	}{
		{"float64", 1.5, 1.5, true},
		{"float32", float32(2.5), 2.5, true},
		{"int", 3, 3, true},
		{"int64", int64(-4), -4, true},
		{"int32", int32(5), 5, true},
		{"json.Number", json.Number("6.25"), 6.25, true},
		{"json.Number bad", json.Number("x"), 0, false},
		{"string", "7", 0, false},
		{"bool", true, 0, false},
		{"nil", nil, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := asFloat(c.in)
			if got != c.want || ok != c.ok {
				t.Fatalf("asFloat(%#v) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
			}
		})
	}
}
