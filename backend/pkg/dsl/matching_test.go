package dsl

import (
	"os"
	"strings"
	"testing"
)

func matchingApp(t *testing.T) *App {
	t.Helper()
	raw, err := os.ReadFile("testdata/matching.json")
	if err != nil {
		t.Fatal(err)
	}
	a, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// #507: a profile plus a matched entity parses and is found by the helpers.
func TestMatchingParses(t *testing.T) {
	a := matchingApp(t)
	if p := a.ProfileEntity(); p == nil || p.ID != "profile" {
		t.Fatalf("profile = %+v", p)
	}
	if got := a.MatchedEntities(); len(got) != 1 || got[0] != "contact" {
		t.Fatalf("matched = %v", got)
	}
	if !a.HasMatching() {
		t.Fatal("HasMatching")
	}
}

func TestMatchingRejects(t *testing.T) {
	cases := []struct {
		name string
		edit func(a *App)
		want string
	}{
		{"private", func(a *App) { a.Entities[0].Scope = "" }, "needs scope public or shared"},
		{"members edit", func(a *App) { a.Entities[1].Edit = EditMembers }, "omit edit"},
		{"location", func(a *App) {
			a.Entities[1].Fields = append(a.Entities[1].Fields, Field{ID: "where", Name: "場所", Type: FieldLocation})
		}, "coarse regions"},
		{"photo on profile", func(a *App) {
			a.Entities[0].Fields = append(a.Entities[0].Fields, Field{ID: "face", Name: "写真", Type: FieldPhoto})
		}, "leave photos out"},
		{"two profiles", func(a *App) { a.Entities[1].Match = MatchProfile }, "only one entity"},
		{"matched alone", func(a *App) { a.Entities[0].Match = "" }, `needs a match "profile"`},
		{"unknown", func(a *App) { a.Entities[0].Match = "dating" }, "must be profile or matched"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := matchingApp(t)
			c.edit(a)
			err := a.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
}
