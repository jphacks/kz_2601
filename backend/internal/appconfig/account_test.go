package appconfig

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

type fakeDeleter struct {
	deleted []string
	err     error
}

func (f *fakeDeleter) DeleteUser(_ context.Context, uid string) error {
	f.deleted = append(f.deleted, uid)
	return f.err
}

func TestDeleteMe(t *testing.T) {
	e := newEnv(t)
	users := &fakeDeleter{}
	e.h = auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory(), WithUserDeleter(users)))

	// leaver: a private app, a shared board, a share link and a catalog listing
	const mine, board = "/v1/apps/LEAVER-KAKEIBO", "/v1/apps/BOARD-1"
	e.ownApp("leaver", mine, kakeibo(t))
	e.do("leaver", http.MethodPost, mine+"/entities/expense/records", `{"data":{"amount":100}}`, nil)
	tok, _ := e.do("leaver", http.MethodPost, mine+"/share", nil, nil).body["token"].(string)
	if r := e.do("leaver", http.MethodPost, mine+"/publish", `{}`, nil); r.code != http.StatusOK {
		t.Fatalf("publish: %d %s", r.code, r.raw)
	}
	// stayer owns the shared board; both post to it
	e.ownApp("stayer", board, sharedKakeibo(t))
	e.ownApp("stayer", "/v1/apps/STAYER-OWN", kakeibo(t))
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
	if len(users.deleted) != 2 || users.deleted[0] != "leaver" {
		t.Fatalf("auth user delete: %v", users.deleted)
	}

	// leaver's data is gone (a fresh GET would re-seed, so check the list)
	if r := e.do("leaver", http.MethodGet, "/v1/apps", nil, nil); len(r.body["items"].([]any)) != 0 {
		t.Fatalf("leaver apps remain: %s", r.raw)
	}
	if r := e.do("stayer", http.MethodGet, "/v1/shares/"+tok, nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("share remains: %d %s", r.code, r.raw)
	}
	if r := e.do("stayer", http.MethodGet, "/v1/public-apps", nil, nil); len(r.body["items"].([]any)) != 0 {
		t.Fatalf("listing remains: %s", r.raw)
	}

	// stayer keeps everything, including their post on the shared board
	if r := e.do("stayer", http.MethodGet, "/v1/apps", nil, nil); len(r.body["items"].([]any)) != 2 {
		t.Fatalf("stayer apps: %s", r.raw)
	}
	r := e.do("stayer", http.MethodGet, board+"/entities/expense/records", nil, nil)
	items, _ := r.body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["author"] != "stayer" {
		t.Fatalf("shared records after delete: %s", r.raw)
	}
}

func TestDeleteMeAuthFailureIsRetryable(t *testing.T) {
	e := newEnv(t)
	users := &fakeDeleter{err: errors.New("permission denied")}
	e.h = auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory(), WithUserDeleter(users)))
	e.ownApp("u1", "/v1/apps/A", kakeibo(t))

	if r := e.do("u1", http.MethodDelete, "/v1/me", nil, nil); r.code != http.StatusInternalServerError {
		t.Fatalf("auth failure must be 500: %d %s", r.code, r.raw)
	}
	users.err = nil
	if r := e.do("u1", http.MethodDelete, "/v1/me", nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("retry: %d %s", r.code, r.raw)
	}
}
