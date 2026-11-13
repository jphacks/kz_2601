package appconfig

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// versionsDownStore fails the version reads (and optionally GetApp) like a
// store that is unavailable.
type versionsDownStore struct {
	store.Store
	failGetApp, failList, failGet bool
}

var errVersionsDown = errors.New("injected versions failure")

func (s *versionsDownStore) GetApp(ctx context.Context, uid, appID string) (*store.App, error) {
	if s.failGetApp {
		return nil, errVersionsDown
	}
	return s.Store.GetApp(ctx, uid, appID)
}

func (s *versionsDownStore) ListVersions(ctx context.Context, uid, appID string, limit, offset int) ([]store.Version, error) {
	if s.failList {
		return nil, errVersionsDown
	}
	return s.Store.ListVersions(ctx, uid, appID, limit, offset)
}

func (s *versionsDownStore) GetVersion(ctx context.Context, uid, appID string, n int) (*store.Version, error) {
	if s.failGet {
		return nil, errVersionsDown
	}
	return s.Store.GetVersion(ctx, uid, appID, n)
}

func versionsErrMessage(r resp) string {
	e, _ := r.body["error"].(map[string]any)
	s, _ := e["message"].(string)
	return s
}

// #2567: the failure paths of GET /versions and GET /versions/{n}.
func TestVersionsReadFailurePaths(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	st := &versionsDownStore{Store: store.NewMemory()}
	e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}
	const app = "/v1/apps/VERSIONS-FAIL-1"
	e.ownApp("owner", app, kakeibo(t))

	for _, c := range []struct {
		name, uid, path string
		code            int
		errCode, msg    string
	}{
		{"list: invalid appId", "owner", "/v1/apps/null/versions", http.StatusBadRequest, "bad_request", "invalid appId"},
		{"list: bad pageSize", "owner", app + "/versions?pageSize=0", http.StatusBadRequest, "bad_request", "pageSize must be a positive integer"},
		{"list: bad pageToken", "owner", app + "/versions?pageToken=%21%21", http.StatusBadRequest, "bad_request", "invalid pageToken"},
		{"get: invalid appId", "owner", "/v1/apps/null/versions/1", http.StatusBadRequest, "bad_request", "invalid appId"},
		{"get: version 0", "owner", app + "/versions/0", http.StatusBadRequest, "bad_request", "version must be a positive integer"},
		{"get: negative version", "owner", app + "/versions/-1", http.StatusBadRequest, "bad_request", "version must be a positive integer"},
		{"get: non-numeric version", "owner", app + "/versions/abc", http.StatusBadRequest, "bad_request", "version must be a positive integer"},
		{"get: unknown version", "owner", app + "/versions/99", http.StatusNotFound, "not_found", "version not found"},
		{"get: unknown app", "owner", "/v1/apps/VERSIONS-NONE/versions/1", http.StatusNotFound, "not_found", "version not found"},
		// another user's tree never holds the owner's versions
		{"get: not the owner", "other", app + "/versions/2", http.StatusNotFound, "not_found", "version not found"},
	} {
		r := e.do(c.uid, http.MethodGet, c.path, nil, nil)
		if r.code != c.code || r.errCode() != c.errCode || !strings.Contains(versionsErrMessage(r), c.msg) {
			t.Errorf("%s: %d %s", c.name, r.code, r.raw)
		}
	}

	// listing an appId one has not got seeds one's own copy: version 1 only,
	// not the owner's history
	r := e.do("other", http.MethodGet, app+"/versions", nil, nil)
	if items, _ := r.body["items"].([]any); r.code != http.StatusOK || len(items) != 1 {
		t.Fatalf("other's list: %d %s", r.code, r.raw)
	}
	if r := e.do("owner", http.MethodGet, app+"/versions", nil, nil); r.code != http.StatusOK {
		t.Fatalf("owner's list: %d %s", r.code, r.raw)
	} else if items, _ := r.body["items"].([]any); len(items) != 2 {
		t.Fatalf("owner's list: %s", r.raw)
	}

	// store errors are 500 internal and do not leak the cause
	for _, c := range []struct {
		name, path string
		set        func(on bool)
	}{
		{"list: GetApp", app + "/versions", func(on bool) { st.failGetApp = on }},
		{"list: ListVersions", app + "/versions", func(on bool) { st.failList = on }},
		{"get: GetVersion", app + "/versions/1", func(on bool) { st.failGet = on }},
	} {
		c.set(true)
		r := e.do("owner", http.MethodGet, c.path, nil, nil)
		c.set(false)
		if r.code != http.StatusInternalServerError || r.errCode() != "internal" || strings.Contains(string(r.raw), errVersionsDown.Error()) {
			t.Errorf("%s: %d %s", c.name, r.code, r.raw)
		}
	}

	// after the store recovers the same reads succeed
	if r := e.do("owner", http.MethodGet, app+"/versions/2", nil, nil); r.code != http.StatusOK || r.version() != 2 {
		t.Fatalf("recovered get: %d %s", r.code, r.raw)
	}
}
