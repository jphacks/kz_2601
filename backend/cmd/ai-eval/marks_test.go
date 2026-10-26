package main

import (
	"os"
	"slices"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func parseExample(t *testing.T, name string) *dsl.App {
	t.Helper()
	raw, err := os.ReadFile("../../../docs/dsl/examples/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func TestMarkMetrics(t *testing.T) {
	app := parseExample(t, "recipes")
	toggles, mine := markUses(app)
	if !slices.Equal(toggles, []string{"home:list:save", "saved:list:save"}) || !slices.Equal(mine, []string{"saved:stat:save", "saved:list:save"}) {
		t.Fatalf("toggles=%v mine=%v", toggles, mine)
	}
	if miss := checkMarks(map[string]any{"mark": true, "markedByMe": true, "markEntity": false}, app); len(miss) != 0 {
		t.Fatal(miss)
	}
	// the board example has neither
	if miss := checkMarks(map[string]any{"mark": true}, parseExample(t, "board")); len(miss) != 1 {
		t.Fatalf("board must miss mark: %v", miss)
	}
}

func TestMarkEntitiesFlagHandMadeLikes(t *testing.T) {
	app := parseExample(t, "recipes")
	app.Entities = append(app.Entities,
		dsl.Entity{ID: "likes", Name: "いいね", Fields: []dsl.Field{{ID: "post", Name: "投稿", Type: dsl.FieldRef, RefEntity: "recipes"}}},
		dsl.Entity{ID: "saved_recipe", Name: "保存したレシピ", Fields: []dsl.Field{{ID: "recipe", Name: "レシピ", Type: dsl.FieldRef, RefEntity: "recipes"}}},
		// a comment is a real child record, not a mark
		dsl.Entity{ID: "comments", Name: "コメント", Fields: []dsl.Field{{ID: "post", Name: "投稿", Type: dsl.FieldRef, RefEntity: "recipes"}}},
	)
	if got := markEntities(app); !slices.Equal(got, []string{"likes", "saved_recipe"}) {
		t.Fatalf("markEntities = %v", got)
	}
}
