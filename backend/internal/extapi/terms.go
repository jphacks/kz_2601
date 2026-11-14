package extapi

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Terms are what a provider's terms of use ask of Tailor It, kept in code so
// a provider can ship under its terms instead of being left out
// (docs/backend/features/external-api-terms-modes.md, 2026-11-15 owner).
type Terms struct {
	// NoCharge: the terms forbid charging people for the provider's
	// features, directly or indirectly (Atlassian 8.3, non-commercial
	// APIs). A call is never Pro-gated and counts no `runs` and no coins:
	// it works on the free plan. The call limits (limits.go) still apply.
	// A paid provider cannot be NoCharge.
	NoCharge bool
	// NoAI: the provider's results must not go to an LLM (aiButton
	// inputFeeds, aiChat ...). The answer carries noAI so the device leaves
	// the rows out of AI input, and DSL validation refuses the wiring
	// (dsl.ExternalTerms).
	NoAI bool
	// ShowOwnDataOnly: the OAuth data may be shown only to the person who
	// connected. No server rule (it would act with the creator's token for
	// others) and no app with a shared / public entity. OAuth providers only.
	ShowOwnDataOnly bool
	// Attribution is the credit the terms ask the app to show with the
	// results (one line). It is sent with every answer (Output.Attribution).
	Attribution string
}

// MaxAttribution is the longest Terms.Attribution in runes.
const MaxAttribution = 300

// ErrOwnDataOnly is a call the provider's terms do not allow here: a
// ShowOwnDataOnly provider from a server rule or a shared app (400 / 403).
var ErrOwnDataOnly = fmt.Errorf("%w: this provider shows a person's data only to that person", ErrUnknown)

func (t Terms) check(p Provider) error {
	switch {
	case t.NoCharge && p.Billing == BillingPaid:
		return fmt.Errorf("%s: a paid provider cannot be NoCharge", p.ID)
	case t.ShowOwnDataOnly && !p.NeedsOAuth:
		return fmt.Errorf("%s: ShowOwnDataOnly is for OAuth providers", p.ID)
	case utf8.RuneCountInString(t.Attribution) > MaxAttribution:
		return fmt.Errorf("%s: attribution over %d characters", p.ID, MaxAttribution)
	case t.Attribution != strings.TrimSpace(t.Attribution) || strings.IndexFunc(t.Attribution, unicode.IsControl) >= 0:
		return errors.New(p.ID + ": attribution must be one trimmed line")
	}
	return nil
}

// WithTerms puts the provider's terms on an answer for the device: the
// attribution (an op's own one wins) and noAI.
func (p Provider) WithTerms(out Output) Output {
	if out.Attribution == "" {
		out.Attribution = p.Terms.Attribution
	}
	out.NoAI = out.NoAI || p.Terms.NoAI
	return out
}

// termsOf is dsl.ExternalTerms: unknown providers have no limits here (the
// registry refuses them elsewhere).
func termsOf(provider string) dsl.ExternalProviderTerms {
	p, ok := Lookup(provider)
	if !ok {
		return dsl.ExternalProviderTerms{}
	}
	return dsl.ExternalProviderTerms{NoAI: p.Terms.NoAI, OwnDataOnly: p.Terms.ShowOwnDataOnly, Ended: p.Sunset(sunsetNow())}
}

// sunsetNow is the clock of the sunset checks outside a Service (tests swap it).
var sunsetNow = time.Now

func init() { dsl.ExternalTerms = termsOf }

// SetTermsForTest replaces a fake provider's Terms and returns the undo
// (other packages' tests: appconfig). Real providers cannot be changed.
func SetTermsForTest(id string, t Terms) (undo func()) {
	regMu.Lock()
	defer regMu.Unlock()
	p, ok := registry[id]
	if !ok || !p.Fake {
		panic("extapi: SetTermsForTest on a non-fake provider: " + id)
	}
	old := p.Terms
	p.Terms = t
	registry[id] = p
	return func() {
		regMu.Lock()
		defer regMu.Unlock()
		p := registry[id]
		p.Terms = old
		registry[id] = p
	}
}
