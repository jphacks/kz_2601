package coins

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestConfigFromEnvDefaults(t *testing.T) {
	c, err := ConfigFromEnv(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != ModeOff || c.PerUSD != 0 || c.MinPerCall != 1 || c.HoldTTL != DefaultHoldTTL || c.InitialGrant != 0 || c.ProMonthly != 0 || len(c.Packs) != 0 {
		t.Fatalf("defaults %+v", c)
	}
	for _, op := range Ops {
		if v, ok := c.Reserve[op]; !ok || v != 0 {
			t.Errorf("reserve %s = %d %v", op, v, ok)
		}
	}
	if c.Price(1) != 0 {
		t.Fatalf("price without COIN_PER_USD = %d", c.Price(1))
	}
}

func TestConfigFromEnv(t *testing.T) {
	c, err := ConfigFromEnv(env(map[string]string{
		"COIN_BILLING_MODE": "Enforce", "COIN_PER_USD": "12.5", "COIN_MIN_PER_CALL": "2",
		"COIN_RESERVE_GENERATE": "9", "COIN_RESERVE_TEXT": "1", "COIN_ESTIMATE_CHAT": "3",
		"COIN_PACKS": "a.small=10, b.large=100", "COIN_INITIAL_GRANT": "4", "COIN_PRO_MONTHLY": "6",
		"COIN_HOLD_TTL": "15m", "ALLOWANCE_PRO_RUNS": "8",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != ModeEnforce || c.PerUSD != 12.5 || c.MinPerCall != 2 || c.Reserve[OpGenerate] != 9 || c.Reserve[OpText] != 1 ||
		c.Estimates[OpChat] != 3 || c.InitialGrant != 4 || c.ProMonthly != 6 || c.HoldTTL != 15*time.Minute || c.Allowance["runs"] != 8 {
		t.Fatalf("%+v", c)
	}
	if n, ok := c.PackCoins("b.large"); !ok || n != 100 || strings.Join(c.PackIDs(), ",") != "a.small,b.large" {
		t.Fatalf("packs %+v", c.Packs)
	}
	// ceil(0.01 × 12.5) = 1 → min 2; ceil(1 × 12.5) = 13
	if c.Price(0.01) != 2 || c.Price(1) != 13 {
		t.Fatalf("price %d %d", c.Price(0.01), c.Price(1))
	}
}

func TestConfigFromEnvRejects(t *testing.T) {
	for _, m := range []map[string]string{
		{"COIN_BILLING_MODE": "on"},
		{"COIN_PER_USD": "-1"},
		{"COIN_PER_USD": "abc"},
		{"COIN_RESERVE_CHAT": "-2"},
		{"COIN_INITIAL_GRANT": "1.5"},
		{"COIN_PACKS": "a=0"},
		{"COIN_PACKS": "a=1,a=2"},
		{"COIN_PACKS": "a"},
		{"COIN_HOLD_TTL": "10s"},
	} {
		if _, err := ConfigFromEnv(env(m)); err == nil {
			t.Errorf("%v accepted", m)
		}
	}
}

func TestPublicHidesPerUSD(t *testing.T) {
	c := testConfig(ModeLog)
	c.PerUSD = 123.456
	b, _ := json.Marshal(c.Public())
	if strings.Contains(string(b), "123.456") || strings.Contains(strings.ToLower(string(b)), "usd") {
		t.Fatalf("public config leaks the rate: %s", b)
	}
	var got PublicConfig
	_ = json.Unmarshal(b, &got)
	if got.Mode != ModeLog || len(got.Packs) != 1 || got.Estimates[OpGenerate] != 3 || len(got.Estimates) != 1 {
		t.Fatalf("%s", b)
	}
	if b, _ := json.Marshal(Config{}.Public()); !strings.Contains(string(b), `"mode":"off"`) || !strings.Contains(string(b), `"packs":[]`) {
		t.Fatalf("zero %s", b)
	}
}

func TestWriteError(t *testing.T) {
	cases := []struct {
		err  error
		code string
	}{
		{&InsufficientError{Required: 5, Balance: 2}, `"code":"coins_insufficient"`},
		{ErrAllowanceExhausted, `"code":"allowance_exhausted"`},
		{&OverflowCapError{Cap: 100, Spent: 100}, `"code":"overflow_cap_reached"`},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		if !WriteError(w, c.err) || w.Code != http.StatusPaymentRequired || !strings.Contains(w.Body.String(), c.code) {
			t.Errorf("%v: %d %s", c.err, w.Code, w.Body)
		}
	}
	w := httptest.NewRecorder()
	WriteError(w, &InsufficientError{Required: 5, Balance: 2})
	if !strings.Contains(w.Body.String(), `"payer":"self"`) || !strings.Contains(w.Body.String(), `"required":5`) {
		t.Errorf("details %s", w.Body)
	}
	if WriteError(httptest.NewRecorder(), ErrNotHeld) {
		t.Error("other errors are not written")
	}
}

// enforce with an op's hold left at 0 would charge that op nothing:
// CheckReserves names it. log / off do not need the reserves.
func TestCheckReserves(t *testing.T) {
	all := map[string]string{"COIN_BILLING_MODE": "enforce",
		"COIN_RESERVE_GENERATE": "1", "COIN_RESERVE_EDIT": "1", "COIN_RESERVE_CHAT": "1", "COIN_RESERVE_TEXT": "1"}
	check := func(m map[string]string) error {
		t.Helper()
		c, err := ConfigFromEnv(env(m))
		if err != nil {
			t.Fatalf("%v: %v", m, err)
		}
		return c.CheckReserves()
	}
	if err := check(all); err != nil {
		t.Fatalf("all set: %v", err)
	}
	for _, op := range Ops {
		key := "COIN_RESERVE_" + strings.ToUpper(op)
		for _, v := range []string{"", "0"} {
			m := map[string]string{}
			for k, x := range all {
				m[k] = x
			}
			m[key] = v
			if err := check(m); err == nil || !strings.Contains(err.Error(), key) {
				t.Errorf("%s=%q under enforce: %v", key, v, err)
			}
		}
	}
	for _, mode := range []string{"", "off", "log"} {
		if err := check(map[string]string{"COIN_BILLING_MODE": mode}); err != nil {
			t.Errorf("mode %q without reserves: %v", mode, err)
		}
	}
}
