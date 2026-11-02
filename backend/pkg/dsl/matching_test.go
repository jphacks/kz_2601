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

// #507 phase 2: list.item.interest and matchList only make sense on the
// match:"profile" entity.
func TestMatchingComponents(t *testing.T) {
	if err := matchingApp(t).Validate(); err != nil {
		t.Fatalf("testdata: %v", err)
	}
	if l := matchingApp(t).Lint(); len(l) > 0 {
		for _, w := range l {
			if strings.Contains(w, "matchList") || strings.Contains(w, "interest") {
				t.Errorf("lint: %s", w)
			}
		}
	}
	matchList := func(a *App) *Component { return &a.Screens[1].Components[0] }
	cases := []struct {
		name string
		edit func(a *App)
		want string
	}{
		{"interest on contact", func(a *App) {
			a.Screens[0].Components[1].Item.Interest = true
		}, `item.interest needs the list entity to have match "profile"`},
		{"matchList on contact", func(a *App) { matchList(a).Entity = "contact" }, `must be the match "profile" entity`},
		{"matchList no entity", func(a *App) { matchList(a).Entity = "" }, "matchList needs entity"},
		{"matchList missing entity", func(a *App) { matchList(a).Entity = "nope" }, `matchList entity "nope" does not exist`},
		{"bad show", func(a *App) { matchList(a).Show = "likes" }, "must be incoming, outgoing or matches"},
		{"no show", func(a *App) { matchList(a).Show = "" }, "must be incoming, outgoing or matches"},
		{"unknown field", func(a *App) { matchList(a).Item.Title = "age" }, `matchList item field "age" not in entity`},
		{"interest in matchList", func(a *App) { matchList(a).Item.Interest = true }, "matchList item uses only"},
		{"mark in matchList", func(a *App) { matchList(a).Item.Mark = "like" }, "matchList item uses only"},
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

// matchList lists the profile records one by one, so the profile entity
// counts as listed even without a list.
func TestMatchListCountsAsListed(t *testing.T) {
	a := matchingApp(t)
	a.Screens[0].Components = a.Screens[0].Components[1:] // drop the profile list
	for _, id := range a.UnlistedEntities() {
		if id == "profile" {
			t.Fatal("profile shown by matchList is reported unlisted")
		}
	}
}
