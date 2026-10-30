package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckGroupBy(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "dsl", "examples", "ranking.json"))
	if err != nil {
		t.Fatal(err)
	}
	ranking, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := groupedTables(ranking); len(got) != 1 || got[0] != "review:shop" {
		t.Fatalf("groupedTables = %v", got)
	}
	if miss := checkGroupBy(map[string]any{"groupedTable": true}, ranking); miss != nil {
		t.Fatalf("ranking: %v", miss)
	}
	if miss := checkGroupBy(map[string]any{"groupedTable": false}, ranking); miss == nil {
		t.Fatal("groupedTable=false should miss on the ranking example")
	}
	if miss := checkGroupBy(map[string]any{}, ranking); miss != nil {
		t.Fatalf("no expect key: %v", miss)
	}

	plain := &dsl.App{Screens: []dsl.Screen{{ID: "home", Components: []dsl.Component{
		{Type: dsl.CompList, Entity: "review"},
		{Type: dsl.CompTable, Entity: "review", Columns: []dsl.TableColumn{{Field: "shop"}, {Field: "score"}}},
	}}}}
	if miss := checkGroupBy(map[string]any{"groupedTable": true}, plain); miss == nil {
		t.Fatal("a per-record table must not count as grouped")
	}
}
