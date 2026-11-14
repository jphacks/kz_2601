package dsl

import "testing"

func TestCheckExternalUse(t *testing.T) {
	old := ExternalTerms
	t.Cleanup(func() { ExternalTerms = old })
	ExternalTerms = nil
	if errs := CheckExternalUse(nil, "x", "any", true); errs != nil {
		t.Fatalf("no hook: %v", errs)
	}
	ExternalTerms = func(p string) ExternalProviderTerms {
		return map[string]ExternalProviderTerms{"noAi": {NoAI: true}, "own": {OwnDataOnly: true}, "gone": {Ended: true}}[p]
	}
	shared := &App{Entities: []Entity{{ID: "a"}, {ID: "b", Scope: ScopePublic}}}
	for _, c := range []struct {
		app      *App
		provider string
		toAI     bool
		n        int
	}{
		{nil, "noAi", true, 1}, {nil, "noAi", false, 0},
		{shared, "own", false, 1}, {&App{Entities: []Entity{{ID: "a"}}}, "own", true, 0},
		{nil, "gone", false, 1}, {shared, "plain", true, 0},
	} {
		if errs := CheckExternalUse(c.app, "x", c.provider, c.toAI); len(errs) != c.n {
			t.Errorf("%s toAI=%v: %v", c.provider, c.toAI, errs)
		}
	}
}
