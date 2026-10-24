package ai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// kakeiboWithoutLists drops every list (and a section holding one) from the
// kakeibo few-shot: the expense records are then a dead end (#130 a04).
func kakeiboWithoutLists(t *testing.T) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(fewShotKakeibo), &doc); err != nil {
		t.Fatal(err)
	}
	hasList := func(c map[string]any) bool {
		if c["type"] == "list" {
			return true
		}
		ch, _ := c["children"].([]any)
		for _, x := range ch {
			if xm, _ := x.(map[string]any); xm["type"] == "list" {
				return true
			}
		}
		return false
	}
	for _, s := range doc["screens"].([]any) {
		sm := s.(map[string]any)
		var keep []any
		for _, c := range sm["components"].([]any) {
			if !hasList(c.(map[string]any)) {
				keep = append(keep, c)
			}
		}
		sm["components"] = keep
	}
	raw, _ := json.Marshal(doc)
	return raw
}

func TestAddMissingListsFixesDeadEnd(t *testing.T) {
	raw := kakeiboWithoutLists(t)
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(app.UnlistedEntities()) == 0 {
		t.Fatal("fixture should have an unlisted entity")
	}
	fixed := addMissingLists(raw, app)
	got, err := dsl.Parse(fixed)
	if err != nil {
		t.Fatalf("fixed DSL must validate: %v", err)
	}
	if u := got.UnlistedEntities(); len(u) > 0 {
		t.Fatalf("still unlisted: %v", u)
	}
	for _, w := range got.Lint() {
		if strings.Contains(w, "has no list") {
			t.Fatalf("lint still warns: %s", w)
		}
	}
	if !strings.Contains(string(fixed), `"onItemTap":{"presentation":"push","screen":"expense_form","type":"navigate"}`) {
		t.Errorf("list should open the form: %s", fixed)
	}
}

func TestAddMissingListsKeepsCompleteApps(t *testing.T) {
	for _, raw := range []string{fewShotKakeibo, fewShotExpenseEN, fewShotWarikan} {
		app, err := dsl.Parse([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if got := addMissingLists([]byte(raw), app); string(got) != raw {
			t.Errorf("complete app must be returned unchanged")
		}
	}
}
