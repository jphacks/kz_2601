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

// reactivateFaultStore counts UpdateAppMeta and fails it on demand.
type reactivateFaultStore struct {
	store.Store
	fail  bool
	calls int
}

var errReactivateDown = errors.New("injected reactivate failure")

func (s *reactivateFaultStore) UpdateAppMeta(ctx context.Context, uid, appID string, name, status *string) (*store.App, error) {
	s.calls++
	if s.fail {
		return nil, errReactivateDown
	}
	return s.Store.UpdateAppMeta(ctx, uid, appID, name, status)
}

// #2572: getting a joined app again un-archives only the caller's copy, and
// only when it is archived; the failures leave it as it was.
func TestReactivateFailurePaths(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	mem := store.NewMemory()
	st := &reactivateFaultStore{Store: mem}
	e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}
	const id = "REACT-1"
	app := "/v1/apps/" + id
	e.ownApp("owner", app, sharedKakeibo(t))
	r := e.do("owner", http.MethodPost, app+"/share", nil, nil)
	tok, _ := r.body["token"].(string)
	if r.code != http.StatusCreated || tok == "" {
		t.Fatalf("share: %d %s", r.code, r.raw)
	}
	importPath := "/v1/shares/" + tok + "/import"
	status := func(uid string) string {
		t.Helper()
		a, err := mem.GetApp(t.Context(), uid, id)
		if err != nil {
			t.Fatal(err)
		}
		return a.Status
	}
	archive := func(uid string) {
		t.Helper()
		if r := e.do(uid, http.MethodPatch, app, `{"status":"archived"}`, nil); r.code != http.StatusOK || r.body["status"] != "archived" {
			t.Fatalf("%s archives: %d %s", uid, r.code, r.raw)
		}
	}
	if r := e.do("joiner", http.MethodPost, importPath, nil, nil); r.code != http.StatusCreated || r.body["joined"] != true || r.body["created"] != true {
		t.Fatalf("first import: %d %s", r.code, r.raw)
	}

	// not archived: nothing to un-archive, no write
	st.calls = 0
	if r := e.do("joiner", http.MethodPost, importPath, nil, nil); r.code != http.StatusCreated || r.body["created"] != false || r.body["status"] != "active" {
		t.Fatalf("import of an active copy: %d %s", r.code, r.raw)
	}
	if st.calls != 0 {
		t.Errorf("active copy rewritten: %d calls", st.calls)
	}

	// archived + a store error: 500 that hides the cause, still archived
	archive("joiner")
	st.fail = true
	r = e.do("joiner", http.MethodPost, importPath, nil, nil)
	st.fail = false
	if r.code != http.StatusInternalServerError || r.errCode() != "internal" || strings.Contains(string(r.raw), errReactivateDown.Error()) {
		t.Errorf("store error: %d %s", r.code, r.raw)
	}
	if s := status("joiner"); s != "archived" {
		t.Errorf("after the failure: %q", s)
	}

	// the owner's archived copy is not the joiner's to bring back
	archive("owner")
	st.calls = 0
	if r := e.do("joiner", http.MethodPost, importPath, nil, nil); r.code != http.StatusCreated || r.body["status"] != "active" || r.body["created"] != false {
		t.Fatalf("reactivate: %d %s", r.code, r.raw)
	}
	if st.calls != 1 || status("joiner") != "active" || status("owner") != "archived" {
		t.Errorf("calls=%d joiner=%q owner=%q", st.calls, status("joiner"), status("owner"))
	}

	// unknown or revoked token: 404 and the archived copy stays archived
	archive("joiner")
	if r := e.do("joiner", http.MethodPost, "/v1/shares/tok_unknown_reactivate/import", nil, nil); r.code != http.StatusNotFound || r.errCode() != "not_found" {
		t.Errorf("unknown token: %d %s", r.code, r.raw)
	}
	if r := e.do("owner", http.MethodDelete, "/v1/shares/"+tok, nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("revoke: %d %s", r.code, r.raw)
	}
	if r := e.do("joiner", http.MethodPost, importPath, nil, nil); r.code != http.StatusNotFound || r.errCode() != "not_found" {
		t.Errorf("revoked token: %d %s", r.code, r.raw)
	}
	if s := status("joiner"); s != "archived" {
		t.Errorf("after a rejected import: %q", s)
	}
}

// #2572: installing a listing again goes through the same reactivate.
func TestReactivateOnReinstallFailurePaths(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	mem := store.NewMemory()
	st := &reactivateFaultStore{Store: mem}
	e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}
	const id = "REACT-PUB"
	app := "/v1/apps/" + id
	e.ownApp("owner", app, sharedKakeibo(t))
	if r := e.do("owner", http.MethodPost, app+"/publish", nil, nil); r.code != http.StatusOK {
		t.Fatalf("publish: %d %s", r.code, r.raw)
	}
	install := "/v1/public-apps/" + id + "/install"
	r := e.do("installer", http.MethodPost, install, nil, nil)
	if r.code != http.StatusCreated || r.body["id"] != id {
		t.Fatalf("install: %d %s", r.code, r.raw)
	}
	if r := e.do("installer", http.MethodPatch, app, `{"status":"archived"}`, nil); r.code != http.StatusOK {
		t.Fatalf("archive: %d %s", r.code, r.raw)
	}

	st.fail = true
	r = e.do("installer", http.MethodPost, install, nil, nil)
	st.fail = false
	if r.code != http.StatusInternalServerError || r.errCode() != "internal" {
		t.Errorf("store error: %d %s", r.code, r.raw)
	}
	if a, err := mem.GetApp(t.Context(), "installer", id); err != nil || a.Status != "archived" {
		t.Errorf("after the failure: %+v %v", a, err)
	}

	if r := e.do("installer", http.MethodPost, install, nil, nil); r.code != http.StatusCreated || r.body["status"] != "active" || r.body["id"] != id {
		t.Errorf("reinstall: %d %s", r.code, r.raw)
	}
	if r := e.do("installer", http.MethodPost, "/v1/public-apps/REACT-NONE/install", nil, nil); r.code != http.StatusNotFound || r.errCode() != "not_found" {
		t.Errorf("unknown listing: %d %s", r.code, r.raw)
	}
}
