package main

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func app2752(fields int, opts []string) *dsl.App {
	e := dsl.Entity{ID: "task"}
	names := []string{"title", "done", "memo", "due"}
	for i := 0; i < fields; i++ {
		e.Fields = append(e.Fields, dsl.Field{ID: names[i]})
	}
	e.Fields = append(e.Fields, dsl.Field{ID: "status", Options: opts})
	return &dsl.App{Entities: []dsl.Entity{e}}
}

func TestCheckKeep2752(t *testing.T) {
	a := app2752(2, nil) // title, done, status
	tests := []struct {
		name    string
		exp     map[string]any
		summary string
		want    string // substring of the single miss, "" = pass
	}{
		{"no keepEntity", map[string]any{"keep": []any{"title"}}, "", ""},
		{"entity missing", map[string]any{"keepEntity": "nope"}, "", "keepEntity missing nope"},
		{"all kept", map[string]any{"keepEntity": "task", "keep": []any{"title", "done"}}, "", ""},
		{"removed field", map[string]any{"keepEntity": "task", "keep": []any{"title", "gone"}}, "", "removed gone"},
		{"grew enough", map[string]any{"keepEntity": "task", "growTo": float64(3)}, "", ""},
		{"short without cap note", map[string]any{"keepEntity": "task", "growTo": float64(5)}, "追加しました", "fields=3 < 5"},
		{"short, ja cap note", map[string]any{"keepEntity": "task", "growTo": float64(5)}, "項目の上限のため", ""},
		{"short, en cap note", map[string]any{"keepEntity": "task", "growTo": float64(5)}, "Reached the LIMIT", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			miss := checkKeep(tc.exp, a, tc.summary)
			if tc.want == "" {
				if len(miss) != 0 {
					t.Fatalf("want pass, got %v", miss)
				}
				return
			}
			if len(miss) != 1 || !strings.Contains(miss[0], tc.want) {
				t.Fatalf("want miss containing %q, got %v", tc.want, miss)
			}
		})
	}
}

func TestCheckKeepBothMisses2752(t *testing.T) {
	exp := map[string]any{"keepEntity": "task", "keep": []any{"gone"}, "growTo": float64(9)}
	if miss := checkKeep(exp, app2752(1, nil), ""); len(miss) != 2 {
		t.Fatalf("want removed + growTo misses, got %v", miss)
	}
}

func TestCheckEnum2752(t *testing.T) {
	a := app2752(1, []string{"todo", "doing", "done"})
	// a ref field with options, and a screen showing its target entity
	ref := &dsl.App{
		Entities: []dsl.Entity{
			{ID: "task", Fields: []dsl.Field{{ID: "cat", Type: dsl.FieldRef, RefEntity: "cat", Options: []string{"a"}}}},
			{ID: "cat"},
		},
		Screens: []dsl.Screen{{ID: "cats", Entity: "cat"}},
	}
	tests := []struct {
		name string
		exp  map[string]any
		app  *dsl.App
		want string
	}{
		{"no enumField", map[string]any{"optHas": []any{"x"}}, a, ""},
		{"entity missing", map[string]any{"enumField": "nope.status"}, a, "enumField missing nope.status"},
		{"field missing", map[string]any{"enumField": "task.nope"}, a, "enumField missing task.nope"},
		{"optHas ok", map[string]any{"enumField": "task.status", "optHas": []any{"todo", "done"}}, a, ""},
		{"optHas absent", map[string]any{"enumField": "task.status", "optHas": []any{"later"}}, a, "optHas later"},
		{"optNot ok", map[string]any{"enumField": "task.status", "optNot": []any{"later"}}, a, ""},
		{"optNot present", map[string]any{"enumField": "task.status", "optNot": []any{"doing"}}, a, "optNot doing"},
		{"fields ok", map[string]any{"enumField": "task.status", "fields": float64(2)}, a, ""},
		{"fields wrong", map[string]any{"enumField": "task.status", "fields": float64(4)}, a, "fields=2 (want 4)"},
		{"refPick ok", map[string]any{"enumField": "task.cat", "refPick": true}, ref, ""},
		{"refPick not ref", map[string]any{"enumField": "task.status", "refPick": true}, a, "refPick=false"},
		{"refPick want false but is", map[string]any{"enumField": "task.cat", "refPick": false}, ref, "refPick=true"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			miss := checkEnum(tc.exp, tc.app)
			if tc.want == "" {
				if len(miss) != 0 {
					t.Fatalf("want pass, got %v", miss)
				}
				return
			}
			if len(miss) != 1 || !strings.Contains(miss[0], tc.want) {
				t.Fatalf("want miss containing %q, got %v", tc.want, miss)
			}
		})
	}
}
