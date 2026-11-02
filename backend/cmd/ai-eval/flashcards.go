package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Flashcards (#264) expect key, checked by checkFlashcards:
//   - flashcardsUnlearned (bool): some flashcards saves learned / not yet into
//     a boolean toggle field and its filter drops the learned cards
//     (toggle eq false or neq true).
//
// Cases: internal/ai/eval/flashcards_cases.json (-cases) and
// flashcards_chat_cases.json (-chat -cases).

// unlearnedDeck reports whether c drops learned cards from the deck.
func unlearnedDeck(c dsl.Component, a *dsl.App) bool {
	if c.Type != dsl.CompFlashcards || c.Toggle == "" || c.Filter == nil || c.Filter.Field != c.Toggle {
		return false
	}
	e := a.EntityByID(c.Entity)
	if e == nil {
		return false
	}
	if f := e.FieldByID(c.Toggle); f == nil || f.Type != dsl.FieldBoolean {
		return false
	}
	b, ok := c.Filter.Value.(bool)
	return ok && ((c.Filter.Op == "eq" && !b) || (c.Filter.Op == "neq" && b))
}

func checkFlashcards(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["flashcardsUnlearned"].(bool)
	if !ok {
		return nil
	}
	got, decks := false, 0
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if x.Type == dsl.CompFlashcards {
					decks++
					got = got || unlearnedDeck(x, a)
				}
			}
		}
	}
	if got != want {
		return []string{fmt.Sprintf("flashcardsUnlearned=%v (flashcards=%d)", got, decks)}
	}
	return nil
}
