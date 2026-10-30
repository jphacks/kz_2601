package main

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func parseBoard(t *testing.T, edit func(string) string) *dsl.App {
	t.Helper()
	raw, err := os.ReadFile("../../../docs/dsl/examples/board.json")
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse([]byte(edit(string(raw))))
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func TestAuthorMetrics(t *testing.T) {
	m := measure(parseBoard(t, func(s string) string { return s }))
	if !slices.Contains(m.AuthorShown, "home:list") || !slices.Equal(m.MeFilter, []string{"mine:stat", "mine:table"}) || len(m.NameInputs) != 0 {
		t.Fatalf("authorShown=%v meFilter=%v nameInputs=%v", m.AuthorShown, m.MeFilter, m.NameInputs)
	}
	if miss := checkExpect(map[string]any{"authorShown": true, "meFilter": true, "nameInput": false}, m, ""); len(miss) != 0 {
		t.Fatal(miss)
	}
}

func TestNameInputsFlagTypedNames(t *testing.T) {
	app := parseBoard(t, func(s string) string {
		return strings.Replace(s, `{ "id": "place", "name": "拾った場所", "type": "string" },`,
			`{ "id": "place", "name": "拾った場所", "type": "string" }, { "id": "member_name", "name": "名前", "type": "string" }, { "id": "staff", "name": "担当", "type": "string" },`, 1)
	})
	if got := nameInputs(app); !slices.Equal(got, []string{"items.member_name", "items.staff"}) {
		t.Fatalf("nameInputs = %v", got)
	}
	// private entities may keep a name field (a personal contact list)
	app.Entities[0].Scope = ""
	if got := nameInputs(app); len(got) != 0 {
		t.Fatalf("private entity flagged: %v", got)
	}
}
