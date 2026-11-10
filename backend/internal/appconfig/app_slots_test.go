package appconfig

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/plan"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// fakeSlots is the coin wallet's slots per uid.
type fakeSlots map[string]coins.SlotInfo

func (f fakeSlots) AppSlots(_ context.Context, uid string) (coins.SlotInfo, error) {
	return f[uid], nil
}

func newSlotEnv(t *testing.T, pro proSet, slots fakeSlots, mode plan.Mode) *env {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory(), WithPro(pro), WithPlanLimits(tinyPlans(mode)), WithAppSlots(slots)))}
}

// wantAppLimit checks a 402 app_limit_reached and returns its details.
func wantAppLimit(t *testing.T, what string, r resp, held, limit int) map[string]any {
	t.Helper()
	if r.code != http.StatusPaymentRequired || r.errCode() != "app_limit_reached" {
		t.Fatalf("%s: want 402 app_limit_reached, got %d %s", what, r.code, r.raw)
	}
	e, _ := r.body["error"].(map[string]any)
	d, _ := e["details"].(map[string]any)
	if d["resource"] != plan.Apps || d["held"] != float64(held) || d["limit"] != float64(limit) {
		t.Fatalf("%s: details %v", what, d)
	}
	if m, _ := e["message"].(string); m == "" {
		t.Fatalf("%s: no message", what)
	}
	return d
}

// heldIDs maps details.apps to id -> mine.
func heldIDs(d map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, a := range d["apps"].([]any) {
		m := a.(map[string]any)
		out[m["id"].(string)] = m["mine"].(bool)
	}
	return out
}

// Free holds 2 apps however a new one comes (the first GET of a new appId,
// POST /apps, duplicate, a share link); a blank app is not held; Pro holds 3.
func TestHeldAppLimit(t *testing.T) {
	pro := proSet{}
	e := newSlotEnv(t, pro, fakeSlots{}, plan.ModeEnforce)
	e.ownApp("u", "/v1/apps/HA-1", kakeibo(t))
	// a blank (opened, never generated) is not held
	if r := e.do("u", http.MethodGet, "/v1/apps/HA-BLANK/dsl", nil, nil); r.code != http.StatusOK {
		t.Fatalf("blank: %d %s", r.code, r.raw)
	}
	e.ownApp("u", "/v1/apps/HA-2", kakeibo(t))

	d := wantAppLimit(t, "new appId", e.do("u", http.MethodGet, "/v1/apps/HA-3/dsl", nil, nil), 2, 2)
	if d["plan"] != "free" || d["base"] != float64(2) || d["purchasedSlots"] != float64(0) || d["canBuySlot"] != false || d["upgrade"] != true {
		t.Fatalf("details %v", d)
	}
	if ids := heldIDs(d); len(ids) != 2 || !ids["HA-1"] || !ids["HA-2"] {
		t.Fatalf("apps %v", d["apps"])
	}
	wantAppLimit(t, "POST", e.do("u", http.MethodPost, "/v1/apps", map[string]any{"id": "HA-4"}, nil), 2, 2)
	wantAppLimit(t, "duplicate", e.do("u", http.MethodPost, "/v1/apps/HA-1/duplicate", nil, nil), 2, 2)
	// the blank made before the limit cannot be generated into a third app
	wantAppLimit(t, "generate the blank", e.do("u", http.MethodPut, "/v1/apps/HA-BLANK/dsl", putBody(kakeibo(t)), ifMatch(1)), 2, 2)
	// the held apps still open and change
	if r := e.do("u", http.MethodGet, "/v1/apps/HA-1/dsl", nil, nil); r.code != http.StatusOK {
		t.Fatalf("open: %d %s", r.code, r.raw)
	}
	if r := e.do("u", http.MethodPost, "/v1/apps/HA-1/versions/1/restore", nil, nil); r.code != http.StatusCreated {
		t.Fatalf("restore: %d %s", r.code, r.raw)
	}
	// deleting one makes room again
	if r := e.do("u", http.MethodDelete, "/v1/apps/HA-2", nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", r.code, r.raw)
	}
	if r := e.do("u", http.MethodPut, "/v1/apps/HA-BLANK/dsl", putBody(kakeibo(t)), ifMatch(1)); r.code != http.StatusCreated {
		t.Fatalf("after delete: %d %s", r.code, r.raw)
	}

	pro["u"] = true
	if r := e.do("u", http.MethodPost, "/v1/apps/HA-1/duplicate", nil, nil); r.code != http.StatusCreated {
		t.Fatalf("pro duplicate: %d %s", r.code, r.raw)
	}
	if d := wantAppLimit(t, "pro", e.do("u", http.MethodPost, "/v1/apps/HA-1/duplicate", nil, nil), 3, 3); d["plan"] != "pro" || d["upgrade"] != false {
		t.Fatalf("pro details %v", d)
	}
}

// A shared app the caller joined is held too, and listed as not theirs.
func TestHeldAppLimitJoined(t *testing.T) {
	e := newSlotEnv(t, proSet{"owner": true}, fakeSlots{}, plan.ModeEnforce)
	const shared = "/v1/apps/HJ-SHARED"
	e.ownApp("owner", shared, sharedKakeibo(t))
	tok := e.share("owner", shared)
	e.ownApp("friend", "/v1/apps/HJ-OWN", kakeibo(t))
	if r := e.do("friend", http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil); r.code != http.StatusCreated || r.body["joined"] != true {
		t.Fatalf("join: %d %s", r.code, r.raw)
	}
	d := wantAppLimit(t, "3rd", e.do("friend", http.MethodGet, "/v1/apps/HJ-NEW/dsl", nil, nil), 2, 2)
	if ids := heldIDs(d); !ids["HJ-OWN"] || ids["HJ-SHARED"] {
		t.Fatalf("mine flags %v", d["apps"])
	}
	// joining again what one already holds is not a new app
	if r := e.do("friend", http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil); r.code != http.StatusOK && r.code != http.StatusCreated {
		t.Fatalf("re-join: %d %s", r.code, r.raw)
	}
	// at the limit, a new link is refused and leaves no blank copy behind
	other := "/v1/apps/HJ-OTHER"
	e.ownApp("owner", other, sharedKakeibo(t))
	wantAppLimit(t, "join at limit", e.do("friend", http.MethodPost, "/v1/shares/"+e.share("owner", other)+"/import", nil, nil), 2, 2)
	if r := e.do("friend", http.MethodGet, "/v1/apps/HJ-OTHER", nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("blank copy left: %d %s", r.code, r.raw)
	}
}

// Bought slots raise the limit; details say whether one more can be bought.
func TestHeldAppLimitSlots(t *testing.T) {
	slots := fakeSlots{"u": {Purchased: 1, Price: 7, Available: 20}}
	e := newSlotEnv(t, proSet{}, slots, plan.ModeEnforce)
	for _, id := range []string{"HS-1", "HS-2", "HS-3"} {
		e.ownApp("u", "/v1/apps/"+id, kakeibo(t))
	}
	d := wantAppLimit(t, "4th", e.do("u", http.MethodGet, "/v1/apps/HS-4/dsl", nil, nil), 3, 3)
	if d["base"] != float64(2) || d["purchasedSlots"] != float64(1) || d["canBuySlot"] != true || d["slotCoins"] != float64(7) || d["balance"] != float64(20) {
		t.Fatalf("details %v", d)
	}
	slots["u"] = coins.SlotInfo{Purchased: 2}
	e.ownApp("u", "/v1/apps/HS-4", kakeibo(t))
	u := e.do("u", http.MethodGet, "/v1/me/usage", nil, nil)
	if a, _ := u.body["apps"].(map[string]any); a["used"] != float64(4) || a["limit"] != float64(4) {
		t.Fatalf("usage apps %v", u.body["apps"])
	}
	if s, _ := u.body["appSlots"].(map[string]any); s["purchased"] != float64(2) || s["canBuySlot"] != false {
		t.Fatalf("usage appSlots %v", u.body["appSlots"])
	}
}

// Two blanks generated at once with one slot left: one is refused.
func TestHeldAppLimitConcurrent(t *testing.T) {
	e := newSlotEnv(t, proSet{}, fakeSlots{}, plan.ModeEnforce)
	e.ownApp("u", "/v1/apps/HC-1", kakeibo(t))
	ids := []string{"HC-A", "HC-B", "HC-C"}
	for _, id := range ids {
		if r := e.do("u", http.MethodGet, "/v1/apps/"+id+"/dsl", nil, nil); r.code != http.StatusOK {
			t.Fatalf("blank %s: %d %s", id, r.code, r.raw)
		}
	}
	codes := make([]int, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = e.do("u", http.MethodPut, "/v1/apps/"+id+"/dsl", putBody(kakeibo(t)), ifMatch(1)).code
		}()
	}
	wg.Wait()
	created, refused := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusPaymentRequired:
			refused++
		}
	}
	if created != 1 || refused != 2 {
		t.Fatalf("codes %v", codes)
	}
}

// PLAN_LIMITS_MODE=log never stops the held apps.
func TestHeldAppLimitLogMode(t *testing.T) {
	e := newSlotEnv(t, proSet{}, fakeSlots{}, plan.ModeLog)
	for _, id := range []string{"HL-1", "HL-2", "HL-3"} {
		e.ownApp("u", "/v1/apps/"+id, kakeibo(t))
	}
	if r := e.do("u", http.MethodPost, "/v1/apps/HL-1/duplicate", nil, nil); r.code != http.StatusCreated {
		t.Fatalf("duplicate: %d %s", r.code, r.raw)
	}
}

// staleCount answers CountApps 0, as if another request took the last slot
// after checkAppLimit counted.
type staleCount struct{ *store.Memory }

func (staleCount) CountApps(context.Context, string) (int, error) { return 0, nil }

// A copy that loses the last slot in the commit removes its blank.
func TestHeldAppLimitLostRaceRemovesBlank(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(staleCount{store.NewMemory()}, WithPlanLimits(tinyPlans(plan.ModeEnforce))))}
	e.ownApp("u", "/v1/apps/HR-1", kakeibo(t))
	e.ownApp("u", "/v1/apps/HR-2", kakeibo(t))
	wantAppLimit(t, "duplicate", e.do("u", http.MethodPost, "/v1/apps/HR-1/duplicate", nil, nil), 2, 2)
	if r := e.do("u", http.MethodGet, "/v1/apps", nil, nil); len(r.body["items"].([]any)) != 2 {
		t.Fatalf("a blank copy was left: %s", r.raw)
	}
}

// Two joins at once with one slot left (a share link): one is refused and
// leaves no blank copy behind.
func TestHeldAppLimitConcurrentJoins(t *testing.T) {
	e := newSlotEnv(t, proSet{"owner": true}, fakeSlots{}, plan.ModeEnforce)
	e.ownApp("friend", "/v1/apps/HCJ-OWN", kakeibo(t))
	ids := []string{"HCJ-A", "HCJ-B", "HCJ-C"}
	toks := make([]string, len(ids))
	for i, id := range ids {
		e.ownApp("owner", "/v1/apps/"+id, sharedKakeibo(t))
		toks[i] = e.share("owner", "/v1/apps/"+id)
	}
	codes := make([]int, len(ids))
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = e.do("friend", http.MethodPost, "/v1/shares/"+toks[i]+"/import", nil, nil).code
		}()
	}
	wg.Wait()
	created, refused := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusPaymentRequired:
			refused++
		}
	}
	if created != 1 || refused != 2 {
		t.Fatalf("codes %v", codes)
	}
	if r := e.do("friend", http.MethodGet, "/v1/apps", nil, nil); len(r.body["items"].([]any)) != 2 {
		t.Fatalf("held %s", r.raw)
	}
}
