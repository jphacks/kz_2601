package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

const rowWindowHead = `{"entities":[{"id":"logs","fields":[{"id":"date","type":"date"},{"id":"done","type":"boolean"}]},`

// The parsed shape (dsl.go), without the schema's required names.
func parseRowWindow(t *testing.T, weeks string) *dsl.App {
	t.Helper()
	var app dsl.App
	if err := json.Unmarshal([]byte(rowWindowHead+weeks+`]}`), &app); err != nil {
		t.Fatal(err)
	}
	return &app
}

func TestCheckRowWindow(t *testing.T) {
	window := parseRowWindow(t, `{"id":"weeks","fields":[{"id":"start","type":"date"},
  {"id":"n","type":"integer","computed":{"op":"add","args":[0,{"agg":"count","entity":"logs","filter":[
   {"field":"date","op":"gte","value":{"op":"addDays","args":[{"ref":"start"},-7]}},{"field":"date","op":"lt","value":{"ref":"start"}}]}]}}]}`)
	today := parseRowWindow(t, `{"id":"weeks","fields":[{"id":"start","type":"date"},
  {"id":"n","type":"integer","computed":{"agg":"count","entity":"logs","filter":[
   {"field":"date","op":"gte","value":{"op":"addDays","args":[{"op":"today","args":[]},-6]}}]}}]}`)
	thisWeek := parseRowWindow(t, `{"id":"weeks","fields":[{"id":"start","type":"date"},
  {"id":"n","type":"integer","computed":{"agg":"count","entity":"logs","filter":{"field":"date","op":"thisWeek","value":null}}}]}`)
	byID := parseRowWindow(t, `{"id":"weeks","fields":[{"id":"start","type":"date"},
  {"id":"n","type":"integer","computed":{"agg":"count","entity":"logs","filter":{"field":"done","op":"eq","value":{"ref":"id"}}}}]}`)
	for _, tc := range []struct {
		name string
		exp  map[string]any
		app  *dsl.App
		miss int
	}{
		{"row window wanted", map[string]any{"rowWindow": true}, window, 0},
		{"today window is not row", map[string]any{"rowWindow": true}, today, 1},
		{"thisWeek is not row", map[string]any{"rowWindow": true}, thisWeek, 1},
		{"ref id eq is not a window", map[string]any{"rowWindow": true}, byID, 1},
		{"unwanted", map[string]any{"rowWindow": false}, window, 1},
		{"none wanted none", map[string]any{"rowWindow": false}, today, 0},
		{"no key", map[string]any{}, window, 0},
	} {
		if miss := checkRowWindow(tc.exp, tc.app); len(miss) != tc.miss {
			t.Errorf("%s: want %d misses, got %v", tc.name, tc.miss, miss)
		}
	}
}

// The example the key is named after must pass it.
func TestCheckRowWindowExample(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "dsl", "examples", "adaptive_study.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	got := rowWindowFields(app)
	if len(got) != 2 || got[0] != "weeks.last_total" || got[1] != "weeks.last_done" {
		t.Errorf("rowWindowFields = %v, want weeks.last_total, weeks.last_done", got)
	}
}
