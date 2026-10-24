package appconfig

import (
	"net/http"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

func TestDeleteMe(t *testing.T) {
	e := newEnv(t)
	st := store.NewMemory()
	e.h = auth.Middleware(nil, auth.ModeDev)(RejectDeleted(st, New(st)))

	// leaver: a private app, a share link, a catalog listing and a post on stayer's shared board
	const mine, board = "/v1/apps/LEAVER-KAKEIBO", "/v1/apps/BOARD-1"
	e.ownApp("leaver", mine, kakeibo(t))
	e.do("leaver", http.MethodPost, mine+"/entities/expense/records", `{"data":{"amount":100}}`, nil)
	tok, _ := e.do("leaver", http.MethodPost, mine+"/share", nil, nil).body["token"].(string)
	if r := e.do("leaver", http.MethodPost, mine+"/publish", `{}`, nil); r.code != http.StatusOK {
		t.Fatalf("publish: %d %s", r.code, r.raw)
	}
	e.ownApp("stayer", board, sharedKakeibo(t))
	e.ownApp("stayer", "/v1/apps/STAYER-OWN", kakeibo(t))
	stayerTok, _ := e.do("stayer", http.MethodPost, "/v1/apps/STAYER-OWN/share", nil, nil).body["token"].(string)
	e.do("stayer", http.MethodPost, "/v1/apps/STAYER-OWN/publish", `{}`, nil)
	e.do("stayer", http.MethodPost, board+"/entities/expense/records", `{"data":{"amount":1,"memo":"stayer"}}`, nil)
	e.do("leaver", http.MethodGet, board+"/dsl", nil, nil) // joins (seeded from apps_public)
	if r := e.do("leaver", http.MethodPost, board+"/entities/expense/records", `{"data":{"amount":2,"memo":"leaver"}}`, nil); r.code != http.StatusCreated {
		t.Fatalf("leaver post: %d %s", r.code, r.raw)
	}

	if r := e.do("", http.MethodDelete, "/v1/me", nil, nil); r.code != http.StatusUnauthorized {
		t.Fatalf("auth required: %d", r.code)
	}
	for i := range 2 { // idempotent
		if r := e.do("leaver", http.MethodDelete, "/v1/me", nil, nil); r.code != http.StatusNoContent {
			t.Fatalf("delete #%d: %d %s", i+1, r.code, r.raw)
		}
	}

	// every later request of the deleted uid is 410
	for _, p := range []string{"/v1/apps", mine + "/dsl", "/v1/public-apps"} {
		if r := e.do("leaver", http.MethodGet, p, nil, nil); r.code != http.StatusGone || r.errCode() != CodeAccountDeleted {
			t.Fatalf("GET %s after delete: %d %s", p, r.code, r.raw)
		}
	}
	// stored data is kept (logical delete) but hidden from others
	ctx := t.Context()
	if a, err := st.GetApp(ctx, "leaver", "LEAVER-KAKEIBO"); err != nil || a.Status != "archived" {
		t.Fatalf("leaver app must be archived: %+v %v", a, err)
	}
	if r := e.do("stayer", http.MethodGet, "/v1/shares/"+tok, nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("leaver share must be revoked: %d %s", r.code, r.raw)
	}
	r := e.do("stayer", http.MethodGet, "/v1/public-apps", nil, nil)
	if items := r.body["items"].([]any); len(items) != 1 || items[0].(map[string]any)["id"] != "STAYER-OWN" {
		t.Fatalf("only stayer's listing must remain: %s", r.raw)
	}

	// stayer is unaffected, and both posts on the shared board remain
	if r := e.do("stayer", http.MethodGet, "/v1/apps", nil, nil); r.code != http.StatusOK || len(r.body["items"].([]any)) != 2 {
		t.Fatalf("stayer apps: %d %s", r.code, r.raw)
	}
	if r := e.do("stayer", http.MethodGet, "/v1/shares/"+stayerTok, nil, nil); r.code != http.StatusOK {
		t.Fatalf("stayer share: %d %s", r.code, r.raw)
	}
	if r := e.do("stayer", http.MethodGet, board+"/entities/expense/records", nil, nil); len(r.body["items"].([]any)) != 2 {
		t.Fatalf("shared records must be kept: %s", r.raw)
	}
}
