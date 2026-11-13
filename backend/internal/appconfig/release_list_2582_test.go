package appconfig

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// release2582Store fails GetApp / ListReleaseRequests when set.
type release2582Store struct {
	store.Store
	getApp, list error
}

func (s *release2582Store) GetApp(ctx context.Context, uid, appID string) (*store.App, error) {
	if s.getApp != nil {
		return nil, s.getApp
	}
	return s.Store.GetApp(ctx, uid, appID)
}

func (s *release2582Store) ListReleaseRequests(ctx context.Context, uid, appID string) ([]store.ReleaseRequest, error) {
	if s.list != nil {
		return nil, s.list
	}
	return s.Store.ListReleaseRequests(ctx, uid, appID)
}

func newRelease2582Env(t *testing.T) (*env, *release2582Store) {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	st := &release2582Store{Store: store.NewMemory()}
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}, st
}

func want2582(t *testing.T, what string, r resp, code int, errCode string) {
	t.Helper()
	if r.code != code || r.errCode() != errCode {
		t.Fatalf("%s: want %d %s, got %d %s", what, code, errCode, r.code, r.raw)
	}
}

func items2582(t *testing.T, what string, r resp) []any {
	t.Helper()
	items, ok := r.body["items"].([]any)
	if r.code != http.StatusOK || !ok {
		t.Fatalf("%s: want 200 with items, got %d %s", what, r.code, r.raw)
	}
	return items
}

// #2582: an app without requests and a caller without any list [] (not null).
func TestRelease2582NoRequests(t *testing.T) {
	e, _ := newRelease2582Env(t)
	const app = "/v1/apps/R2582-EMPTY"
	e.ownApp("owner", app, kakeibo(t))
	if got := items2582(t, "app", e.do("owner", http.MethodGet, app+"/release-requests", nil, nil)); len(got) != 0 {
		t.Fatalf("app: %v", got)
	}
	if got := items2582(t, "mine", e.do("owner", http.MethodGet, "/v1/release-requests", nil, nil)); len(got) != 0 {
		t.Fatalf("mine: %v", got)
	}
	if got := items2582(t, "nobody", e.do("nobody", http.MethodGet, "/v1/release-requests", nil, nil)); len(got) != 0 {
		t.Fatalf("nobody: %v", got)
	}
}

// #2582: an invalid appId is 400; a missing app or someone else's is 404 and
// shows nothing of the owner's requests.
func TestRelease2582NotOwner(t *testing.T) {
	e, _ := newRelease2582Env(t)
	const app = "/v1/apps/R2582-OWNED"
	e.ownApp("owner", app, kakeibo(t))
	if r := e.do("owner", http.MethodPost, app+"/release-requests", `{"purpose":"毎日使う"}`, nil); r.code != http.StatusCreated {
		t.Fatalf("create: %d %s", r.code, r.raw)
	}
	want2582(t, "invalid appId", e.do("owner", http.MethodGet, "/v1/apps/undefined/release-requests", nil, nil), http.StatusBadRequest, httpx.CodeBadRequest)
	want2582(t, "missing app", e.do("owner", http.MethodGet, "/v1/apps/R2582-NONE/release-requests", nil, nil), http.StatusNotFound, httpx.CodeNotFound)
	want2582(t, "not the owner", e.do("other", http.MethodGet, app+"/release-requests", nil, nil), http.StatusNotFound, httpx.CodeNotFound)
	// another user who opened the same appId has a copy of their own, without the owner's request
	e.do("other", http.MethodGet, app+"/dsl", nil, nil)
	if got := items2582(t, "other's copy", e.do("other", http.MethodGet, app+"/release-requests", nil, nil)); len(got) != 0 {
		t.Fatalf("other's copy: %v", got)
	}
	if got := items2582(t, "owner", e.do("owner", http.MethodGet, app+"/release-requests", nil, nil)); len(got) != 1 {
		t.Fatalf("owner: %v", got)
	}
}

// #2582: store failures are 500.
func TestRelease2582StoreErrors(t *testing.T) {
	boom := errors.New("injected store failure")
	e, st := newRelease2582Env(t)
	const app = "/v1/apps/R2582-ERR"
	e.ownApp("owner", app, kakeibo(t))

	st.getApp = boom
	want2582(t, "GetApp", e.do("owner", http.MethodGet, app+"/release-requests", nil, nil), http.StatusInternalServerError, httpx.CodeInternal)
	st.getApp = nil

	st.list = boom
	want2582(t, "list of app", e.do("owner", http.MethodGet, app+"/release-requests", nil, nil), http.StatusInternalServerError, httpx.CodeInternal)
	want2582(t, "list of mine", e.do("owner", http.MethodGet, "/v1/release-requests", nil, nil), http.StatusInternalServerError, httpx.CodeInternal)
}
