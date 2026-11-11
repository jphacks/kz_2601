// Package extapi is the runner's door to external APIs (calendar, weather,
// translation ...; docs/decisions/external-apis.md, #1493).
//
// Only a closed list of providers and their fixed operations can be called:
// a provider is registered from its own file's init() (one file per
// provider, the child issues of #1493), and the DSL names a provider and an
// op, never a URL, a header or a body shape.
//
// Counting (Service.Call): every call is one `runs` of the creator's monthly
// allowance (quota.ExtRun; past it the overflow paid with coins as for a
// run). A paid provider also holds coins (coins op "ext", the hold
// COIN_RESERVE_EXT_<PROVIDER>), commits the actual cost × COIN_PER_USD and
// gives the rest back; the ledger row is spend_ext. A failed call gives the
// run and the coins back.
//
// Per-user OAuth tokens (connections.go) live in
// users/{uid}/connections/{provider}, sealed (Sealer: Cloud KMS in
// production) and readable by the runner only.
package extapi

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// Billing says how a provider is paid for.
type Billing string

const (
	// BillingFree: the API itself costs nothing per call (a key or OAuth
	// only). A call is one `runs`; no coins are taken for the API.
	BillingFree Billing = "free"
	// BillingPaid: each call has a cost. A call is one `runs` and the cost
	// × COIN_PER_USD in coins (hold → commit → the rest back).
	BillingPaid Billing = "paid"
)

// OpSpec is one fixed operation of a provider.
type OpSpec struct {
	// Name is the DSL's op (lowerCamel, e.g. "listEvents").
	Name string
	// Write: the op changes the person's data (create an event ...). Never
	// retried; the DSL asks for a confirmation.
	Write bool
	// Scopes are the OAuth scopes the op needs (NeedsOAuth providers).
	Scopes []string
	// Args are the argument names the op takes (the DSL's args are
	// additionalProperties:false); any other name is refused.
	Args []string
	// Check validates the argument values (currency codes, dates ...)
	// before anything is counted; an error is 400 bad_request. nil = only
	// the common checks of CheckArgs.
	Check func(args map[string]string) error
	// CacheTTL keeps the answer of the same arguments this long (cache.go):
	// a read op of a provider without OAuth, as long as the provider's terms
	// allow. 0 = never cached.
	CacheTTL time.Duration
}

// Provider is one external API. Everything here is static; the secrets come
// from env when the runner starts (Secrets).
type Provider struct {
	// ID is the DSL's provider (lowerCamel, e.g. "googleCalendar"). Env
	// names use its UPPER_SNAKE form (EnvName).
	ID      string
	Billing Billing
	// NeedsOAuth: each person connects their own account; the call uses
	// the token of the person it acts for. Not in the review build
	// (決定 6, ReviewBuildFeatures).
	NeedsOAuth bool
	// NameJa / NameEn are the service's name on the device's 「つないだサービス」
	// screen (GET /v1/me/connections providers). Required for NeedsOAuth.
	NameJa, NameEn string
	Ops            []OpSpec
	// Hosts are the only scheme://host the provider's client may reach
	// (NewHTTPClient).
	Hosts []string
	// Fake is the stg / test provider with fixed answers (決定 5). It is
	// never enabled in production.
	Fake bool
	// New builds the operations from the provider's secrets. It runs once
	// for an enabled provider when the runner starts.
	New func(Secrets) (map[string]Operation, error)
	// NewOAuth builds the OAuth client of a NeedsOAuth provider.
	NewOAuth func(Secrets) (OAuthClient, error)
}

// Op returns the provider's op name.
func (p Provider) Op(name string) (OpSpec, bool) {
	for _, o := range p.Ops {
		if o.Name == name {
			return o, true
		}
	}
	return OpSpec{}, false
}

// MaxArgLen is the longest argument value.
const MaxArgLen = 4096

// CheckArgs finds op and checks args: only the names the op takes, at most
// MaxArgLen, no URL (no URL reaches a provider from the DSL), then the op's
// own Check. Errors wrap ErrUnknown (400 bad_request). appconfig runs it
// before calling the runner; Service.Call runs it again.
func (p Provider) CheckArgs(op string, args map[string]string) (OpSpec, error) {
	spec, ok := p.Op(op)
	if !ok {
		return spec, ErrUnknown
	}
	for k, v := range args {
		if !slices.Contains(spec.Args, k) || len(v) > MaxArgLen {
			return spec, fmt.Errorf("%w: argument %q", ErrUnknown, k)
		}
		if u, err := url.Parse(v); err == nil && u.Scheme != "" && u.Host != "" {
			return spec, fmt.Errorf("%w: argument %q is a URL", ErrUnknown, k)
		}
	}
	if spec.Check != nil {
		if err := spec.Check(args); err != nil {
			return spec, fmt.Errorf("%w: %v", ErrUnknown, err)
		}
	}
	return spec, nil
}

// EnvName is the provider's part of env names: googleCalendar → GOOGLE_CALENDAR.
func (p Provider) EnvName() string { return envName(p.ID) }

// FeatureKey is the supportedFeatures value of the provider
// (externalCall.<provider>): an iOS build that does not list it gets no
// externalCall / externalList of the provider.
func (p Provider) FeatureKey() string { return "externalCall." + p.ID }

var (
	regMu    sync.RWMutex
	registry = map[string]Provider{}
	idRe     = regexp.MustCompile(`^[a-z][a-zA-Z0-9]{1,31}$`)
	opRe     = regexp.MustCompile(`^[a-z][a-zA-Z0-9]{1,47}$`)
)

// register adds p. Only this package's init() functions call it, so the list
// stays closed; a bad or duplicate definition panics at start.
func register(p Provider) {
	if err := p.check(); err != nil {
		panic("extapi: " + err.Error())
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, dup := registry[p.ID]; dup {
		panic("extapi: provider registered twice: " + p.ID)
	}
	registry[p.ID] = p
}

func (p Provider) check() error {
	switch {
	case !idRe.MatchString(p.ID):
		return fmt.Errorf("bad provider id %q", p.ID)
	case p.Billing != BillingFree && p.Billing != BillingPaid:
		return fmt.Errorf("%s: billing must be free or paid", p.ID)
	case len(p.Ops) == 0:
		return fmt.Errorf("%s: no ops", p.ID)
	case p.New == nil:
		return fmt.Errorf("%s: no New", p.ID)
	case p.NeedsOAuth && p.NewOAuth == nil:
		return fmt.Errorf("%s: needs OAuth but has no NewOAuth", p.ID)
	}
	seen := map[string]bool{}
	for _, o := range p.Ops {
		if !opRe.MatchString(o.Name) || seen[o.Name] {
			return fmt.Errorf("%s: bad or duplicate op %q", p.ID, o.Name)
		}
		if o.CacheTTL < 0 || (o.CacheTTL > 0 && (p.NeedsOAuth || o.Write)) {
			return fmt.Errorf("%s.%s: only a read op without OAuth may be cached", p.ID, o.Name)
		}
		seen[o.Name] = true
	}
	for _, h := range p.Hosts {
		if !strings.HasPrefix(h, "https://") || strings.Count(h, "/") != 2 {
			return fmt.Errorf("%s: host must be https://<host>, got %q", p.ID, h)
		}
	}
	if p.NeedsOAuth && (p.NameJa == "" || p.NameEn == "") {
		return fmt.Errorf("%s: an OAuth provider needs NameJa and NameEn", p.ID)
	}
	return nil
}

// Lookup returns the registered provider id.
func Lookup(id string) (Provider, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	p, ok := registry[id]
	return p, ok
}

// Providers returns every registered provider, sorted by id.
func Providers() []Provider {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]Provider, 0, len(registry))
	for _, p := range registry {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b Provider) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// ReviewBuildFeatures are the feature keys the App Store review build may
// list (決定 6): providers without OAuth only, fakes never.
func ReviewBuildFeatures() []string {
	var out []string
	for _, p := range Providers() {
		if !p.NeedsOAuth && !p.Fake {
			out = append(out, p.FeatureKey())
		}
	}
	return out
}

// envName turns lowerCamel into UPPER_SNAKE.
func envName(id string) string {
	var b strings.Builder
	for i, r := range id {
		if r >= 'A' && r <= 'Z' && i > 0 {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return strings.ToUpper(b.String())
}
