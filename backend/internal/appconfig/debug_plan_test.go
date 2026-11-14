package appconfig

import (
	"net/http"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/debugoverride"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// #2085: on stg an allowlisted uid sending X-Debug-Plan: free is refused a
// paid save although its stored entitlement is Pro; pro the other way round.
func TestDebugPlanOverridesProGate(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	cfg, err := debugoverride.FromEnv(func(k string) string {
		return map[string]string{"APP_ENV": "stg", debugoverride.EnabledEnv: "1", debugoverride.UIDsEnv: "listedPro,listedFree"}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	pro := debugoverride.Wrap(proSet{"listedPro": true, "otherPro": true}, cfg, nil)
	e := &env{t: t, h: debugoverride.Middleware(cfg)(auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory(), WithPro(pro))))}
	put := func(uid, app, plan string) resp {
		r := e.do(uid, http.MethodGet, app+"/dsl", nil, nil)
		h := ifMatch(r.version())
		if plan != "" {
			h[debugoverride.Header] = plan
		}
		return e.do(uid, http.MethodPut, app+"/dsl", putBody(sharedKakeibo(t)), h)
	}

	wantPro(t, "listed, stored Pro, free", put("listedPro", "/v1/apps/DP-1", "free"), "sharedData")
	if r := put("listedPro", "/v1/apps/DP-1", ""); r.code != http.StatusCreated {
		t.Fatalf("listed, stored Pro, no header: %d %s", r.code, r.raw)
	}
	if r := put("listedFree", "/v1/apps/DP-2", "pro"); r.code != http.StatusCreated {
		t.Fatalf("listed, free, pro: %d %s", r.code, r.raw)
	}
	wantPro(t, "listed, free, no header", put("listedFree", "/v1/apps/DP-3", ""), "sharedData")
	wantPro(t, "not listed, pro", put("stranger", "/v1/apps/DP-4", "pro"), "sharedData")
	if r := put("otherPro", "/v1/apps/DP-5", "free"); r.code != http.StatusCreated {
		t.Fatalf("not listed, stored Pro, free: %d %s", r.code, r.raw)
	}
}
