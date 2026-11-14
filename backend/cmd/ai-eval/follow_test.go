package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestFollowMetrics(t *testing.T) {
	app := parseExample(t, "timeline")
	if miss := checkFollows(map[string]any{"followFilter": true, "followButton": true, "followEntity": false}, app); len(miss) != 0 {
		t.Fatal(miss)
	}
	if miss := checkFollows(map[string]any{"followFilter": true, "followButton": true}, parseExample(t, "board")); len(miss) != 2 {
		t.Fatalf("board has neither: %v", miss)
	}
	app.Entities = append(app.Entities, dsl.Entity{ID: "follows", Name: "フォロー"})
	if miss := checkFollows(map[string]any{"followEntity": false}, app); len(miss) != 1 {
		t.Fatalf("a hand-made follow entity must be flagged: %v", miss)
	}
}
