package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

// #2133: no per-OS variable = the same config as before.
func TestConfigFromEnvNoPlatformOverride(t *testing.T) {
	cfg, err := ConfigFromEnv(envOf(map[string]string{"RATE_LIMIT_USER_DAILY": "40"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PlatformLimits != nil {
		t.Fatalf("PlatformLimits = %v, want nil", cfg.PlatformLimits)
	}
	for _, p := range append([]string{""}, httpx.Platforms...) {
		for _, tier := range []Tier{TierAnonymous, TierSignedIn, TierPro} {
			if got, want := cfg.PlatformDailyLimit(tier, p), cfg.DailyLimit(tier); got != want {
				t.Errorf("%q %s: %d, want %d", p, tier, got, want)
			}
		}
	}
	if cfg.PlatformDailyLimit(TierSignedIn, httpx.PlatformWindows) != 40 {
		t.Fatal("the base RATE_LIMIT_USER_DAILY must apply to every OS")
	}
}

func TestConfigFromEnvPlatformOverride(t *testing.T) {
	env := map[string]string{
		"RATE_LIMIT_USER_DAILY_WINDOWS": "20",
		"RATE_LIMIT_PRO_DAILY_WINDOWS":  "100",
		"RATE_LIMIT_ANON_DAILY_OTHER":   "0",
		"RATE_LIMIT_USER_DAILY_linux":   "1", // only the upper-case name is read
	}
	cfg, err := ConfigFromEnv(envOf(env))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]map[Tier]int{
		"windows": {TierSignedIn: 20, TierPro: 100},
		"other":   {TierAnonymous: 0},
	}
	if !reflect.DeepEqual(cfg.PlatformLimits, want) {
		t.Fatalf("PlatformLimits = %v", cfg.PlatformLimits)
	}
	if cfg.Limits != DefaultLimits {
		t.Fatalf("base limits changed: %+v", cfg.Limits)
	}
	for _, tc := range []struct {
		tier     Tier
		platform string
		want     int
	}{
		{TierSignedIn, "windows", 20},
		{TierPro, "windows", 100},
		{TierAnonymous, "windows", DefaultLimits.Anonymous},
		{TierAnonymous, "other", 0},
		{TierSignedIn, "other", DefaultLimits.SignedIn},
		{TierSignedIn, "linux", DefaultLimits.SignedIn},
		{TierSignedIn, "ios", DefaultLimits.SignedIn},
		{TierSignedIn, "", DefaultLimits.SignedIn},
	} {
		if got := cfg.PlatformDailyLimit(tc.tier, tc.platform); got != tc.want {
			t.Errorf("%s on %q = %d, want %d", tc.tier, tc.platform, got, tc.want)
		}
	}

	env["RATE_LIMIT_ANON_DAILY_MACOS"] = "-1"
	if _, err := ConfigFromEnv(envOf(env)); err == nil || !strings.Contains(err.Error(), "RATE_LIMIT_ANON_DAILY_MACOS") {
		t.Fatalf("want an error naming the variable, got %v", err)
	}
}

// Without an override every OS gets exactly the decisions of Allow.
func TestAllowPlatformWithoutOverrideIsAllow(t *testing.T) {
	cfg, err := ConfigFromEnv(envOf(map[string]string{"RATE_LIMIT_USER_DAILY": "5", "RATE_LIMIT_ANON_DAILY": "3"}))
	if err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.Date(2026, 10, 25, 12, 0, 0, 0, JST) }
	cfg.Now = now
	for _, p := range httpx.Platforms {
		base := New(cfg, NewMemory(), nil)
		onOS := New(cfg, NewMemory(), nil)
		for i := range 4 {
			for _, id := range []auth.Identity{signed, anon} {
				want, err1 := base.Allow(context.Background(), id, "10.0.0.1", 2)
				got, err2 := onOS.AllowPlatform(context.Background(), id, "10.0.0.1", p, 2)
				if err1 != nil || err2 != nil {
					t.Fatal(err1, err2)
				}
				want.charge, got.charge = charge{}, charge{}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("%s call %d %s: %+v, want %+v", p, i, id.UID, got, want)
				}
			}
		}
	}
}

// The middleware picks the limit from the normalised X-Client-Platform.
func TestMiddlewarePlatformLimit(t *testing.T) {
	l, _ := newTestLimiter(Limits{SignedIn: 10})
	l.cfg.PlatformLimits = map[string]map[Tier]int{"windows": {TierSignedIn: 4}, "other": {TierSignedIn: 2}}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := l.Middleware(auth.DevVerifier{})(ok)
	call := func(uid, platform string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/v1/generate", strings.NewReader(`{"mode":"edit"}`))
		r.Header.Set("Authorization", "Bearer "+uid)
		if platform != "" {
			r.Header.Set(httpx.ClientPlatformHeader, platform)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct {
		uid, platform, limit string
	}{
		{"u-win", "Windows", "4"},
		{"u-ios", "ios", "10"},
		{"u-none", "", "2"},
		{"u-odd", "haiku-os", "2"},
	} {
		w := call(tc.uid, tc.platform)
		if w.Code != http.StatusOK || w.Header().Get("X-RateLimit-Limit") != tc.limit {
			t.Fatalf("%s: %d limit=%q, want %s", tc.platform, w.Code, w.Header().Get("X-RateLimit-Limit"), tc.limit)
		}
	}
	// windows: 2 + 2 = 4, the third edit is over the OS limit
	call("u-win", "windows")
	if w := call("u-win", "windows"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("windows over limit: %d", w.Code)
	}
	// the same uid on iOS still has the base limit (one shared counter: 4 used)
	if w := call("u-win", "ios"); w.Code != http.StatusOK || w.Header().Get("X-RateLimit-Remaining") != "4" {
		t.Fatalf("ios after windows: %d remaining=%q", w.Code, w.Header().Get("X-RateLimit-Remaining"))
	}
}
