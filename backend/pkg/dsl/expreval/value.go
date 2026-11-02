// Package expreval evaluates DSL expressions (dsl.Expr) exactly like the iOS
// evaluator (ios/Packages/MetamonDSL/Sources/MetamonDSL/Logic/ExprEvaluator.swift).
//
// iOS is the source of truth. The shared cases in docs/dsl/conformance/expr/
// are generated from iOS and this package must reproduce every one of them.
// Where Go and Swift/Foundation differ, this package imitates the Swift side;
// see doc.go for the list.
package expreval

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
)

// Kind is the type of a Value.
type Kind int

const (
	// Nil means "unavailable" (division by zero, type mismatch, missing
	// reference ...). iOS shows it as "—".
	Nil Kind = iota
	Number
	String
	Bool
	Date
)

func (k Kind) String() string {
	return [...]string{"nil", "number", "string", "bool", "date"}[k]
}

// Value is an evaluation result. The zero Value is Nil.
type Value struct {
	Kind Kind
	Num  float64
	Str  string
	Bool bool
	Time time.Time
}

// NumberValue, StringValue, BoolValue and DateValue build Values.
func NumberValue(n float64) Value   { return Value{Kind: Number, Num: n} }
func StringValue(s string) Value    { return Value{Kind: String, Str: s} }
func BoolValue(b bool) Value        { return Value{Kind: Bool, Bool: b} }
func DateValue(t time.Time) Value   { return Value{Kind: Date, Time: t} }
func (v Value) IsNil() bool         { return v.Kind == Nil }
func (v Value) isEmptyString() bool { return v.Kind == String && v.Str == "" }

// number is ExprValue.number: numbers, and strings that parse after removing
// commas ("1,200" = 1200). Bools and dates are not numbers.
func (v Value) number() (float64, bool) {
	switch v.Kind {
	case Number:
		return v.Num, true
	case String:
		return swiftDouble(strings.ReplaceAll(v.Str, ",", ""))
	}
	return 0, false
}

// truthy is ExprValue.truthy.
func (v Value) truthy() bool {
	switch v.Kind {
	case Number:
		return v.Num != 0
	case String:
		return v.Str != "" && v.Str != "false"
	case Bool:
		return v.Bool
	case Date:
		return true
	}
	return false
}

// Text is ExprValue.text: how concat / toString / len see the value.
// Dates are "YYYY-MM-DD" in loc.
func (v Value) Text(loc *time.Location) string {
	switch v.Kind {
	case Number:
		return numberDisplay(v.Num)
	case String:
		return v.Str
	case Bool:
		if v.Bool {
			return "true"
		}
		return "false"
	case Date:
		return v.Time.In(loc).Format("2006-01-02")
	}
	return ""
}

// recordJSON is ExprValue.json(for:): the value as stored in Record.data.
// Dates: datetime fields get RFC 3339 in UTC (ISO8601DateFormatter's default
// zone), everything else "YYYY-MM-DD" in loc.
func (v Value) recordJSON(datetime bool, loc *time.Location) any {
	switch v.Kind {
	case Number:
		return v.Num
	case String:
		return v.Str
	case Bool:
		return v.Bool
	case Date:
		if datetime {
			return v.Time.UTC().Format("2006-01-02T15:04:05Z")
		}
		return v.Time.In(loc).Format("2006-01-02")
	}
	return nil
}

// numberDisplay is JSONValue.displayString for a number: integers below 1e15
// without ".0", everything else as Swift's Double description.
func numberDisplay(n float64) string {
	if n == math.Round(n) && math.Abs(n) < 1e15 {
		return strconv.FormatInt(int64(n), 10)
	}
	return swiftDescription(n)
}

// swiftDescription is Swift's `String(Double)`: shortest round-trip digits,
// decimal notation for 1e-4 <= |n| <= 2^53 (always with a fraction part, "1e15"
// is "1000000000000000.0"), exponential otherwise ("1e+16", "1e-05").
func swiftDescription(n float64) string {
	switch {
	case math.IsNaN(n):
		return "nan"
	case math.IsInf(n, 1):
		return "inf"
	case math.IsInf(n, -1):
		return "-inf"
	}
	a := math.Abs(n)
	if a != 0 && (a < 1e-4 || a > 1<<53) {
		return strconv.FormatFloat(n, 'e', -1, 64)
	}
	s := strconv.FormatFloat(n, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// swiftDouble is Swift's `Double(String)` (strtod on the whole string): no
// surrounding spaces, "inf"/"infinity"/"nan" in any case, hex with or without a
// "p" exponent, overflow gives ±inf. Underscores and "0b" are rejected.
func swiftDouble(s string) (float64, bool) {
	if s == "" || strings.ContainsRune(s, '_') {
		return 0, false
	}
	body := strings.TrimLeft(s, "+-")
	if len(s)-len(body) > 1 {
		return 0, false
	}
	if len(body) > 1 && body[0] == '0' && (body[1] == 'x' || body[1] == 'X') && !strings.ContainsAny(body, "pP") {
		s += "p0"
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return f, true
		}
		return 0, false
	}
	return f, true
}

// ---- stored JSON values (Record.data) ---------------------------------------

// jsonDouble is JSONValue.doubleValue (numbers only).
func jsonDouble(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

// jsonDisplay is JSONValue.displayString.
func jsonDisplay(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return numberDisplay(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case nil:
		return ""
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = jsonDisplay(e)
		}
		return strings.Join(parts, ", ")
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return "{}"
		}
		return string(b)
	}
}

// jsonEqual is JSONValue ==.
func jsonEqual(a, b any) bool {
	switch x := a.(type) {
	case nil:
		return b == nil
	case float64:
		y, ok := b.(float64)
		return ok && x == y
	case string:
		y, ok := b.(string)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !jsonEqual(x[i], y[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, xv := range x {
			yv, ok := y[k]
			if !ok || !jsonEqual(xv, yv) {
				return false
			}
		}
		return true
	}
	return false
}

// normalizeJSON turns numeric Go types into float64 so values decoded or built
// by callers compare like JSON.
func normalizeJSON(v any) any {
	switch x := v.(type) {
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case float32:
		return float64(x)
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return string(x)
		}
		return f
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalizeJSON(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = normalizeJSON(e)
		}
		return out
	}
	return v
}
