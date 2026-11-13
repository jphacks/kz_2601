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

// faultStore2575 fails the store calls of restore / getEntity on demand.
type faultStore2575 struct {
	store.Store
	getApp, getVersion, commit error
	badVersionDSL              bool
}

func (s *faultStore2575) GetApp(ctx context.Context, uid, appID string) (*store.App, error) {
	if s.getApp != nil {
		return nil, s.getApp
	}
	return s.Store.GetApp(ctx, uid, appID)
}

func (s *faultStore2575) GetVersion(ctx context.Context, uid, appID string, n int) (*store.Version, error) {
	if s.getVersion != nil {
		return nil, s.getVersion
	}
	v, err := s.Store.GetVersion(ctx, uid, appID, n)
	if err == nil && s.badVersionDSL {
		c := *v
		c.DSL = json.RawMessage(`{"not":"a dsl"}`)
		return &c, nil
	}
	return v, err
}

func (s *faultStore2575) CommitVersion(ctx context.Context, uid, appID string, c store.Commit) (*store.Version, error) {
	if s.commit != nil {
		return nil, s.commit
	}
	return s.Store.CommitVersion(ctx, uid, appID, c)
}

func newFaultEnv2575(t *testing.T) (*env, *faultStore2575) {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	st := &faultStore2575{Store: store.NewMemory()}
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}, st
}

func TestRestoreVersionFailures2575(t *testing.T) {
	e, st := newFaultEnv2575(t)
	const app = "/v1/apps/RESTORE-2575"
	e.ownApp("u1", app, kakeibo(t))
	restore := func(uid, path string) resp {
		t.Helper()
		return e.do(uid, http.MethodPost, path, nil, nil)
	}

	cases := []struct {
		name, uid, path string
		code            int
		errCode         string
	}{
		{"invalid appId", "u1", "/v1/apps/" + strings.Repeat("x", 129) + "/versions/1/restore", http.StatusBadRequest, "bad_request"},
		{"version zero", "u1", app + "/versions/0/restore", http.StatusBadRequest, "bad_request"},
		{"version not a number", "u1", app + "/versions/abc/restore", http.StatusBadRequest, "bad_request"},
		{"no such version", "u1", app + "/versions/9/restore", http.StatusNotFound, "not_found"},
		{"unknown app", "u1", "/v1/apps/NEW-2575/versions/2/restore", http.StatusNotFound, "not_found"},
		{"not the owner", "u2", app + "/versions/2/restore", http.StatusNotFound, "not_found"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := restore(c.uid, c.path)
			if r.code != c.code || r.errCode() != c.errCode {
				t.Fatalf("got %d %s", r.code, r.raw)
			}
		})
	}

	// The owner's app is untouched by the failed restores.
	if r := e.do("u1", http.MethodGet, app+"/dsl", nil, nil); r.code != http.StatusOK || r.version() != 2 {
		t.Fatalf("after failures: %d %s", r.code, r.raw)
	}

	faults := []struct {
		name    string
		set     func()
		code    int
		errCode string
	}{
		{"GetApp fails", func() { st.getApp = errors.New("injected") }, http.StatusInternalServerError, "internal"},
		{"GetVersion fails", func() { st.getVersion = errors.New("injected") }, http.StatusInternalServerError, "internal"},
		{"stored version invalid", func() { st.badVersionDSL = true }, http.StatusUnprocessableEntity, "invalid_dsl"},
		{"commit fails", func() { st.commit = errors.New("injected") }, http.StatusInternalServerError, "internal"},
		{"commit conflicts", func() { st.commit = &store.ConflictError{Current: 7} }, http.StatusConflict, "version_conflict"},
	}
	for _, f := range faults {
		t.Run(f.name, func(t *testing.T) {
			*st = faultStore2575{Store: st.Store}
			f.set()
			defer func() { *st = faultStore2575{Store: st.Store} }()
			r := restore("u1", app+"/versions/1/restore")
			if r.code != f.code || r.errCode() != f.errCode {
				t.Fatalf("got %d %s", r.code, r.raw)
			}
		})
	}

	// Still restorable once the store recovers.
	if r := restore("u1", app+"/versions/1/restore"); r.code != http.StatusCreated || r.version() != 3 {
		t.Fatalf("restore after faults: %d %s", r.code, r.raw)
	}
}

func TestGetEntityFailures2575(t *testing.T) {
	e, st := newFaultEnv2575(t)
	const app = "/v1/apps/ENTITY-2575"
	e.ownApp("u1", app, kakeibo(t))

	if r := e.do("u1", http.MethodGet, app+"/entities/expense", nil, nil); r.code != http.StatusOK || r.body["id"] != "expense" {
		t.Fatalf("known entity: %d %s", r.code, r.raw)
	}
	cases := []struct {
		name, uid, path string
		code            int
		errCode         string
	}{
		{"invalid appId", "u1", "/v1/apps/" + strings.Repeat("x", 129) + "/entities/expense", http.StatusBadRequest, "bad_request"},
		{"invalid entityId", "u1", app + "/entities/" + strings.Repeat("e", 129), http.StatusBadRequest, "bad_request"},
		{"unknown entity", "u1", app + "/entities/nope", http.StatusNotFound, "not_found"},
		{"new app has no entity", "u1", "/v1/apps/NEW-2575/entities/expense", http.StatusNotFound, "not_found"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := e.do(c.uid, http.MethodGet, c.path, nil, nil)
			if r.code != c.code || r.errCode() != c.errCode {
				t.Fatalf("got %d %s", r.code, r.raw)
			}
		})
	}

	t.Run("GetApp fails", func(t *testing.T) {
		st.getApp = errors.New("injected")
		defer func() { st.getApp = nil }()
		r := e.do("u1", http.MethodGet, app+"/entities/expense", nil, nil)
		if r.code != http.StatusInternalServerError || r.errCode() != "internal" {
			t.Fatalf("got %d %s", r.code, r.raw)
		}
	})
}
