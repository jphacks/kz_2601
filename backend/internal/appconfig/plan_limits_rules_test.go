package appconfig

import (
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/plan"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// #272 #273 x #317: a record write checks admins_only, then the plan limit,
// then claims its unique key. A refusal by the limit claims no key; a
// duplicate takes no room in the limit. (Shared data needs a Pro creator.)
func TestPlanLimitWithDataRules(t *testing.T) {
	limits := plan.Table{
		Free: plan.Limits{Apps: 1000, RecordsPerApp: 2, PhotosPerApp: 1},
		Pro:  plan.Limits{Apps: 1000, RecordsPerApp: 2, PhotosPerApp: 2},
		Mode: plan.ModeEnforce,
	}
	run := func(t *testing.T, st store.Store, sfx string) {
		t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
		e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st, WithPro(proSet{"owner": true}), WithPlanLimits(limits)))}
		app := "/v1/apps/SHIFT-PLAN" + sfx
		recs := app + "/entities/shift/records"
		joinShift(t, e, app)

		if r := e.do("staff", http.MethodPost, recs, `{"data":{"day":"2026-11-07","slot":9}}`, nil); r.code != http.StatusCreated {
			t.Fatalf("first: %d %s", r.code, r.raw)
		}
		wantDuplicate(t, e.do("boss", http.MethodPost, recs, `{"data":{"day":"2026-11-07","slot":9}}`, nil), "duplicate")
		r := e.do("boss", http.MethodPost, recs, `{"data":{"day":"2026-11-07","slot":10}}`, nil)
		if r.code != http.StatusCreated {
			t.Fatalf("second fits (the duplicate took no room): %d %s", r.code, r.raw)
		}
		second := r.body["id"].(string)
		// full: admins_only is told before the limit, the limit before a duplicate
		wantAdminsOnly(t, e.do("staff", http.MethodPost, recs, `{"data":{"day":"2026-11-07","slot":11,"confirmed":true}}`, nil), "admins_only first")
		wantLimit(t, "limit before duplicate", e.do("staff", http.MethodPost, recs, `{"data":{"day":"2026-11-07","slot":9}}`, nil), plan.RecordsPerApp, "pro", 2, false)
		wantLimit(t, "limit", e.do("staff", http.MethodPost, recs, `{"data":{"day":"2026-11-07","slot":11}}`, nil), plan.RecordsPerApp, "pro", 2, false)
		// the refused slot 11 holds no key: after making room it can be taken
		if r = e.do("owner", http.MethodDelete, recs+"/"+second, nil, nil); r.code != http.StatusNoContent {
			t.Fatalf("delete: %d %s", r.code, r.raw)
		}
		if r = e.do("staff", http.MethodPost, recs, `{"data":{"day":"2026-11-07","slot":11}}`, nil); r.code != http.StatusCreated {
			t.Fatalf("slot refused by the limit: %d %s", r.code, r.raw)
		}
	}
	t.Run("memory", func(t *testing.T) { run(t, store.NewMemory(), "") })
	t.Run("firestore", func(t *testing.T) {
		if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
			t.Skip("FIRESTORE_EMULATOR_HOST not set")
		}
		st, err := store.NewFirestore(t.Context(), "demo-kz2601", "(default)")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.Close() })
		run(t, st, "-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	})
}
