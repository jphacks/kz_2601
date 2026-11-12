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

var errMembers2581 = errors.New("injected store failure")

// members2581Store fails the n-th AppOwner / GetMember call (1-based, 0 = never)
// and every ListMembers / SetMemberAdmin call when set.
type members2581Store struct {
	store.Store
	ownerFailAt, memberFailAt int
	ownerCalls, memberCalls   int
	listErr, setAdminErr      error
}

func (s *members2581Store) reset() {
	s.ownerFailAt, s.memberFailAt, s.ownerCalls, s.memberCalls = 0, 0, 0, 0
	s.listErr, s.setAdminErr = nil, nil
}

func (s *members2581Store) AppOwner(ctx context.Context, appID string) (string, error) {
	s.ownerCalls++
	if s.ownerCalls == s.ownerFailAt {
		return "", errMembers2581
	}
	return s.Store.AppOwner(ctx, appID)
}

func (s *members2581Store) GetMember(ctx context.Context, appID, uid string) (*store.Member, error) {
	s.memberCalls++
	if s.memberCalls == s.memberFailAt {
		return nil, errMembers2581
	}
	return s.Store.GetMember(ctx, appID, uid)
}

func (s *members2581Store) ListMembers(ctx context.Context, appID string, limit int) ([]store.Member, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.Store.ListMembers(ctx, appID, limit)
}

func (s *members2581Store) SetMemberAdmin(ctx context.Context, appID, uid string, admin bool) error {
	if s.setAdminErr != nil {
		return s.setAdminErr
	}
	return s.Store.SetMemberAdmin(ctx, appID, uid, admin)
}

// newMembers2581Env makes a family app owned by "owner" that "mom" joined.
func newMembers2581Env(t *testing.T, id string) (*env, *members2581Store, string) {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	st := &members2581Store{Store: store.NewMemory()}
	e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}
	app := "/v1/apps/" + id
	e.ownApp("owner", app, familyKakeibo(t))
	tok := e.share("owner", app)
	if r := e.do("mom", http.MethodPost, "/v1/shares/"+tok+"/import", `{"displayName":"ママ"}`, nil); r.code != http.StatusCreated || r.body["member"] != true {
		t.Fatalf("mom joins: %d %s", r.code, r.raw)
	}
	st.reset()
	return e, st, app
}

func want2581(t *testing.T, what string, r resp, code int, errCode string) {
	t.Helper()
	if r.code != code || r.errCode() != errCode {
		t.Fatalf("%s: want %d %s, got %d %s", what, code, errCode, r.code, r.raw)
	}
}

// #2581: listMembers refuses an invalid appId and people who did not join.
func TestMembers2581ListRefused(t *testing.T) {
	e, _, app := newMembers2581Env(t, "M2581-LIST")
	want2581(t, "invalid appId", e.do("mom", http.MethodGet, "/v1/apps/undefined/members", nil, nil), http.StatusBadRequest, httpx.CodeBadRequest)
	wantNotMember(t, e.do("stranger", http.MethodGet, app+"/members", nil, nil), "stranger")
	wantNotMember(t, e.do("mom", http.MethodGet, "/v1/apps/M2581-NONE/members", nil, nil), "unknown app")
}

// #2581: the creator also recorded as a member is listed once, as owner.
func TestMembers2581OwnerListedOnce(t *testing.T) {
	e, st, app := newMembers2581Env(t, "M2581-ONCE")
	if _, err := st.AddMember(t.Context(), "M2581-ONCE", store.Member{UID: "owner", Via: "share"}); err != nil {
		t.Fatal(err)
	}
	r := e.do("owner", http.MethodGet, app+"/members", nil, nil)
	items, _ := r.body["items"].([]any)
	if r.code != http.StatusOK || len(items) != 2 || r.body["role"] != "owner" || r.body["admin"] != true {
		t.Fatalf("members: %d %s", r.code, r.raw)
	}
	first, second := items[0].(map[string]any), items[1].(map[string]any)
	if first["uid"] != "owner" || first["role"] != "owner" || first["you"] != true || second["uid"] != "mom" || second["admin"] != false {
		t.Fatalf("items: %s", r.raw)
	}
	// a plain member is not an admin
	if r = e.do("mom", http.MethodGet, app+"/members", nil, nil); r.code != http.StatusOK || r.body["role"] != "member" || r.body["admin"] != false {
		t.Fatalf("mom: %d %s", r.code, r.raw)
	}
}

// #2581: store failures in listMembers are 500.
func TestMembers2581ListStoreErrors(t *testing.T) {
	e, st, app := newMembers2581Env(t, "M2581-LERR")
	for _, c := range []struct {
		name string
		set  func()
	}{
		{"AppOwner in memberRole", func() { st.ownerFailAt = 1 }},
		{"GetMember in memberRole", func() { st.memberFailAt = 1 }},
		{"AppOwner after memberRole", func() { st.ownerFailAt = 2 }},
		{"ListMembers", func() { st.listErr = errMembers2581 }},
	} {
		c.set()
		r := e.do("mom", http.MethodGet, app+"/members", nil, nil)
		st.reset()
		want2581(t, c.name, r, http.StatusInternalServerError, httpx.CodeInternal)
	}
}

// #2581: bad requests to patchMember.
func TestMembers2581PatchBadRequests(t *testing.T) {
	e, _, app := newMembers2581Env(t, "M2581-PBAD")
	for _, c := range []struct {
		name, path string
		body       any
	}{
		{"invalid appId", "/v1/apps/undefined/members/mom", `{"admin":true}`},
		{"invalid uid", app + "/members/a%20b", `{"admin":true}`},
		{"not json", app + "/members/mom", `{`},
		{"unknown field", app + "/members/mom", `{"admin":true,"role":"owner"}`},
		{"admin missing", app + "/members/mom", `{}`},
		{"admin not bool", app + "/members/mom", `{"admin":"yes"}`},
	} {
		want2581(t, c.name, e.do("owner", http.MethodPatch, c.path, c.body, nil), http.StatusBadRequest, httpx.CodeBadRequest)
	}
	// nothing changed
	r := e.do("owner", http.MethodGet, app+"/members", nil, nil)
	if mom := r.body["items"].([]any)[1].(map[string]any); mom["admin"] != false {
		t.Fatalf("mom after bad requests: %s", r.raw)
	}
}

// #2581: who may patch: not a participant 403 not_member, a member 403
// admins_only, a missing participant 404, the creator 400.
func TestMembers2581PatchRefused(t *testing.T) {
	e, _, app := newMembers2581Env(t, "M2581-PREF")
	wantNotMember(t, e.do("stranger", http.MethodPatch, app+"/members/mom", `{"admin":true}`, nil), "stranger")
	wantAdminsOnly(t, e.do("mom", http.MethodPatch, app+"/members/me", `{"admin":true}`, nil), "mom promotes herself")
	want2581(t, "missing participant", e.do("owner", http.MethodPatch, app+"/members/nobody", `{"admin":true}`, nil), http.StatusNotFound, httpx.CodeNotFound)
	want2581(t, "creator via me", e.do("owner", http.MethodPatch, app+"/members/me", `{"admin":false}`, nil), http.StatusBadRequest, httpx.CodeBadRequest)
	if r := e.do("owner", http.MethodPatch, app+"/members/mom", `{"admin":true}`, nil); r.code != http.StatusOK || r.body["uid"] != "mom" || r.body["admin"] != true {
		t.Fatalf("promote: %d %s", r.code, r.raw)
	}
}

// #2581: store failures in patchMember are 500 and change nothing.
func TestMembers2581PatchStoreErrors(t *testing.T) {
	e, st, app := newMembers2581Env(t, "M2581-PERR")
	for _, c := range []struct {
		name, uid string
		set       func()
	}{
		{"AppOwner in isAdmin", "owner", func() { st.ownerFailAt = 1 }},
		{"GetMember in isAdmin", "mom", func() { st.memberFailAt = 1 }},
		{"GetMember in memberRole", "mom", func() { st.memberFailAt = 2 }},
		{"AppOwner after isAdmin", "owner", func() { st.ownerFailAt = 2 }},
		{"SetMemberAdmin", "owner", func() { st.setAdminErr = errMembers2581 }},
	} {
		c.set()
		r := e.do(c.uid, http.MethodPatch, app+"/members/mom", `{"admin":true}`, nil)
		st.reset()
		want2581(t, c.name, r, http.StatusInternalServerError, httpx.CodeInternal)
	}
	r := e.do("owner", http.MethodGet, app+"/members", nil, nil)
	if mom := r.body["items"].([]any)[1].(map[string]any); mom["admin"] != false {
		t.Fatalf("mom after failures: %s", r.raw)
	}
}
