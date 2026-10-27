package ai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// The catalog (generated from the schema) and the system prompt both teach
// table.groupBy + column aggregates (#261).
func TestGroupByRules_InPromptAndCatalog(t *testing.T) {
	for _, want := range []string{"groupBy", "aggregate(count|sum|avg|min|max)"} {
		if !strings.Contains(catalog, want) {
			t.Errorf("catalog lacks %q", want)
		}
	}
	tableLine := ""
	for _, l := range strings.Split(catalog, "\n") {
		if strings.HasPrefix(l, "- table:") {
			tableLine = l
		}
	}
	if !strings.Contains(tableLine, "groupBy") {
		t.Errorf("catalog table line lacks groupBy: %s", tableLine)
	}
	for _, want := range []string{"## グループごとの集計の一覧(#261)", `"groupBy":"shop"`, `"aggregate":"avg"`} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
}

// The canonical ranking example is valid and lint-clean, so the prompt's
// recipe is a shape the validator accepts.
func TestGroupByRules_RankingExampleValid(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "dsl", "examples", "ranking.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatalf("ranking.json must validate: %v", err)
	}
	if p := app.Lint(); len(p) > 0 {
		t.Fatalf("ranking.json lint: %v", p)
	}
}
