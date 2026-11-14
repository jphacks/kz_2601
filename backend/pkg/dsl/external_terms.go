package dsl

import "fmt"

// ExternalProviderTerms are what an external API provider's terms allow
// (docs/backend/features/external-api-terms-modes.md).
type ExternalProviderTerms struct {
	// NoAI: the provider's rows must not reach an LLM.
	NoAI bool
	// OwnDataOnly: the provider's OAuth data may be shown only to the
	// person who connected (no shared / public entity in the app).
	OwnDataOnly bool
	// Ended: the provider's API has shut down (SunsetAt passed).
	Ended bool
}

// ExternalTerms tells validation a provider's terms (internal/extapi sets
// it from the registry; nil = no provider has limits).
var ExternalTerms func(provider string) ExternalProviderTerms

// CheckExternalUse is the rule an externalCall / externalList of provider
// must pass (#1643: the components call it when they land; nothing in the
// DSL reaches a provider yet). toAI: its rows feed an AI component
// (aiButton inputFeeds, aiChat context ...). where prefixes each message.
func CheckExternalUse(a *App, where, provider string, toAI bool) []string {
	if ExternalTerms == nil {
		return nil
	}
	t := ExternalTerms(provider)
	var errs []string
	if t.Ended {
		errs = append(errs, fmt.Sprintf("%s: %s has shut down its API; use another provider", where, provider))
	}
	if t.NoAI && toAI {
		errs = append(errs, fmt.Sprintf("%s: the terms of %s forbid sending its results to AI (aiButton inputFeeds / aiChat)", where, provider))
	}
	if t.OwnDataOnly && a != nil {
		for i := range a.Entities {
			if a.Entities[i].IsShared() {
				errs = append(errs, fmt.Sprintf("%s: the terms of %s show a person's data only to that person; it cannot be used in an app with a shared or public entity (%q)", where, provider, a.Entities[i].ID))
				break
			}
		}
	}
	return errs
}
