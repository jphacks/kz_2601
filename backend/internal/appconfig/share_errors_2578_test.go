package appconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

func TestMapShareErr2578(t *testing.T) {
	other := errors.New("unavailable")
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"forbidden", store.ErrForbidden, http.StatusForbidden, "forbidden"},
		{"wrapped forbidden", fmt.Errorf("tx: %w", store.ErrForbidden), http.StatusForbidden, "forbidden"},
		{"not found", store.ErrNotFound, http.StatusNotFound, "not_found"},
		{"wrapped not found", fmt.Errorf("tx: %w", store.ErrNotFound), http.StatusNotFound, "not_found"},
		{"conflict", &store.ConflictError{Current: 3}, http.StatusConflict, "version_conflict"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ae *apiError
			if !errors.As(mapShareErr(c.err, "listing"), &ae) || ae.status != c.status || ae.code != c.code {
				t.Fatalf("got %+v", ae)
			}
		})
	}
	t.Run("message names what", func(t *testing.T) {
		var ae *apiError
		if !errors.As(mapShareErr(store.ErrForbidden, "share"), &ae) || ae.msg != "only the owner can change this share" {
			t.Fatalf("got %+v", ae)
		}
		if !errors.As(mapShareErr(store.ErrNotFound, "listing"), &ae) || ae.msg != "listing not found" {
			t.Fatalf("got %+v", ae)
		}
	})
	t.Run("other errors pass through", func(t *testing.T) {
		var ae *apiError
		if got := mapShareErr(other, "share"); !errors.Is(got, other) || errors.As(got, &ae) {
			t.Fatalf("got %v", got)
		}
	})
}

// faultStore2578 fails the share-related store calls on demand.
type faultStore2578 struct {
	store.Store
	getApp, appOwner, createShare, getShare, revokeShare error
	badDSL                                               bool
}

func (s *faultStore2578) GetApp(ctx context.Context, uid, appID string) (*store.App, error) {
	if s.getApp != nil {
		return nil, s.getApp
	}
	a, err := s.Store.GetApp(ctx, uid, appID)
	if err == nil && s.badDSL {
		c := *a
		c.DSL, c.CurrentVersion = json.RawMessage(`{"not":"a dsl"}`), 999
		return &c, nil
	}
	return a, err
}

func (s *faultStore2578) AppOwner(ctx context.Context, appID string) (string, error) {
	if s.appOwner != nil {
		return "", s.appOwner
	}
	return s.Store.AppOwner(ctx, appID)
}

func (s *faultStore2578) CreateShare(ctx context.Context, sh store.Share) error {
	if s.createShare != nil {
		return s.createShare
	}
	return s.Store.CreateShare(ctx, sh)
}

func (s *faultStore2578) GetShare(ctx context.Context, token string) (*store.Share, error) {
	if s.getShare != nil {
		return nil, s.getShare
	}
	return s.Store.GetShare(ctx, token)
}

func (s *faultStore2578) RevokeShare(ctx context.Context, token string) error {
	if s.revokeShare != nil {
		return s.revokeShare
	}
	return s.Store.RevokeShare(ctx, token)
}

func newFaultEnv2578(t *testing.T) (*env, *faultStore2578) {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	st := &faultStore2578{Store: store.NewMemory()}
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}, st
}

func TestCreateShareFailures2578(t *testing.T) {
	e, st := newFaultEnv2578(t)
	const app = "/v1/apps/SHARE-2578"
	e.ownApp("u1", app, kakeibo(t))
	share := func(uid, path string) resp {
		t.Helper()
		return e.do(uid, http.MethodPost, path+"/share", nil, nil)
	}

	for name, c := range map[string]struct {
		uid, path string
		code      int
		errCode   string
	}{
		"reserved appId":   {"u1", "/v1/apps/null", http.StatusBadRequest, "bad_request"},
		"unknown app":      {"u1", "/v1/apps/MISSING-2578", http.StatusNotFound, "not_found"},
		"not the owner":    {"u2", app, http.StatusNotFound, "not_found"},
		"blank app":        {"u1", "/v1/apps/BLANK-2578", http.StatusBadRequest, "bad_request"},
		"owner, untouched": {"u1", app, http.StatusCreated, ""},
	} {
		if name == "blank app" {
			e.do("u1", http.MethodGet, "/v1/apps/BLANK-2578/dsl", nil, nil)
		}
		if r := share(c.uid, c.path); r.code != c.code || r.errCode() != c.errCode {
			t.Fatalf("%s: got %d %s", name, r.code, r.raw)
		}
	}

	faults := []struct {
		name string
		set  func()
	}{
		{"GetApp fails", func() { st.getApp = errors.New("injected") }},
		{"stored dsl invalid", func() { st.badDSL = true }},
		{"owner lookup fails", func() { st.appOwner = errors.New("injected") }},
		{"CreateShare fails", func() { st.createShare = errors.New("injected") }},
	}
	for _, f := range faults {
		t.Run(f.name, func(t *testing.T) {
			f.set()
			defer func() { *st = faultStore2578{Store: st.Store} }()
			if r := share("u1", app); r.code != http.StatusInternalServerError || r.errCode() != "internal" {
				t.Fatalf("got %d %s", r.code, r.raw)
			}
		})
	}
}

func TestRevokeShareFailures2578(t *testing.T) {
	e, st := newFaultEnv2578(t)
	const app = "/v1/apps/REVOKE-2578"
	e.ownApp("u1", app, kakeibo(t))
	tok, _ := e.do("u1", http.MethodPost, app+"/share", nil, nil).body["token"].(string)
	if tok == "" {
		t.Fatal("no share token")
	}
	revoke := func(uid, tok string) resp {
		t.Helper()
		return e.do(uid, http.MethodDelete, "/v1/shares/"+tok, nil, nil)
	}

	for name, c := range map[string]struct {
		uid, tok string
		code     int
		errCode  string
	}{
		"dots only":     {"u1", "...", http.StatusBadRequest, "bad_request"},
		"invalid token": {"u1", "a%20b", http.StatusBadRequest, "bad_request"},
		"unknown token": {"u1", "MISSING-2578", http.StatusNotFound, "not_found"},
		"not the owner": {"u2", tok, http.StatusForbidden, "forbidden"},
	} {
		if r := revoke(c.uid, c.tok); r.code != c.code || r.errCode() != c.errCode {
			t.Fatalf("%s: got %d %s", name, r.code, r.raw)
		}
	}
	if r := e.do("u2", http.MethodGet, "/v1/shares/"+tok, nil, nil); r.code != http.StatusOK {
		t.Fatalf("refused revokes must keep the link: %d %s", r.code, r.raw)
	}

	st.getShare = errors.New("injected")
	if r := revoke("u1", tok); r.code != http.StatusInternalServerError || r.errCode() != "internal" {
		t.Fatalf("GetShare fails: %d %s", r.code, r.raw)
	}
	st.getShare, st.revokeShare = nil, errors.New("injected")
	if r := revoke("u1", tok); r.code != http.StatusInternalServerError || r.errCode() != "internal" {
		t.Fatalf("RevokeShare fails: %d %s", r.code, r.raw)
	}
	st.revokeShare = store.ErrNotFound
	if r := revoke("u1", tok); r.code != http.StatusNotFound || r.errCode() != "not_found" {
		t.Fatalf("RevokeShare not found: %d %s", r.code, r.raw)
	}
	st.revokeShare = nil

	// Revoking twice is 204 both times; the link stops working.
	for i := range 2 {
		if r := revoke("u1", tok); r.code != http.StatusNoContent {
			t.Fatalf("revoke %d: %d %s", i, r.code, r.raw)
		}
	}
	if r := e.do("u2", http.MethodGet, "/v1/shares/"+tok, nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("revoked link: %d %s", r.code, r.raw)
	}
	if r := revoke("u2", tok); r.code != http.StatusForbidden {
		t.Fatalf("revoked link by someone else: %d %s", r.code, r.raw)
	}
}
