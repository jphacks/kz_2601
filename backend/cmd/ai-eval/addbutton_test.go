package main

import (
	"os"
	"slices"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #416: oneAdd misses on a real output with both the toolbar ＋ and the button.
func TestOneAddMetric(t *testing.T) {
	load := func(path string) *Metrics {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		a, err := dsl.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		return measure(a)
	}
	dup := load("../../internal/ai/examples/okozukai.json")
	if !slices.Equal(dup.DupAdd, []string{"home>追加"}) {
		t.Fatalf("dupAdd = %v", dup.DupAdd)
	}
	if miss := checkExpect(map[string]any{"oneAdd": true}, dup, ""); len(miss) != 1 {
		t.Fatalf("oneAdd should miss: %v", miss)
	}
	ok := load("../../../docs/dsl/examples/kakeibo.json")
	if miss := checkExpect(map[string]any{"oneAdd": true}, ok, ""); len(miss) > 0 {
		t.Fatalf("kakeibo: %v", miss)
	}
}
