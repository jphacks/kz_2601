package appconfig

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/plan"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// install2579Store fails the store calls installPublicApp makes when set.
type install2579Store struct {
	store.Store
	getInstall, getApp, updateMeta, recordInstall error
}

func (s *install2579Store) GetInstall(ctx context.Context, id, uid string) (string, error) {
	if s.getInstall != nil {
		return "", s.getInstall
	}
	return s.Store.GetInstall(ctx, id, uid)
}

func (s *install2579Store) GetApp(ctx context.Context, uid, appID string) (*store.App, error) {
	if s.getApp != nil {
		return nil, s.getApp
	}
	return s.Store.GetApp(ctx, uid, appID)
}

func (s *install2579Store) UpdateAppMeta(ctx context.Context, uid, appID string, name, status *string) (*store.App, error) {
	if s.updateMeta != nil {
		return nil, s.updateMeta
	}
	return s.Store.UpdateAppMeta(ctx, uid, appID, name, status)
}

func (s *install2579Store) RecordInstall(ctx context.Context, id, uid, appID string, count bool) (bool, error) {
	if s.recordInstall != nil {
		return false, s.recordInstall
	}
	return s.Store.RecordInstall(ctx, id, uid, appID, count)
}

func newInstall2579Env(t *testing.T) (*env, *install2579Store) {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	st := &install2579Store{Store: store.NewMemory()}
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}, st
}

func publish2579(e *env, owner, id string) {
	e.t.Helper()
	e.ownApp(owner, "/v1/apps/"+id, kakeibo(e.t))
	if r := e.do(owner, http.MethodPost, "/v1/apps/"+id+"/publish", `{}`, nil); r.code != http.StatusOK {
		e.t.Fatalf("publish %s: %d %s", id, r.code, r.raw)
	}
}

// #2579: a missing, invalid or hidden listing is refused before anything is copied.
func TestInstallPublic2579MissingOrHidden(t *testing.T) {
	e, st := newInstall2579Env(t)
	if r := e.do("finder", http.MethodPost, "/v1/public-apps/NO-SUCH-LISTING/install", nil, nil); r.code != http.StatusNotFound || r.errCode() != httpx.CodeNotFound {
		t.Fatalf("missing: %d %s", r.code, r.raw)
	}
	if r := e.do("finder", http.MethodPost, "/v1/public-apps/bad%20id/install", nil, nil); r.code != http.StatusBadRequest || r.errCode() != httpx.CodeBadRequest {
		t.Fatalf("invalid id: %d %s", r.code, r.raw)
	}
	publish2579(e, "owner", "I2579-HIDDEN")
	if err := st.SetCatalogHidden(t.Context(), "I2579-HIDDEN", true, "不適切な内容のため"); err != nil {
		t.Fatal(err)
	}
	r := e.do("finder", http.MethodPost, "/v1/public-apps/I2579-HIDDEN/install", nil, nil)
	if r.code != http.StatusForbidden || r.errCode() != httpx.CodeUnpublished {
		t.Fatalf("hidden: %d %s", r.code, r.raw)
	}
	if apps := e.do("finder", http.MethodGet, "/v1/apps", nil, nil).body["items"].([]any); len(apps) != 0 {
		t.Fatalf("nothing copied: %v", apps)
	}
}

// #2579: at the held-app limit installing a listing is 402 app_limit_reached.
func TestInstallPublic2579AppLimit(t *testing.T) {
	e := newSlotEnv(t, proSet{}, fakeSlots{}, plan.ModeEnforce)
	publish2579(e, "owner", "I2579-LIMIT")
	e.ownApp("finder", "/v1/apps/I2579-F1", kakeibo(t))
	e.ownApp("finder", "/v1/apps/I2579-F2", kakeibo(t))
	wantAppLimit(t, "install at limit", e.do("finder", http.MethodPost, "/v1/public-apps/I2579-LIMIT/install", nil, nil), 2, 2)
}

// #2579: the publisher installing their own listing gets mine:true, and doing
// it twice returns the same copy (created:false).
func TestInstallPublic2579OwnTwice(t *testing.T) {
	e, _ := newInstall2579Env(t)
	publish2579(e, "owner", "I2579-OWN")
	r1 := e.do("owner", http.MethodPost, "/v1/public-apps/I2579-OWN/install", nil, nil)
	if r1.code != http.StatusCreated || r1.body["mine"] != true || r1.body["created"] != true {
		t.Fatalf("first: %d %s", r1.code, r1.raw)
	}
	r2 := e.do("owner", http.MethodPost, "/v1/public-apps/I2579-OWN/install", nil, nil)
	if r2.code != http.StatusCreated || r2.body["mine"] != true || r2.body["created"] == true || r2.body["id"] != r1.body["id"] {
		t.Fatalf("second: %d %s", r2.code, r2.raw)
	}
}

// #2579: installing again an archived copy brings it back as active.
func TestInstallPublic2579ArchivedCopy(t *testing.T) {
	e, _ := newInstall2579Env(t)
	publish2579(e, "owner", "I2579-ARCH")
	r := e.do("finder", http.MethodPost, "/v1/public-apps/I2579-ARCH/install", nil, nil)
	copyID, _ := r.body["id"].(string)
	if r.code != http.StatusCreated || copyID == "" {
		t.Fatalf("install: %d %s", r.code, r.raw)
	}
	if r = e.do("finder", http.MethodPatch, "/v1/apps/"+copyID, `{"status":"archived"}`, nil); r.code != http.StatusOK {
		t.Fatalf("archive: %d %s", r.code, r.raw)
	}
	r = e.do("finder", http.MethodPost, "/v1/public-apps/I2579-ARCH/install", nil, nil)
	if r.code != http.StatusCreated || r.body["id"] != copyID || r.body["status"] != "active" || r.body["created"] == true {
		t.Fatalf("again: %d %s", r.code, r.raw)
	}
}

// #2579: store failures other than not-found are 500, and a failed install
// record still returns the copy.
func TestInstallPublic2579StoreErrors(t *testing.T) {
	boom := errors.New("injected store failure")
	e, st := newInstall2579Env(t)
	publish2579(e, "owner", "I2579-ERR")
	const path = "/v1/public-apps/I2579-ERR/install"

	st.getInstall = boom
	if r := e.do("finder", http.MethodPost, path, nil, nil); r.code != http.StatusInternalServerError || r.errCode() != httpx.CodeInternal {
		t.Fatalf("GetInstall error: %d %s", r.code, r.raw)
	}
	st.getInstall = nil

	st.recordInstall = boom
	r := e.do("finder", http.MethodPost, path, nil, nil)
	if r.code != http.StatusCreated || r.body["created"] != true {
		t.Fatalf("RecordInstall error is only logged: %d %s", r.code, r.raw)
	}
	st.recordInstall = nil
	// the install was not recorded, so record it now for the next steps
	r = e.do("finder", http.MethodPost, path, nil, nil)
	copyID, _ := r.body["id"].(string)
	if r.code != http.StatusCreated || copyID == "" {
		t.Fatalf("install: %d %s", r.code, r.raw)
	}

	st.getApp = boom
	if r = e.do("finder", http.MethodPost, path, nil, nil); r.code != http.StatusInternalServerError || r.errCode() != httpx.CodeInternal {
		t.Fatalf("GetApp error: %d %s", r.code, r.raw)
	}
	st.getApp = nil

	if r = e.do("finder", http.MethodPatch, "/v1/apps/"+copyID, `{"status":"archived"}`, nil); r.code != http.StatusOK {
		t.Fatalf("archive: %d %s", r.code, r.raw)
	}
	st.updateMeta = boom
	if r = e.do("finder", http.MethodPost, path, nil, nil); r.code != http.StatusInternalServerError || r.errCode() != httpx.CodeInternal {
		t.Fatalf("reactivate error: %d %s", r.code, r.raw)
	}
}
