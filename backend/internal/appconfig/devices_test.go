package appconfig

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// devTokenSalt keeps this package's tokens apart from the other packages'
// that share the emulator in parallel: the token index (device_tokens) is
// global, so registering the same token elsewhere would drop our device.
var devTokenSalt = rand.Uint64()

// devToken is the n-th test token, lower-case as the store keeps it.
func devToken(n int) string { return fmt.Sprintf("%016x%048x", devTokenSalt, n) }

func deviceBody(tok int) string {
	return fmt.Sprintf(`{"token":%q,"environment":"sandbox","appVersion":"1.2.0","os":"iOS 18.1"}`, strings.ToUpper(devToken(tok)))
}

// #269: PUT / DELETE /v1/me/devices/{installationId} on the caller's own devices.
func TestDevicesAPI(t *testing.T) {
	storeEnvs(t, func(t *testing.T, e *env, st store.Store, sfx string) {
		ctx := context.Background()
		a, b := "dev-api-a"+sfx, "dev-api-b"+sfx
		path := "/v1/me/devices/inst-0001" + sfx

		r := e.do(a, http.MethodPut, path, deviceBody(1), nil)
		if r.code != http.StatusOK || r.body["environment"] != "sandbox" || r.body["os"] != "iOS 18.1" || r.body["createdAt"] == nil {
			t.Fatalf("put: %d %s", r.code, r.raw)
		}
		if strings.Contains(strings.ToLower(string(r.raw)), devToken(1)) || r.body["token"] != nil {
			t.Fatalf("token echoed: %s", r.raw)
		}
		ds, err := st.ListDevices(ctx, a)
		if err != nil || len(ds) != 1 || ds[0].Token != devToken(1) || ds[0].Environment != store.DeviceSandbox {
			t.Fatalf("stored (lower-cased): %+v %v", ds, err)
		}
		// update keeps one entry
		if r := e.do(a, http.MethodPut, path, deviceBody(2), nil); r.code != http.StatusOK {
			t.Fatalf("update: %d %s", r.code, r.raw)
		}
		if ds, _ := st.ListDevices(ctx, a); len(ds) != 1 || ds[0].Token != devToken(2) {
			t.Fatalf("after update: %+v", ds)
		}

		// someone else cannot reach a's device: the path is always the caller's own
		if r := e.do(b, http.MethodDelete, path, nil, nil); r.code != http.StatusNoContent {
			t.Fatalf("b delete: %d %s", r.code, r.raw)
		}
		if ds, _ := st.ListDevices(ctx, a); len(ds) != 1 {
			t.Fatalf("b's delete removed a's device: %+v", ds)
		}
		if r := e.do("", http.MethodPut, path, deviceBody(3), nil); r.code != http.StatusUnauthorized {
			t.Fatalf("no auth: %d", r.code)
		}

		if r := e.do(a, http.MethodDelete, path, nil, nil); r.code != http.StatusNoContent {
			t.Fatalf("delete: %d %s", r.code, r.raw)
		}
		if r := e.do(a, http.MethodDelete, path, nil, nil); r.code != http.StatusNoContent {
			t.Fatalf("delete again: %d %s", r.code, r.raw)
		}
		if ds, _ := st.ListDevices(ctx, a); len(ds) != 0 {
			t.Fatalf("left: %+v", ds)
		}
	})
}

func TestDevicesAPIValidation(t *testing.T) {
	e := newEnv(t)
	ok := deviceBody(1)
	for _, c := range []struct{ name, path, body, field string }{
		{"short id", "/v1/me/devices/abc", ok, ""},
		{"id with dot", "/v1/me/devices/inst.0001", ok, ""},
		{"no token", "/v1/me/devices/inst-0001", `{"environment":"sandbox"}`, "token"},
		{"not hex", "/v1/me/devices/inst-0001", `{"token":"` + strings.Repeat("zz", 32) + `","environment":"sandbox"}`, "token"},
		{"too short", "/v1/me/devices/inst-0001", `{"token":"` + strings.Repeat("ab", 31) + `","environment":"sandbox"}`, "token"},
		{"odd length", "/v1/me/devices/inst-0001", `{"token":"` + strings.Repeat("ab", 32) + `a","environment":"sandbox"}`, "token"},
		{"bad environment", "/v1/me/devices/inst-0001", `{"token":"` + strings.Repeat("ab", 32) + `","environment":"dev"}`, "environment"},
		{"long os", "/v1/me/devices/inst-0001", `{"token":"` + strings.Repeat("ab", 32) + `","environment":"production","os":"` + strings.Repeat("x", 33) + `"}`, "os"},
		{"unknown field", "/v1/me/devices/inst-0001", `{"token":"` + strings.Repeat("ab", 32) + `","environment":"production","uid":"x"}`, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := e.do("val", http.MethodPut, c.path, c.body, nil)
			if r.code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d %s", r.code, r.raw)
			}
			if c.field != "" {
				d, _ := r.body["error"].(map[string]any)["details"].(map[string]any)
				if d["field"] != c.field {
					t.Fatalf("field: %s", r.raw)
				}
			}
		})
	}
}

// #269: the 11th device removes the least recently updated.
func TestDevicesAPILimit(t *testing.T) {
	st := store.NewMemory()
	e := newEnv(t)
	e.h = auth.Middleware(nil, auth.ModeDev)(New(st))
	for i := range store.MaxDevicesPerUser + 1 {
		if r := e.do("lim", http.MethodPut, fmt.Sprintf("/v1/me/devices/inst-%04d", i), deviceBody(10+i), nil); r.code != http.StatusOK {
			t.Fatalf("put %d: %d %s", i, r.code, r.raw)
		}
	}
	ds, _ := st.ListDevices(context.Background(), "lim")
	if len(ds) != store.MaxDevicesPerUser {
		t.Fatalf("count: %d", len(ds))
	}
	for _, d := range ds {
		if d.InstallationID == "inst-0000" {
			t.Fatal("the oldest device was kept")
		}
	}
}

// #339: PUT /v1/me/notify-consent/{appId}.
func TestNotifyConsentAPI(t *testing.T) {
	storeEnvs(t, func(t *testing.T, e *env, st store.Store, sfx string) {
		ctx := context.Background()
		owner, mom, stranger := "nc-owner"+sfx, "nc-mom"+sfx, "nc-stranger"+sfx
		id := "NC-FAMILY" + sfx
		app := "/v1/apps/" + id
		consent := "/v1/me/notify-consent/" + id
		e.ownApp(owner, app, familyKakeibo(t))
		tok := e.share(owner, app)
		if r := e.do(mom, http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil); r.code != http.StatusCreated {
			t.Fatalf("join: %d %s", r.code, r.raw)
		}

		r := e.do(mom, http.MethodPut, consent, `{"allowed":true}`, nil)
		if r.code != http.StatusOK || r.body["allowed"] != true || r.body["appId"] != id {
			t.Fatalf("mom: %d %s", r.code, r.raw)
		}
		if ok, err := st.NotifyAllowed(ctx, mom, id); err != nil || !ok {
			t.Fatalf("stored: %v %v", ok, err)
		}

		// no copy of the app: 404; a copy without joining the shared data: 403
		if r := e.do(stranger, http.MethodPut, consent, `{"allowed":true}`, nil); r.code != http.StatusNotFound {
			t.Fatalf("stranger without copy: %d %s", r.code, r.raw)
		}
		e.do(stranger, http.MethodGet, app+"/dsl", nil, nil)
		wantNotMember(t, e.do(stranger, http.MethodPut, consent, `{"allowed":true}`, nil), "stranger consent")
		if ok, _ := st.NotifyAllowed(ctx, stranger, id); ok {
			t.Fatal("refused consent was stored")
		}
		// turning it off always works
		if r := e.do(stranger, http.MethodPut, consent, `{"allowed":false}`, nil); r.code != http.StatusOK {
			t.Fatalf("stranger off: %d %s", r.code, r.raw)
		}
		if r := e.do(mom, http.MethodPut, consent, `{"allowed":false}`, nil); r.code != http.StatusOK || r.body["allowed"] != false {
			t.Fatalf("mom off: %d %s", r.code, r.raw)
		}
		if ok, _ := st.NotifyAllowed(ctx, mom, id); ok {
			t.Fatal("withdrawal not stored")
		}
		if r := e.do(mom, http.MethodPut, consent, `{}`, nil); r.code != http.StatusBadRequest {
			t.Fatalf("missing allowed: %d %s", r.code, r.raw)
		}
	})
}

// Decision #337: DELETE /v1/me removes the devices at once.
func TestDeleteMeRemovesDevices(t *testing.T) {
	st := store.NewMemory()
	e := newEnv(t)
	e.h = auth.Middleware(nil, auth.ModeDev)(New(st))
	if r := e.do("leaver", http.MethodPut, "/v1/me/devices/inst-0001", deviceBody(5), nil); r.code != http.StatusOK {
		t.Fatalf("put: %d %s", r.code, r.raw)
	}
	if r := e.do("leaver", http.MethodDelete, "/v1/me", nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("delete me: %d %s", r.code, r.raw)
	}
	if ds, _ := st.ListDevices(context.Background(), "leaver"); len(ds) != 0 {
		t.Fatalf("devices after deletion: %+v", ds)
	}
}
