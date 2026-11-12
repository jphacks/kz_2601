package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestExtractJSONTable2590(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "bare object", in: `{"a":1}`, want: `{"a":1}`},
		{name: "surrounding space", in: "\n  {\"a\":1}  \n", want: `{"a":1}`},
		{name: "prose before and after", in: `Here it is: {"a":1} Hope this helps.`, want: `{"a":1}`},
		{name: "code fence", in: "```json\n{\"a\":{\"b\":[1,2]}}\n```", want: `{"a":{"b":[1,2]}}`},
		{name: "two objects: the first", in: `{"a":1}{"b":2}`, want: `{"a":1}`},
		{name: "two objects with prose", in: `{"a":1} Wait, also {"b":2}`, want: `{"a":1}`},
		{name: "brace inside a string", in: `note {"s":"}{"} end`, want: `{"s":"}{"}`},
		{name: "broken: first to last brace", in: `x {"a": } y }`, want: `{"a": } y }`},
		{name: "broken: no closing brace", in: `prefix {"a":1`, want: `prefix {"a":1`},
		{name: "no object", in: "  not json  ", want: "not json"},
		{name: "empty", in: "", want: ""},
		{name: "array only", in: `[1,2]`, want: `[1,2]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(extractJSON(tc.in)); got != tc.want {
				t.Fatalf("extractJSON(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCanonTable2590(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "keys sorted and compacted", in: "{ \"b\": 1,\n \"a\": [ 2, 3 ] }", want: `{"a":[2,3],"b":1}`},
		{name: "already canonical", in: `{"a":1}`, want: `{"a":1}`},
		{name: "nested", in: `{"z":{"y":true,"x":null}}`, want: `{"z":{"x":null,"y":true}}`},
		{name: "scalar", in: ` "s" `, want: `"s"`},
		{name: "invalid is kept as-is", in: `{"a":`, want: `{"a":`},
		{name: "empty is kept as-is", in: ``, want: ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(canon([]byte(tc.in))); got != tc.want {
				t.Fatalf("canon(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
	// two spellings of the same value compare equal after canon
	if a, b := canon([]byte(`{"a":1,"b":2}`)), canon([]byte(`{"b":2, "a":1}`)); string(a) != string(b) {
		t.Errorf("canon differs: %s vs %s", a, b)
	}
}

func TestProblemsOfTable2590(t *testing.T) {
	ve := &dsl.ValidationError{Problems: []string{"p1", "p2"}}
	truncated := "output was truncated (max_tokens); produce a smaller app"
	cases := []struct {
		name string
		err  error
		stop string
		want []string
	}{
		{name: "validation problems", err: ve, stop: "end_turn", want: []string{"p1", "p2"}},
		{name: "validation problems truncated", err: ve, stop: "max_tokens", want: []string{"p1", "p2", truncated}},
		{name: "wrapped validation error", err: fmt.Errorf("check: %w", ve), stop: "", want: []string{"p1", "p2"}},
		{name: "other error", err: errors.New("bad json"), stop: "max_tokens", want: []string{"bad json"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := problemsOf(tc.err, &Response{StopReason: tc.stop})
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("problemsOf = %q, want %q", got, tc.want)
			}
		})
	}
	// the error's own list is not changed
	_ = problemsOf(ve, &Response{StopReason: "max_tokens"})
	if !reflect.DeepEqual(ve.Problems, []string{"p1", "p2"}) {
		t.Errorf("problems changed: %q", ve.Problems)
	}
}

func TestApplyUncheckedTable2590(t *testing.T) {
	cur := []byte(`{"a":1,"b":{"c":2}}`)
	cases := []struct {
		name   string
		ops    []dsl.PatchOp
		want   string // JSON, compared after canon
		wantOK bool
	}{
		{name: "replace", ops: []dsl.PatchOp{{Op: "replace", Path: "/a", Value: json.RawMessage(`5`)}}, want: `{"a":5,"b":{"c":2}}`, wantOK: true},
		{name: "add and remove", ops: []dsl.PatchOp{
			{Op: "add", Path: "/b/d", Value: json.RawMessage(`"x"`)},
			{Op: "remove", Path: "/a"},
		}, want: `{"b":{"c":2,"d":"x"}}`, wantOK: true},
		{name: "no ops", ops: []dsl.PatchOp{}, want: `{"a":1,"b":{"c":2}}`, wantOK: true},
		{name: "missing path", ops: []dsl.PatchOp{{Op: "replace", Path: "/nope/x", Value: json.RawMessage(`1`)}}},
		{name: "unknown op", ops: []dsl.PatchOp{{Op: "explode", Path: "/a"}}},
		{name: "failed test op", ops: []dsl.PatchOp{{Op: "test", Path: "/a", Value: json.RawMessage(`2`)}}},
		{name: "value not json", ops: []dsl.PatchOp{{Op: "add", Path: "/z", Value: json.RawMessage(`{oops`)}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := applyUnchecked(cur, tc.ops)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (%s)", ok, tc.wantOK, got)
			}
			if !ok {
				if got != nil {
					t.Fatalf("failed apply returned %s", got)
				}
				return
			}
			if string(canon(got)) != string(canon([]byte(tc.want))) {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
