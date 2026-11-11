package extapi

import (
	"net/url"
	"slices"
	"strings"
	"testing"
)

func TestRegistry(t *testing.T) {
	for _, id := range []string{FakeFree, FakePaid, FakeOAuth} {
		p, ok := Lookup(id)
		if !ok || !p.Fake {
			t.Fatalf("%s not registered as a fake", id)
		}
	}
	if p, _ := Lookup(FakeOAuth); !p.NeedsOAuth || p.EnvName() != "FAKE_OAUTH" || p.FeatureKey() != "externalCall.fakeOauth" {
		t.Fatalf("%+v %s", p, p.EnvName())
	}
	if envName("googleCalendar") != "GOOGLE_CALENDAR" || envName("deepl") != "DEEPL" {
		t.Fatal("envName")
	}
	// the review build never lists OAuth providers or fakes (決定 6)
	for _, f := range ReviewBuildFeatures() {
		p, _ := Lookup(strings.TrimPrefix(f, "externalCall."))
		if p.NeedsOAuth || p.Fake {
			t.Fatalf("review build lists %s", f)
		}
	}
	ids := []string{}
	for _, p := range Providers() {
		ids = append(ids, p.ID)
	}
	if !slices.IsSorted(ids) {
		t.Fatalf("not sorted: %v", ids)
	}
}

func TestRegisterRefusesBadDefinitions(t *testing.T) {
	ok := Provider{ID: "okProvider", Billing: BillingFree, Ops: []OpSpec{{Name: "get"}}, New: func(Secrets) (map[string]Operation, error) { return nil, nil }}
	bad := []func(p *Provider){
		func(p *Provider) { p.ID = "Bad" },
		func(p *Provider) { p.Billing = "" },
		func(p *Provider) { p.Ops = nil },
		func(p *Provider) { p.Ops = []OpSpec{{Name: "get"}, {Name: "get"}} },
		func(p *Provider) { p.NeedsOAuth = true },
		func(p *Provider) { p.Hosts = []string{"http://example.com"} },
		func(p *Provider) { p.Hosts = []string{"https://example.com/path"} },
		func(p *Provider) { p.ID = FakeFree }, // duplicate
	}
	for i, f := range bad {
		p := ok
		f(&p)
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("case %d registered", i)
				}
			}()
			register(p)
		}()
	}
}

func TestConfigFromEnv(t *testing.T) {
	env := map[string]string{
		"EXT_FAKE_FREE_ENABLED":        "1",
		"EXT_FAKE_PAID_API_KEY":        "k",
		"EXT_COST_USD_FAKE_PAID_ECHO":  "0.25",
		"COIN_RESERVE_EXT_FAKE_PAID":   "40",
		"EXT_TIMEOUT":                  "5s",
		"EXT_OAUTH_CALLBACK_BASE":      "https://runner.example/",
		"APP_ENV":                      "production",
		"EXT_FAKE_OAUTH_CLIENT_SECRET": "s",
	}
	c, err := ConfigFromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if !c.Enabled[FakeFree] || c.Enabled[FakePaid] || c.Secrets[FakePaid].APIKey != "k" || c.Secrets[FakeOAuth].ClientSecret != "s" ||
		c.CostUSD[FakePaid+".echo"] != 0.25 || c.Reserve[FakePaid] != 40 || c.Timeout.String() != "5s" ||
		c.CallbackBase != "https://runner.example" || !c.Production || c.AppReturnURL != "tailorit://connected" {
		t.Fatalf("%+v", c)
	}
	for k, v := range map[string]string{"EXT_COST_USD_FAKE_PAID_ECHO": "-1", "COIN_RESERVE_EXT_FAKE_PAID": "x", "EXT_TIMEOUT": "2m",
		"EXT_MAX_CONCURRENT": "0", "EXT_OAUTH_CALLBACK_BASE": "http://runner.example"} {
		if _, err := ConfigFromEnv(func(key string) string {
			if key == k {
				return v
			}
			return ""
		}); err == nil {
			t.Errorf("%s=%s accepted", k, v)
		}
	}
	// default: everything off
	c, _ = ConfigFromEnv(func(string) string { return "" })
	for id, on := range c.Enabled {
		if on {
			t.Fatalf("%s on by default", id)
		}
	}
}

func TestAllowedURL(t *testing.T) {
	hosts := []string{"https://api.example.com"}
	for raw, want := range map[string]bool{
		"https://api.example.com/v1/x":      true,
		"http://api.example.com/v1/x":       false,
		"https://evil.example.com/":         false,
		"https://u:p@api.example.com/":      false,
		"https://api.example.com:8443/":     false,
		"https://api.example.com.evil.com/": false,
	} {
		u, _ := url.Parse(raw)
		if AllowedURL(hosts, u) != want {
			t.Errorf("%s: want %v", raw, want)
		}
	}
	if _, err := ReadBody(strings.NewReader(strings.Repeat("x", MaxBody+1))); err == nil {
		t.Fatal("body over 1 MB read")
	}
}
