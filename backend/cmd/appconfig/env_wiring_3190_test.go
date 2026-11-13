package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/calls"
	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/extapi"
	"github.com/jphacks/kz_2601/backend/internal/plan"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// capLogger collects the log output so a test can look for the message.
func capLogger() (*slog.Logger, *bytes.Buffer) {
	var b bytes.Buffer
	return slog.New(slog.NewTextHandler(&b, nil)), &b
}

func clearEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		t.Setenv(k, "")
	}
}

var wiringEnvKeys = []string{
	"CALLS_ENABLED", "LIVEKIT_URL", "LIVEKIT_API_KEY", "LIVEKIT_API_SECRET", "COIN_PER_CALL_MINUTE", "CALL_BLOCK_MINUTES",
	"COIN_BILLING_MODE", "COIN_PACKS", "COIN_PER_USD", "COIN_HOLD_TTL", "APP_ENV", "RATE_LIMIT_COLLECTION",
	"EXT_RUNNER_URL", "PLAN_LIMITS_MODE", "PLAN_FREE_APPS", "PLAN_PRO_APPS",
	"APPSTORE_BUNDLE_IDS", "APPSTORE_PRODUCT_IDS", "APPSTORE_ENVIRONMENTS",
}

// Bad values stop the start (os.Exit(1)); each case re-runs this test binary
// with WIRING_EXIT_CASE set and checks the exit code and the log line.
func TestWiringExitChild_3190(t *testing.T) {
	c := os.Getenv("WIRING_EXIT_CASE")
	if c == "" {
		t.Skip("child process only")
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	st := store.NewMemory()
	switch c {
	case "calls":
		newCalls(st, nil, log)
	case "coins":
		newCoins(st, newEntitlements(st, log), log)
	case "ext":
		newExt(context.Background(), log)
	case "plan":
		planLimits(log)
	case "entitlements":
		newEntitlements(st, log)
	}
}

func TestWiringBadEnvExits_3190(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string // log message
	}{
		{"calls", map[string]string{"CALLS_ENABLED": "maybe"}, "calls config"},
		{"calls", map[string]string{"CALLS_ENABLED": "true"}, "calls config"}, // keys missing
		{"coins", map[string]string{"COIN_BILLING_MODE": "bogus"}, "coin config"},
		{"coins", map[string]string{"COIN_PER_USD": "-1"}, "coin config"},
		{"coins", map[string]string{"COIN_PACKS": "a:1"}, "coin config"},
		{"ext", map[string]string{"EXT_RUNNER_URL": "http://example.com"}, "EXT_RUNNER_URL must be"},
		{"ext", map[string]string{"EXT_RUNNER_URL": "ftp://runner"}, "EXT_RUNNER_URL must be"},
		{"plan", map[string]string{"PLAN_LIMITS_MODE": "bogus"}, "plan limits"},
		{"plan", map[string]string{"PLAN_FREE_APPS": "-3"}, "plan limits"},
		{"entitlements", map[string]string{"APPSTORE_ENVIRONMENTS": "Xcode"}, "appstore config"},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/"+tc.want, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestWiringExitChild_3190$")
			env := os.Environ()
			for _, k := range wiringEnvKeys {
				env = append(env, k+"=")
			}
			env = append(env, "WIRING_EXIT_CASE="+tc.name)
			for k, v := range tc.env {
				env = append(env, k+"="+v)
			}
			cmd.Env = env
			out, err := cmd.CombinedOutput()
			ee, ok := err.(*exec.ExitError)
			if !ok || ee.ExitCode() != 1 {
				t.Fatalf("want exit 1, got %v\n%s", err, out)
			}
			if !strings.Contains(string(out), tc.want) {
				t.Fatalf("log lacks %q:\n%s", tc.want, out)
			}
		})
	}
}

func TestNewCallsEnv_3190(t *testing.T) {
	clearEnv(t, wiringEnvKeys...)
	st := store.NewMemory()

	log, buf := capLogger()
	s := newCalls(st, nil, log)
	if s.Config.Enabled || s.Rooms != nil || s.Store == nil {
		t.Fatalf("default = %+v rooms=%v", s.Config, s.Rooms)
	}
	if !strings.Contains(buf.String(), "enabled=false") || !strings.Contains(buf.String(), "charged=false") {
		t.Fatalf("log: %s", buf)
	}

	t.Setenv("CALLS_ENABLED", "true")
	t.Setenv("LIVEKIT_URL", "wss://lk.example.com")
	t.Setenv("LIVEKIT_API_KEY", "test-key")
	t.Setenv("LIVEKIT_API_SECRET", "test-secret")
	t.Setenv("CALL_BLOCK_MINUTES", "7")
	t.Setenv("COIN_PER_CALL_MINUTE", "3")
	log, buf = capLogger()
	s = newCalls(st, nil, log)
	if !s.Config.Enabled || s.Rooms == nil || s.Config.BlockMinutes != 7 || s.Config.PerMinute != 3 {
		t.Fatalf("enabled = %+v rooms=%v", s.Config, s.Rooms)
	}
	if _, ok := s.Store.(*calls.Memory); !ok {
		t.Fatalf("memory store expected, got %T", s.Store)
	}
	if !strings.Contains(buf.String(), "enabled=true") || !strings.Contains(buf.String(), "charged=true") || !strings.Contains(buf.String(), "blockMinutes=7") {
		t.Fatalf("log: %s", buf)
	}
}

func TestNewCoinsEnv_3190(t *testing.T) {
	clearEnv(t, wiringEnvKeys...)
	st := store.NewMemory()
	ents := newEntitlements(st, quietLogger())

	log, buf := capLogger()
	cs := newCoins(st, ents, log)
	if cs.Config.Mode != coins.ModeOff || cs.Counters != nil || cs.Env != "" || len(cs.Config.Packs) != 0 {
		t.Fatalf("default = %+v", cs.Config)
	}
	if !strings.Contains(buf.String(), "mode=off") {
		t.Fatalf("log: %s", buf)
	}

	t.Setenv("COIN_BILLING_MODE", "LOG")
	t.Setenv("APP_ENV", "stg")
	t.Setenv("COIN_PACKS", "jp.test.pack.small=100")
	log, buf = capLogger()
	cs = newCoins(st, ents, log)
	if cs.Config.Mode != coins.ModeLog || cs.Env != "stg" || len(cs.Config.Packs) != 1 {
		t.Fatalf("set = %+v env=%q", cs.Config, cs.Env)
	}
	if got := cs.Verifier.ProductIDs; len(got) != 1 || got[0] != "jp.test.pack.small" {
		t.Fatalf("coin verifier products = %v", got)
	}
	if !strings.Contains(buf.String(), "mode=log") || !strings.Contains(buf.String(), "packs=1") {
		t.Fatalf("log: %s", buf)
	}
}

func TestNewExtEnv_3190(t *testing.T) {
	clearEnv(t, wiringEnvKeys...)
	log, buf := capLogger()
	if r := newExt(context.Background(), log); r != nil {
		t.Fatalf("unset = %T", r)
	}
	if !strings.Contains(buf.String(), "not set") {
		t.Fatalf("log: %s", buf)
	}
	// 127.0.0.1 is not "localhost": https is required there, so it exits (see the exit cases).
	t.Setenv("EXT_RUNNER_URL", "http://localhost:9/")
	log, buf = capLogger()
	c, ok := newExt(context.Background(), log).(*extapi.Client)
	if !ok || c.BaseURL != "http://localhost:9" || c.HTTP.Timeout != 20*time.Second {
		t.Fatalf("client = %+v", c)
	}
	if !strings.Contains(buf.String(), "ext: runner") {
		t.Fatalf("log: %s", buf)
	}
}

func TestPlanLimitsEnv_3190(t *testing.T) {
	clearEnv(t, wiringEnvKeys...)
	log, buf := capLogger()
	d := planLimits(log)
	if d.Mode != plan.ModeEnforce || d.Free.Apps != plan.Default.Free.Apps || d.Pro.Apps != plan.Default.Pro.Apps {
		t.Fatalf("default = %+v", d)
	}
	if !strings.Contains(buf.String(), "mode=enforce") {
		t.Fatalf("log: %s", buf)
	}

	t.Setenv("PLAN_LIMITS_MODE", "log")
	t.Setenv("PLAN_FREE_APPS", "2")
	t.Setenv("PLAN_PRO_APPS", "50")
	got := planLimits(quietLogger())
	if got.Mode != plan.ModeLog || got.Free.Apps != 2 || got.Pro.Apps != 50 {
		t.Fatalf("set = %+v", got)
	}
	// the other limits keep their defaults
	if got.Free.RecordsPerApp != plan.Default.Free.RecordsPerApp {
		t.Fatalf("records = %d", got.Free.RecordsPerApp)
	}
}

func TestNewEntitlementsEnv_3190(t *testing.T) {
	clearEnv(t, wiringEnvKeys...)
	t.Setenv("APPSTORE_BUNDLE_IDS", " jp.test.a , ,jp.test.b")
	t.Setenv("APPSTORE_PRODUCT_IDS", "jp.test.a.pro")
	t.Setenv("APPSTORE_ENVIRONMENTS", "Production")
	log, buf := capLogger()
	e := newEntitlements(store.NewMemory(), log)
	p := e.Verifier.Policy
	if len(p.BundleIDs) != 2 || p.BundleIDs[0] != "jp.test.a" || p.BundleIDs[1] != "jp.test.b" ||
		len(p.ProductIDs) != 1 || len(p.Environments) != 1 || p.Environments[0] != "Production" {
		t.Fatalf("policy = %+v", p)
	}
	if e.Deleted == nil || !strings.Contains(buf.String(), "jp.test.a.pro") {
		t.Fatalf("deleted=%v log: %s", e.Deleted != nil, buf)
	}
}
