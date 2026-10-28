package main

import (
	"os"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #260: the chat example meets the chat_group expectations (chatThread shows
// the sender, no list, public, no typed-in name).
func TestChatThreadExpect(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/dsl/examples/chat.json")
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	exp := map[string]any{"has": []any{"chatThread"}, "lacks": []any{"list"}, "scope": "public", "authorShown": true, "nameInput": false}
	if miss := checkExpect(exp, measure(app), ""); len(miss) != 0 {
		t.Fatal(miss)
	}
	if miss := checkNav(exp, app, ""); len(miss) != 0 {
		t.Fatal(miss)
	}
}
