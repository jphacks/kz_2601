package appconfig

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// publishPathFaultStore fails the owner lookup and the publish writes on demand.
type publishPathFaultStore struct {
	store.Store
	failOwner, failMirror, failPut, failDelete bool
}

var errPublishPathDown = errors.New("injected publish failure")

func (s *publishPathFaultStore) AppOwner(ctx context.Context, appID string) (string, error) {
	if s.failOwner {
		return "", errPublishPathDown
	}
	return s.Store.AppOwner(ctx, appID)
}

func (s *publishPathFaultStore) PublishApp(ctx context.Context, uid, appID string, raw json.RawMessage, name string, version int) (bool, error) {
	if s.failMirror {
		return false, errPublishPathDown
	}
	return s.Store.PublishApp(ctx, uid, appID, raw, name, version)
}

func (s *publishPathFaultStore) PutCatalogApp(ctx context.Context, uid string, c store.CatalogApp) (*store.CatalogApp, error) {
	if s.failPut {
		return nil, errPublishPathDown
	}
	return s.Store.PutCatalogApp(ctx, uid, c)
}

func (s *publishPathFaultStore) DeleteCatalogApp(ctx context.Context, uid, id string) error {
	if s.failDelete {
		return errPublishPathDown
	}
	return s.Store.DeleteCatalogApp(ctx, uid, id)
}

func newPublishPathEnv(t *testing.T) (*env, *publishPathFaultStore, *store.Memory) {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	mem := store.NewMemory()
	st := &publishPathFaultStore{Store: mem}
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}, st, mem
}

func createAppBody(id string, dsl []byte) string {
	return `{"id":"` + id + `","dsl":` + string(dsl) + `}`
}

// #2570: POST /v1/apps mirrors a new shared app (handler.go publish). Every
// failure of that mirror is logged, never returned: the app is still created.
func TestCreateAppPublishFailurePaths(t *testing.T) {
	e, st, mem := newPublishPathEnv(t)
	mirror := func(id string) *store.PublicApp {
		t.Helper()
		p, err := mem.GetPublicApp(t.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	create := func(uid, id string, dsl []byte) {
		t.Helper()
		if r := e.do(uid, http.MethodPost, "/v1/apps", createAppBody(id, dsl), nil); r.code != http.StatusCreated {
			t.Fatalf("create %s by %s: %d %s", id, uid, r.code, r.raw)
		}
	}

	// a private-only app has nothing to mirror
	create("owner", "PUBF-PRIVATE", kakeibo(t))
	if p := mirror("PUBF-PRIVATE"); p != nil {
		t.Errorf("private app mirrored: %+v", p)
	}

	// owner lookup fails
	st.failOwner = true
	create("owner", "PUBF-OWNERDOWN", sharedKakeibo(t))
	st.failOwner = false
	if p := mirror("PUBF-OWNERDOWN"); p != nil {
		t.Errorf("mirrored without an owner: %+v", p)
	}

	// the mirror write fails
	st.failMirror = true
	create("owner", "PUBF-MIRRORDOWN", sharedKakeibo(t))
	st.failMirror = false
	if p := mirror("PUBF-MIRRORDOWN"); p != nil {
		t.Errorf("mirrored on a failed write: %+v", p)
	}

	// someone who is not the creator never overwrites the creator's mirror
	create("owner", "PUBF-TAKEN", sharedKakeibo(t))
	other := strings.Replace(string(sharedKakeibo(t)), `"name": "家計簿"`, `"name": "乗っ取り"`, 1)
	create("squatter", "PUBF-TAKEN", []byte(other))
	if p := mirror("PUBF-TAKEN"); p == nil || p.OwnerUID != "owner" || strings.Contains(string(p.DSL), "乗っ取り") {
		t.Errorf("creator's mirror changed: %+v", p)
	}
}

// #2570: the failure paths of POST / DELETE /v1/apps/{appId}/publish.
func TestPublishListingFailurePaths(t *testing.T) {
	e, st, mem := newPublishPathEnv(t)
	const id = "PUBF-LISTING"
	app := "/v1/apps/" + id
	e.ownApp("owner", app, kakeibo(t))
	e.do("other", http.MethodGet, app+"/dsl", nil, nil) // other's seeded copy

	for _, c := range []struct {
		name, uid, method, path string
		code                    int
		errCode                 string
	}{
		{"publish: invalid appId", "owner", http.MethodPost, "/v1/apps/null/publish", http.StatusBadRequest, "bad_request"},
		{"publish: unknown app", "owner", http.MethodPost, "/v1/apps/PUBF-NONE/publish", http.StatusNotFound, "not_found"},
		{"publish: not the creator", "other", http.MethodPost, app + "/publish", http.StatusForbidden, "forbidden"},
		{"unpublish: invalid appId", "owner", http.MethodDelete, "/v1/apps/null/publish", http.StatusBadRequest, "bad_request"},
		{"unpublish: not listed", "owner", http.MethodDelete, app + "/publish", http.StatusNotFound, "not_found"},
	} {
		if r := e.do(c.uid, c.method, c.path, nil, nil); r.code != c.code || r.errCode() != c.errCode {
			t.Errorf("%s: %d %s", c.name, r.code, r.raw)
		}
	}

	// a store error on the listing write is a 500 that hides the cause
	st.failPut = true
	r := e.do("owner", http.MethodPost, app+"/publish", nil, nil)
	st.failPut = false
	if r.code != http.StatusInternalServerError || r.errCode() != "internal" || strings.Contains(string(r.raw), errPublishPathDown.Error()) {
		t.Errorf("publish store error: %d %s", r.code, r.raw)
	}
	// an owner lookup error is a 500 too
	st.failOwner = true
	r = e.do("owner", http.MethodPost, app+"/publish", nil, nil)
	st.failOwner = false
	if r.code != http.StatusInternalServerError || r.errCode() != "internal" {
		t.Errorf("publish owner lookup error: %d %s", r.code, r.raw)
	}

	if r := e.do("owner", http.MethodPost, app+"/publish", nil, nil); r.code != http.StatusOK {
		t.Fatalf("publish: %d %s", r.code, r.raw)
	}

	// an app the operator stopped stays stopped when its owner republishes
	if err := mem.SetCatalogHidden(t.Context(), id, true, "運営が停止"); err != nil {
		t.Fatal(err)
	}
	if r := e.do("owner", http.MethodPost, app+"/publish", `{"name":"再公開"}`, nil); r.code != http.StatusOK || r.body["hidden"] != true || r.body["hiddenReason"] != "運営が停止" {
		t.Errorf("republish a stopped app: %d %s", r.code, r.raw)
	}
	if c, err := mem.GetCatalogApp(t.Context(), id); err != nil || !c.Hidden {
		t.Errorf("stopped listing: %+v %v", c, err)
	}

	// unpublishing: not the owner 403, store error 500, then 204 and 404 again
	if r := e.do("other", http.MethodDelete, app+"/publish", nil, nil); r.code != http.StatusForbidden || r.errCode() != "forbidden" {
		t.Errorf("unpublish by other: %d %s", r.code, r.raw)
	}
	st.failDelete = true
	r = e.do("owner", http.MethodDelete, app+"/publish", nil, nil)
	st.failDelete = false
	if r.code != http.StatusInternalServerError || r.errCode() != "internal" {
		t.Errorf("unpublish store error: %d %s", r.code, r.raw)
	}
	if _, err := mem.GetCatalogApp(t.Context(), id); err != nil {
		t.Errorf("listing gone after a failed unpublish: %v", err)
	}
	if r := e.do("owner", http.MethodDelete, app+"/publish", nil, nil); r.code != http.StatusNoContent {
		t.Errorf("unpublish: %d %s", r.code, r.raw)
	}
	if r := e.do("owner", http.MethodDelete, app+"/publish", nil, nil); r.code != http.StatusNotFound || r.errCode() != "not_found" {
		t.Errorf("unpublish twice: %d %s", r.code, r.raw)
	}
}
