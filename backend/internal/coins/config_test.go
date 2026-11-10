package coins

import (
	"context"
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

// A /v1/text kind holds its own amount only when set; otherwise TEXT's.
func TestReserveTextKinds(t *testing.T) {
	c, err := ConfigFromEnv(env(map[string]string{
		"COIN_BILLING_MODE": "enforce", "COIN_RESERVE_TEXT": "2", "COIN_RESERVE_TEXT_LONG": "7", "COIN_RESERVE_TEXT_IMAGE": "0",
	}))
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{Config: c, Store: NewMemory()}
	ctx := context.Background()
	fund(t, s.Store, "u", 100, time.Now())
	for kind, want := range map[string]int64{"": 2, "task": 2, "long": 7, "image": 0} {
		h, err := s.Reserve(ctx, ReserveRequest{UID: "u", Anonymous: true, Op: OpText, Kind: kind, RequestID: "r-" + kind})
		if err != nil || h.Amount != want || h.Op != OpText {
			t.Errorf("kind %q: %+v %v, want %d", kind, h, err, want)
		}
	}
	if _, err := ConfigFromEnv(env(map[string]string{"COIN_RESERVE_TEXT_LONG": "-1"})); err == nil {
		t.Error("negative kind hold accepted")
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
