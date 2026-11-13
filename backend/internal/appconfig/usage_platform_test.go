package appconfig

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/plan"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// #2133: GET /v1/me/usage shows the same per-OS limit as ai's X-RateLimit-Limit.
func TestMyUsageAIUnitsPlatformLimit(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, ratelimit.JST)
	clock := func() time.Time { return now }
	rate := ratelimit.Config{
		Limits:         ratelimit.Limits{Anonymous: 30, SignedIn: 60, Pro: 300},
		PlatformLimits: map[string]map[ratelimit.Tier]int{httpx.PlatformWindows: {ratelimit.TierSignedIn: 20}},
		Weights:        ratelimit.DefaultWeights,
		Env:            "test",
		Now:            clock,
	}
	mem := ratelimit.NewMemoryClock(clock)
	limiter := ratelimit.New(rate, mem, nil)
	e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory(),
		WithPlanLimits(tinyPlans(plan.ModeEnforce)), WithUsage(UsageConfig{Rate: rate, Now: clock, Counters: mem})))}

	d, err := limiter.AllowPlatform(context.Background(), auth.Identity{UID: "apple-1"}, "", httpx.PlatformWindows, 3)
	if err != nil || !d.Allowed || d.Limit != 20 {
		t.Fatalf("charge: %+v %v", d, err)
	}
	for platform, limit := range map[string]int{"windows": 20, "WINDOWS": 20, "ios": 60, "": 60} {
		var hdr map[string]string
		if platform != "" {
			hdr = map[string]string{httpx.ClientPlatformHeader: platform}
		}
		r := e.do("apple-1", http.MethodGet, "/v1/me/usage", nil, hdr)
		if r.code != http.StatusOK {
			t.Fatalf("%q: %d %s", platform, r.code, r.raw)
		}
		wantMeter(t, "platform "+platform, r.body["ai"], 3, limit)
	}
}
