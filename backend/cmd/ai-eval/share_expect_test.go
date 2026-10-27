package main

import (
	"encoding/json"
	"testing"
)

// #254: d_family_todo expects shared, no public, and the link-members note.
func TestCheckExpect_Share(t *testing.T) {
	var exp map[string]any
	if err := json.Unmarshal([]byte(`{"scope":"shared","scopeNot":"public","summaryHas":["招待した人だけが見られます"]}`), &exp); err != nil {
		t.Fatal(err)
	}
	ok := "家事は、作った人とリンクで招待した人だけが見られます(共有シートからリンクを送って招待)。"
	if miss := checkExpect(exp, &Metrics{Scopes: []string{"chores:shared"}}, ok); len(miss) != 0 {
		t.Errorf("want no misses: %v", miss)
	}
	miss := checkExpect(exp, &Metrics{Scopes: []string{"chores:public"}}, "家事は誰でも見られます。")
	if len(miss) != 3 {
		t.Errorf("want scope, scopeNot and summaryHas misses: %v", miss)
	}
}
