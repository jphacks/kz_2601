package main

import (
	"context"
	"log/slog"
	"slices"
	"testing"
	"time"

	"cloud.google.com/go/firestore"

	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/extapi"
)

func TestEnvOrCases(t *testing.T) {
	const key = "RUNNER_TEST_ENV_OR"
	cases := []struct {
		name, set, fallback, want string
	}{
		{"unset uses fallback", "", "8080", "8080"},
		{"set wins", "9090", "8080", "9090"},
		{"set and empty fallback", "x", "", "x"},
		{"both empty", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(key, c.set)
			if got := envOr(key, c.fallback); got != c.want {
				t.Fatalf("envOr = %q, want %q", got, c.want)
			}
		})
	}
}

// offlineClient is a Firestore client that never dials: the constructors under
// test only keep it.
func offlineClient(t *testing.T) *firestore.Client {
	t.Helper()
	t.Setenv("FIRESTORE_EMULATOR_HOST", "127.0.0.1:1")
	c, err := firestore.NewClient(context.Background(), "demo-runner-env")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestExtFromEnv(t *testing.T) {
	c := offlineClient(t)
	logger := slog.New(slog.DiscardHandler)
	sheets := map[string]string{
		"EXT_GOOGLE_SHEETS_ENABLED":       "1",
		"EXT_GOOGLE_SHEETS_CLIENT_ID":     "id",
		"EXT_GOOGLE_SHEETS_CLIENT_SECRET": "secret",
		"EXT_OAUTH_CALLBACK_BASE":         "https://example.test",
	}
	with := func(base map[string]string, kv ...string) map[string]string {
		out := map[string]string{}
		for k, v := range base {
			out[k] = v
		}
		for i := 0; i+1 < len(kv); i += 2 {
			out[kv[i]] = kv[i+1]
		}
		return out
	}
	cases := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{"nothing set: every provider off", nil, []string{}},
		{"an off-by-default provider stays off with only its key", map[string]string{"EXT_FRANKFURTER_API_KEY": "k"}, []string{}},
		{"switched on", map[string]string{"EXT_FRANKFURTER_ENABLED": "1"}, []string{extapi.Frankfurter}},
		{"EXT_DISABLED wins", map[string]string{"EXT_FRANKFURTER_ENABLED": "1", "EXT_DISABLED": "1"}, []string{}},
		{"OAuth without a sealer stays off", sheets, []string{}},
		{"OAuth with the emulator sealer", with(sheets, "FIRESTORE_EMULATOR_HOST", "127.0.0.1:1"), []string{extapi.GoogleSheets}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, err := extFromEnv(context.Background(), envMap(tc.env), nil, nil, c, logger)
			if err != nil {
				t.Fatal(err)
			}
			got := svc.EnabledProviders()
			slices.Sort(got)
			slices.Sort(tc.want)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("enabled = %v, want %v", got, tc.want)
			}
		})
	}
	if _, err := extFromEnv(context.Background(), envMap(map[string]string{"EXT_OAUTH_CALLBACK_BASE": "ftp://x"}), nil, nil, c, logger); err == nil {
		t.Error("a bad EXT_OAUTH_CALLBACK_BASE accepted")
	}
}

func TestOverflowBuyer(t *testing.T) {
	c := offlineClient(t)
	logger := slog.New(slog.DiscardHandler)
	cfg, err := coins.ConfigFromEnv(envMap(nil))
	if err != nil {
		t.Fatal(err)
	}
	s := overflowBuyer(cfg, c, []string{"prod"}, logger)
	if s.Store == nil || s.ProPeriodEnd == nil || s.Log != logger {
		t.Fatalf("service not wired: %+v", s)
	}
	if s.Config.Mode != cfg.Mode || s.Config.InitialGrant != cfg.InitialGrant || s.Config.ProMonthly != cfg.ProMonthly {
		t.Fatalf("config not passed through: %+v vs %+v", s.Config, cfg)
	}
	// an unreadable entitlement is an error, never a Pro period
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if end, err := s.ProPeriodEnd(ctx, "uid-1"); err == nil || !end.IsZero() {
		t.Fatalf("ProPeriodEnd on an unreachable store = %v, %v", end, err)
	}
}
