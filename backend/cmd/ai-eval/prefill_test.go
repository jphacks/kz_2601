package main

import (
	"os"
	"slices"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestPrefillRefsMetric(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/dsl/examples/post_comments.json")
	if err != nil {
		t.Fatal(err)
	}
	a, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	m := measure(a)
	if !slices.Equal(m.PrefillRefs, []string{"post_form>comment_form.post"}) {
		t.Fatalf("prefillRefs = %v", m.PrefillRefs)
	}
	if miss := checkExpect(map[string]any{"prefillRef": true}, m, ""); len(miss) > 0 {
		t.Fatalf("misses: %v", miss)
	}
	if miss := checkExpect(map[string]any{"prefillRef": false}, m, ""); len(miss) != 1 {
		t.Fatalf("prefillRef:false should miss: %v", miss)
	}
}
