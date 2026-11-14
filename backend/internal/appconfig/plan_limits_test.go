package appconfig

import (
	"net/http"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/plan"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// tinyPlans makes every limit reachable in a few requests.
func tinyPlans(mode plan.Mode) plan.Table {
	return plan.Table{
		Free: plan.Limits{Apps: 2, RecordsPerApp: 2, PhotosPerApp: 1},
		Pro:  plan.Limits{Apps: 3, RecordsPerApp: 3, PhotosPerApp: 2},
		Mode: mode,
	}
}

func newPlanEnv(t *testing.T, pro proSet, t2 plan.Table) *env {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory(), WithPro(pro), WithPlanLimits(t2)))}
}

// wantLimit checks a 403 plan_limit_reached and returns its details.
func wantLimit(t *testing.T, what string, r resp, resource, planName string, limit int, upgrade bool) map[string]any {
	t.Helper()
	if r.code != http.StatusForbidden || r.errCode() != "plan_limit_reached" {
		t.Fatalf("%s: want 403 plan_limit_reached, got %d %s", what, r.code, r.raw)
	}
	e, _ := r.body["error"].(map[string]any)
	d, _ := e["details"].(map[string]any)
	if d["resource"] != resource || d["plan"] != planName || d["limit"] != float64(limit) || d["upgrade"] != upgrade {
		t.Fatalf("%s: details %v", what, d)
	}
	if m, _ := e["message"].(string); m == "" {
		t.Fatalf("%s: no message", what)
	}
	return d
}

// Records and photos per app; patches that add no record or photo pass.
func TestPlanLimitRecordsAndPhotos(t *testing.T) {
	e := newPlanEnv(t, proSet{}, tinyPlans(plan.ModeEnforce))
	const app = "/v1/apps/PL-R"
	recs := app + "/entities/expense/records"
	e.ownApp("u", app, kakeibo(t))

	id := e.post("u", recs, `{"data":{"amount":1,"receipt":"https://example.com/1.jpg"}}`)
	id2 := e.post("u", recs, `{"data":{"amount":2}}`)
	wantLimit(t, "3rd record", e.do("u", http.MethodPost, recs, `{"data":{"amount":3}}`, nil), plan.RecordsPerApp, "free", 2, true)
	// a patch is not a new record; a photo where there was none is a new photo
	e.patch("u", recs+"/"+id, `{"data":{"amount":10}}`)
	e.patch("u", recs+"/"+id, `{"data":{"receipt":"https://example.com/1b.jpg"}}`) // replacing keeps the count
	wantLimit(t, "2nd photo", e.do("u", http.MethodPatch, recs+"/"+id2, `{"data":{"receipt":"https://example.com/2.jpg"}}`, nil), plan.PhotosPerApp, "free", 1, true)
	// deleting makes room again
	if r := e.do("u", http.MethodDelete, recs+"/"+id, nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", r.code, r.raw)
	}
	e.post("u", recs, `{"data":{"amount":3,"receipt":"https://example.com/3.jpg"}}`)
}

// A participant of a shared app hits the creator's plan, and is told so.
func TestPlanLimitCreatorPays(t *testing.T) {
	e := newPlanEnv(t, proSet{"owner": true}, tinyPlans(plan.ModeEnforce))
	const app = "/v1/apps/PL-SHARED"
	recs := app + "/entities/expense/records"
	e.ownApp("owner", app, sharedKakeibo(t))
	e.post("owner", recs, `{"data":{"amount":1}}`)
	e.post("owner", recs, `{"data":{"amount":2}}`)
	// installing the shared app is not one of the participant's apps
	if r := e.do("friend", http.MethodGet, app+"/dsl", nil, nil); r.code != http.StatusOK {
		t.Fatalf("install: %d %s", r.code, r.raw)
	}
	e.post("friend", recs, `{"data":{"amount":3}}`) // free participant, creator's Pro limit (3)
	d := wantLimit(t, "participant over the creator's limit", e.do("friend", http.MethodPost, recs, `{"data":{"amount":4}}`, nil), plan.RecordsPerApp, "pro", 3, false)
	if d["reason"] != "creator_plan_limit" {
		t.Fatalf("reason: %v", d)
	}
	if d := wantLimit(t, "creator", e.do("owner", http.MethodPost, recs, `{"data":{"amount":5}}`, nil), plan.RecordsPerApp, "pro", 3, false); d["reason"] != nil {
		t.Fatalf("creator reason: %v", d)
	}
}

// PLAN_LIMITS_MODE=log and off never stop.
func TestPlanLimitModes(t *testing.T) {
	for _, m := range []plan.Mode{plan.ModeLog, plan.ModeOff} {
		t.Run(string(m), func(t *testing.T) {
			e := newPlanEnv(t, proSet{}, tinyPlans(m))
			for _, id := range []string{"PL-M1", "PL-M2", "PL-M3"} {
				if r := e.do("u", http.MethodPost, "/v1/apps", map[string]any{"id": id}, nil); r.code != http.StatusCreated {
					t.Fatalf("%s: %d %s", id, r.code, r.raw)
				}
			}
		})
	}
}
