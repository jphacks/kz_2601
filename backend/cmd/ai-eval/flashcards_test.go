package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckFlashcards(t *testing.T) {
	deck := func(c dsl.Component) *dsl.App {
		app := dsl.Initial()
		app.Entities = []dsl.Entity{{ID: "word", Name: "単語", Fields: []dsl.Field{
			{ID: "term", Name: "単語", Type: dsl.FieldString},
			{ID: "learned", Name: "覚えた", Type: dsl.FieldBoolean},
			{ID: "level", Name: "レベル", Type: dsl.FieldInteger},
		}}}
		app.Screens[0].Components = []dsl.Component{{Type: dsl.CompSection, Children: []dsl.Component{c}}}
		return &app
	}
	base := dsl.Component{Type: dsl.CompFlashcards, Entity: "word", Front: "term", Back: "term", Toggle: "learned"}
	with := func(f *dsl.Filter) dsl.Component { c := base; c.Filter = f; return c }
	for name, tc := range map[string]struct {
		app  *dsl.App
		want bool
	}{
		"eq false":     {deck(with(&dsl.Filter{Field: "learned", Op: "eq", Value: false})), true},
		"neq true":     {deck(with(&dsl.Filter{Field: "learned", Op: "neq", Value: true})), true},
		"eq true":      {deck(with(&dsl.Filter{Field: "learned", Op: "eq", Value: true})), false},
		"no filter":    {deck(base), false},
		"other field":  {deck(with(&dsl.Filter{Field: "level", Op: "eq", Value: false})), false},
		"not a deck":   {deck(dsl.Component{Type: dsl.CompList, Entity: "word"}), false},
		"string false": {deck(with(&dsl.Filter{Field: "learned", Op: "eq", Value: "false"})), false},
		"toggle missing": {deck(func() dsl.Component {
			c := with(&dsl.Filter{Field: "", Op: "eq", Value: false})
			c.Toggle = ""
			return c
		}()), false},
	} {
		miss := checkFlashcards(map[string]any{"flashcardsUnlearned": true}, tc.app)
		if got := len(miss) == 0; got != tc.want {
			t.Errorf("%s: pass=%v want %v (%v)", name, got, tc.want, miss)
		}
	}
	if miss := checkFlashcards(map[string]any{}, deck(base)); len(miss) != 0 {
		t.Errorf("no flashcards keys: %v", miss)
	}
}
