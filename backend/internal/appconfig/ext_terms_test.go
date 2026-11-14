package appconfig

import (
	"net/http"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/extapi"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// The answer carries the provider's credit and noAI for the device.
func TestExtCallTermsInAnswer(t *testing.T) {
	t.Cleanup(extapi.SetTermsForTest(extapi.FakeFree, extapi.Terms{NoAI: true, Attribution: "Data by Fake"}))
	e := newExtEnv(t, 100)
	const app = "/v1/apps/EXT-SOLO"
	e.ownApp("solo", app, kakeibo(t))
	r := e.do("solo", http.MethodPost, app+"/ext/fakeFree/listItems", `{"args":{"q":"rain"}}`, nil)
	if r.code != http.StatusOK || r.body["attribution"] != "Data by Fake" || r.body["noAI"] != true {
		t.Fatalf("%d %s", r.code, r.raw)
	}
}

// NoCharge: a creator without Pro can call (the terms forbid charging).
func TestExtCallNoChargeWithoutPro(t *testing.T) {
	t.Cleanup(extapi.SetTermsForTest(extapi.FakeFree, extapi.Terms{NoCharge: true}))
	e := newExtEnv(t, 100)
	e.ownApp("free", "/v1/apps/EXT-FREE", kakeibo(t))
	if r := e.do("free", http.MethodPost, "/v1/apps/EXT-FREE/ext/fakeFree/listItems", `{"args":{}}`, nil); r.code != http.StatusOK {
		t.Fatalf("%d %s", r.code, r.raw)
	}
}

// ShowOwnDataOnly: not in a shared app (neither for its members nor for
// the creator); fine in a private app of one's own.
func TestExtCallOwnDataOnly(t *testing.T) {
	t.Cleanup(extapi.SetTermsForTest(extapi.FakeOAuth, extapi.Terms{ShowOwnDataOnly: true}))
	e := newExtEnv(t, 100)
	const shared = "/v1/apps/EXT-FAMILY"
	e.ownApp("owner", shared, familyKakeibo(t))
	tok := e.share("owner", shared)
	if r := e.do("mom", http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil); r.code != http.StatusCreated {
		t.Fatalf("join: %d %s", r.code, r.raw)
	}
	for _, uid := range []string{"mom", "owner"} {
		r := e.do(uid, http.MethodPost, shared+"/ext/fakeOauth/listEvents", `{"args":{}}`, nil)
		wantExtErr(t, r, http.StatusForbidden, httpx.CodeForbidden, uid+" in a shared app")
		if r.reason() != reasonOwnDataOnly {
			t.Fatalf("%s: %s", uid, r.raw)
		}
	}
	// a private app: reaches the runner (409: not connected)
	e.ownApp("solo", "/v1/apps/EXT-SOLO", kakeibo(t))
	wantExtErr(t, e.do("solo", http.MethodPost, "/v1/apps/EXT-SOLO/ext/fakeOauth/listEvents", `{"args":{}}`, nil),
		http.StatusConflict, httpx.CodeExtNotConnected, "private app")
}
