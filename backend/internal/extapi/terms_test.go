package extapi

import (
	"errors"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// withProvider changes a registered provider for one test (fakes only).
func withProvider(t *testing.T, id string, change func(*Provider)) {
	t.Helper()
	regMu.Lock()
	old := registry[id]
	p := old
	change(&p)
	if err := p.check(); err != nil {
		regMu.Unlock()
		t.Fatal(err)
	}
	registry[id] = p
	regMu.Unlock()
	t.Cleanup(func() {
		regMu.Lock()
		registry[id] = old
		regMu.Unlock()
	})
}

// NoCharge: not Pro only (the creator has no Pro), no `runs`, no coins —
// also for an answer from the cache.
func TestNoChargeIsFreeForEveryone(t *testing.T) {
	withProvider(t, FakeFree, func(p *Provider) {
		p.Terms = Terms{NoCharge: true}
		p.Ops = []OpSpec{{Name: "listItems", Args: []string{"q", "fail"}, CacheTTL: time.Minute}}
	})
	e := newEnv(t, opts{pro: proSet{}})
	e.fund(user, 10)
	for i := 0; i < 2; i++ { // the second is the cache
		res, err := e.call(e.req(FakeFree, "listItems", map[string]string{"q": "x"}))
		if err != nil || len(res.Output.Rows) != 2 || res.Charge.Coins != 0 {
			t.Fatalf("call %d: %+v %v", i, res, err)
		}
	}
	if e.runs() != 0 || e.wallet(user).Available() != 10 {
		t.Fatalf("runs=%d wallet=%+v", e.runs(), e.wallet(user))
	}
	// without NoCharge the same creator is refused (決定 1)
	e2 := newEnv(t, opts{pro: proSet{}})
	if _, err := e2.call(e2.req(FakePaid, "echo", map[string]string{"text": "x"})); !errors.Is(err, ErrProRequired) {
		t.Fatalf("paid without Pro: %v", err)
	}
}

// NoCharge still counts the call limits (a runaway brake).
func TestNoChargeKeepsLimits(t *testing.T) {
	withProvider(t, FakeFree, func(p *Provider) { p.Terms = Terms{NoCharge: true} })
	e := newEnv(t, opts{pro: proSet{}, cfg: func(c *Config) {
		c.Limits = map[string]Limits{FakeFree: {UserMinute: 2}}
	}})
	for i := 0; i < 2; i++ {
		if _, err := e.call(e.req(FakeFree, "listItems", nil)); err != nil {
			t.Fatal(err)
		}
	}
	var le *LimitError
	if _, err := e.call(e.req(FakeFree, "listItems", nil)); !errors.As(err, &le) || le.Scope != LimitUserMinute {
		t.Fatalf("third call in a minute: %v", err)
	}
	e.c.t = e.c.t.Add(time.Minute)
	if _, err := e.call(e.req(FakeFree, "listItems", nil)); err != nil {
		t.Fatalf("next minute: %v", err)
	}
}

// ShowOwnDataOnly: a server rule (the creator's token for everyone) is
// refused; the person's own call works.
func TestShowOwnDataOnlyRefusesServerRules(t *testing.T) {
	withProvider(t, FakeOAuth, func(p *Provider) { p.Terms = Terms{ShowOwnDataOnly: true} })
	e := newEnv(t, opts{})
	e.connect(user)
	if _, err := e.call(e.req(FakeOAuth, "listEvents", nil)); err != nil {
		t.Fatal(err)
	}
	r := e.req(FakeOAuth, "listEvents", nil)
	r.UserUID = ""
	if _, err := e.call(r); !errors.Is(err, ErrOwnDataOnly) || Reason(err) != ReasonBadRequest {
		t.Fatalf("server rule: %v", err)
	}
}

// WithTerms puts the credit and noAI on the answer; an op's own credit wins.
func TestWithTerms(t *testing.T) {
	p := Provider{Terms: Terms{NoAI: true, Attribution: "Powered by X"}}
	if out := p.WithTerms(Output{}); out.Attribution != "Powered by X" || !out.NoAI {
		t.Fatalf("%+v", out)
	}
	if out := p.WithTerms(Output{Attribution: "own"}); out.Attribution != "own" {
		t.Fatalf("%+v", out)
	}
	if out := (Provider{}).WithTerms(Output{}); out.Attribution != "" || out.NoAI {
		t.Fatalf("%+v", out)
	}
}

func TestTermsCheck(t *testing.T) {
	ok := Provider{ID: "okProvider", Billing: BillingFree, Ops: []OpSpec{{Name: "get"}}, New: func(Secrets) (map[string]Operation, error) { return nil, nil }}
	for name, change := range map[string]func(*Provider){
		"paid NoCharge":      func(p *Provider) { p.Billing = BillingPaid; p.Terms.NoCharge = true },
		"own data, no OAuth": func(p *Provider) { p.Terms.ShowOwnDataOnly = true },
		"two lines":          func(p *Provider) { p.Terms.Attribution = "a\nb" },
		"untrimmed":          func(p *Provider) { p.Terms.Attribution = " a" },
		"too long":           func(p *Provider) { p.Terms.Attribution = string(make([]rune, MaxAttribution+1)) },
	} {
		p := ok
		change(&p)
		if p.check() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	p := ok
	p.Terms = Terms{NoCharge: true, NoAI: true, Attribution: "Data by X"}
	if err := p.check(); err != nil {
		t.Fatal(err)
	}
}

// The providers whose terms ask for it (2026-11-15 owner).
func TestProviderTerms(t *testing.T) {
	for id, want := range map[string]Terms{
		Spotify: {NoCharge: true, NoAI: true, ShowOwnDataOnly: true},
		Zaim:    {ShowOwnDataOnly: true},
	} {
		p, ok := Lookup(id)
		got := p.Terms
		got.Attribution = ""
		if !ok || got != want {
			t.Errorf("%s: %+v", id, p.Terms)
		}
	}
	for _, id := range []string{Spotify, EStat, Wikipedia, Connpass, OpenWeather, Podcasts, GooglePlaces, ODPT, GTFSRealtime} {
		if p, _ := Lookup(id); p.Terms.Attribution == "" {
			t.Errorf("%s: no attribution", id)
		}
	}
	if p, _ := Lookup(Connpass); p.Terms.NoCharge || p.UserMinute <= 0 || p.GlobalDaily <= 0 {
		t.Errorf("connpass (paid plan, capped): %+v", p)
	}
}

// SunsetAt: from the shutdown on the provider is off (ext_unavailable), not
// in the review build's features and refused by the DSL rule.
func TestSunset(t *testing.T) {
	end := time.Date(2026, 11, 11, 12, 30, 0, 0, time.UTC)
	withProvider(t, FakeFree, func(p *Provider) { p.SunsetAt = end })
	e := newEnv(t, opts{})
	e.c.t = end.Add(-time.Second)
	if _, err := e.call(e.req(FakeFree, "listItems", nil)); err != nil || !e.svc.Enabled(FakeFree) {
		t.Fatalf("before: %v", err)
	}
	e.c.t = end
	if _, err := e.call(e.req(FakeFree, "listItems", nil)); !errors.Is(err, ErrUnavailable) || e.svc.Enabled(FakeFree) {
		t.Fatalf("after: %v", err)
	}
	for _, id := range e.svc.EnabledProviders() {
		if id == FakeFree {
			t.Fatal("listed as enabled after the sunset")
		}
	}

	old := sunsetNow
	t.Cleanup(func() { sunsetNow = old })
	sunsetNow = func() time.Time { return end }
	if errs := dsl.CheckExternalUse(nil, "x", FakeFree, false); len(errs) != 1 {
		t.Fatalf("dsl rule after the sunset: %v", errs)
	}
	sunsetNow = func() time.Time { return end.Add(-time.Hour) }
	if errs := dsl.CheckExternalUse(nil, "x", FakeFree, false); len(errs) != 0 {
		t.Fatalf("dsl rule before: %v", errs)
	}
}

// Zaim ends 2026-12-07 00:00 JST (https://content.zaim.net/notices/show/2280).
func TestZaimSunset(t *testing.T) {
	p, _ := Lookup(Zaim)
	jst := time.FixedZone("JST", 9*3600)
	if p.Sunset(time.Date(2026, 12, 6, 23, 59, 59, 0, jst)) || !p.Sunset(time.Date(2026, 12, 7, 0, 0, 0, 0, jst)) {
		t.Fatalf("zaim sunset %v", p.SunsetAt)
	}
	old := sunsetNow
	t.Cleanup(func() { sunsetNow = old })
	sunsetNow = func() time.Time { return time.Date(2026, 12, 8, 0, 0, 0, 0, jst) }
	if errs := dsl.CheckExternalUse(nil, "x", Zaim, false); len(errs) == 0 {
		t.Fatal("zaim offered after its end")
	}
}

// The DSL rule: NoAI providers never feed AI; ShowOwnDataOnly ones never
// sit in an app with a shared / public entity.
func TestDSLExternalUse(t *testing.T) {
	shared := &dsl.App{Entities: []dsl.Entity{{ID: "note", Scope: dsl.ScopeShared}}}
	private := &dsl.App{Entities: []dsl.Entity{{ID: "note"}}}
	if len(dsl.CheckExternalUse(private, "x", Spotify, true)) != 1 {
		t.Fatal("spotify rows to AI accepted")
	}
	if len(dsl.CheckExternalUse(shared, "x", Spotify, false)) != 1 {
		t.Fatal("spotify in a shared app accepted")
	}
	if errs := dsl.CheckExternalUse(private, "x", Spotify, false); len(errs) != 0 {
		t.Fatal(errs)
	}
	if errs := dsl.CheckExternalUse(shared, "x", Wikipedia, true); len(errs) != 0 {
		t.Fatal(errs)
	}
}
