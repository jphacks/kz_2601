package appconfig

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/plan"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

type usageEnv struct {
	*env
	now     time.Time
	limiter *ratelimit.Limiter
}

func newUsageEnv(t *testing.T, pro proSet, table plan.Table, withAI bool) *usageEnv {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	u := &usageEnv{now: time.Date(2026, 11, 7, 23, 0, 0, 0, ratelimit.JST)}
	clock := func() time.Time { return u.now }
	rate := ratelimit.Config{Limits: ratelimit.Limits{Anonymous: 30, SignedIn: 60, Pro: 300}, Weights: ratelimit.DefaultWeights, Env: "test", Now: clock}
	mem := ratelimit.NewMemoryClock(clock)
	u.limiter = ratelimit.New(rate, mem, pro)
	uc := UsageConfig{Rate: rate, Now: clock}
	if withAI {
		uc.Counters = mem
	}
	opts := []Option{WithPro(pro), WithPlanLimits(table), WithUsage(uc)}
	u.env = &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory(), opts...))}
	return u
}

// charge is what the ai service does on POST /v1/generate (weight 3 = create).
func (u *usageEnv) charge(uid string, weight int) ratelimit.Decision {
	u.t.Helper()
	d, err := u.limiter.Allow(context.Background(), auth.Identity{UID: uid, Anonymous: uid[:4] == "anon"}, "", weight)
	if err != nil || !d.Allowed {
		u.t.Fatalf("charge: %+v %v", d, err)
	}
	return d
}

func (u *usageEnv) usage(uid, path string) map[string]any {
	u.t.Helper()
	r := u.do(uid, http.MethodGet, path, nil, nil)
	if r.code != http.StatusOK {
		u.t.Fatalf("GET %s: %d %s", path, r.code, r.raw)
	}
	if cc := r.hdr.Get("Cache-Control"); cc != "private, no-store" {
		u.t.Fatalf("Cache-Control %q", cc)
	}
	return r.body
}

func wantMeter(t *testing.T, what string, v any, used int, limit any) {
	t.Helper()
	m, _ := v.(map[string]any)
	if m == nil || m["used"] != float64(used) {
		t.Fatalf("%s: %v (want used %d)", what, v, used)
	}
	if limit == nil {
		if m["limit"] != nil || m["remaining"] != nil {
			t.Fatalf("%s: want no limit, got %v", what, m)
		}
		return
	}
	l := float64(limit.(int))
	if m["limit"] != l || m["remaining"] != max(0, l-float64(used)) {
		t.Fatalf("%s: %v (want limit %v)", what, m, l)
	}
}

// The AI units are the ones the ai service counts, with the same limit and
// remaining as its X-RateLimit-* headers (what iOS shows as "今日の残り").
func TestMyUsageAIUnitsMatchRateLimit(t *testing.T) {
	pro := proSet{"pro-1": true}
	u := newUsageEnv(t, pro, tinyPlans(plan.ModeEnforce), true)
	for _, tc := range []struct {
		uid, plan, tier string
		limit           int
	}{
		{"apple-1", "free", "signed_in", 60},
		{"anon-1", "free", "anonymous", 30},
		{"pro-1", "pro", "pro", 300},
	} {
		u.charge(tc.uid, 3)
		d := u.charge(tc.uid, 1)
		b := u.usage(tc.uid, "/v1/me/usage")
		if b["plan"] != tc.plan || b["tier"] != tc.tier || b["resetAt"] != "2026-11-07T15:00:00Z" {
			t.Fatalf("%s: %v", tc.uid, b)
		}
		ai, _ := b["ai"].(map[string]any)
		wantMeter(t, tc.uid+" ai", ai, 4, tc.limit)
		if ai["remaining"] != float64(d.Remaining) || ai["limit"] != float64(d.Limit) {
			t.Fatalf("%s: ai %v, header remaining %d / %d", tc.uid, ai, d.Remaining, d.Limit)
		}
		if w, _ := ai["weights"].(map[string]any); w["create"] != float64(3) {
			t.Fatalf("weights: %v", ai["weights"])
		}
	}
	// a refund (#548) shows at once, unlike the header of the request
	d := u.charge("apple-1", 3)
	if err := u.limiter.Refund(context.Background(), d, 3); err != nil {
		t.Fatal(err)
	}
	wantMeter(t, "after refund", u.usage("apple-1", "/v1/me/usage")["ai"], 4, 60)
	// another user's count is not mixed in; the next day starts at 0
	wantMeter(t, "other", u.usage("apple-2", "/v1/me/usage")["ai"], 0, 60)
	u.now = u.now.Add(2 * time.Hour)
	b := u.usage("apple-1", "/v1/me/usage")
	wantMeter(t, "next day", b["ai"], 0, 60)
	if b["resetAt"] != "2026-11-08T15:00:00Z" {
		t.Fatalf("resetAt %v", b["resetAt"])
	}
}

func TestMyUsageApps(t *testing.T) {
	u := newUsageEnv(t, proSet{}, tinyPlans(plan.ModeEnforce), false)
	b := u.usage("u", "/v1/me/usage")
	if b["ai"] != nil {
		t.Fatalf("ai without WithUsage: %v", b["ai"])
	}
	wantMeter(t, "no apps", b["apps"], 0, 2)
	if p, _ := b["perApp"].(map[string]any); p["recordsPerApp"] != float64(2) || p["photosPerApp"] != float64(1) {
		t.Fatalf("perApp %v", b["perApp"])
	}
	u.ownApp("u", "/v1/apps/US-A1", kakeibo(t))
	if s, _ := b["appSlots"].(map[string]any); s["base"] != float64(2) || s["purchased"] != float64(0) || s["limit"] != float64(2) || s["canBuySlot"] != false {
		t.Fatalf("appSlots %v", b["appSlots"])
	}
	wantMeter(t, "cached", u.usage("u", "/v1/me/usage")["apps"], 0, 2) // counts lag up to usageCountTTL
	u.now = u.now.Add(usageCountTTL)
	wantMeter(t, "after ttl", u.usage("u", "/v1/me/usage")["apps"], 1, 2)
}

func TestMyUsageModeOffHasNoLimits(t *testing.T) {
	u := newUsageEnv(t, proSet{}, tinyPlans(plan.ModeOff), false)
	b := u.usage("u", "/v1/me/usage")
	wantMeter(t, "apps", b["apps"], 0, nil)
	if p, _ := b["perApp"].(map[string]any); p["recordsPerApp"] != nil {
		t.Fatalf("perApp %v", p)
	}
}

func TestAppUsageOwnAndParticipant(t *testing.T) {
	pro := proSet{"owner": true}
	u := newUsageEnv(t, pro, tinyPlans(plan.ModeEnforce), false)
	const app = "/v1/apps/US-SHARED"
	recs := app + "/entities/expense/records"
	u.ownApp("owner", app, sharedKakeibo(t))
	u.post("owner", recs, `{"data":{"amount":1,"receipt":"https://example.com/1.jpg"}}`)
	u.post("owner", recs, `{"data":{"amount":2}}`)

	b := u.usage("owner", app+"/usage")
	if b["plan"] != "pro" || b["payer"] != "self" || b["upgrade"] != false || b["appId"] != "US-SHARED" {
		t.Fatalf("owner: %v", b)
	}
	wantMeter(t, "owner records", b["records"], 2, 3)
	wantMeter(t, "owner photos", b["photos"], 1, 2)

	// a free participant counts against the creator's Pro plan, without its uid
	u.do("friend", http.MethodGet, app+"/dsl", nil, nil)
	b = u.usage("friend", app+"/usage")
	if b["plan"] != "pro" || b["payer"] != "creator" {
		t.Fatalf("friend: %v", b)
	}
	if _, ok := b["owner"]; ok {
		t.Fatalf("owner leaked: %v", b)
	}
	wantMeter(t, "friend records", b["records"], 2, 3)
}

// scope:"shared" records count only for participants (the size of another
// group's data is not shown to someone who has not joined).
func TestAppUsageSharedNeedsMembership(t *testing.T) {
	u := newUsageEnv(t, proSet{"owner": true}, tinyPlans(plan.ModeEnforce), false)
	const app = "/v1/apps/US-FAMILY"
	u.ownApp("owner", app, familyKakeibo(t))
	u.post("owner", app+"/entities/expense/records", `{"data":{"amount":1}}`)
	wantMeter(t, "owner", u.usage("owner", app+"/usage")["records"], 1, 3)
	u.do("stranger", http.MethodGet, app+"/dsl", nil, nil)
	b := u.usage("stranger", app+"/usage")
	wantMeter(t, "stranger", b["records"], 0, 3)
	if b["payer"] != "creator" || b["plan"] != "pro" || b["upgrade"] != false {
		t.Fatalf("stranger: %v", b)
	}
}

// Asking does not create the app (unlike GET /dsl): 404 for an app not installed.
func TestAppUsageUnknownApp(t *testing.T) {
	u := newUsageEnv(t, proSet{}, tinyPlans(plan.ModeEnforce), false)
	if r := u.do("u", http.MethodGet, "/v1/apps/US-NONE/usage", nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("%d %s", r.code, r.raw)
	}
	wantMeter(t, "apps", u.usage("u", "/v1/me/usage")["apps"], 0, 2)
	if r := u.do("u", http.MethodGet, "/v1/apps/bad!id/usage", nil, nil); r.code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", r.code)
	}
}
