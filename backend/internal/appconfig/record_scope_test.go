package appconfig

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// editCopy rewrites uid's own copy of appPath to dsl (opening the app first).
func (e *env) editCopy(uid, appPath string, dsl []byte) {
	e.t.Helper()
	r := e.do(uid, http.MethodGet, appPath+"/dsl", nil, nil)
	if r.code != http.StatusOK {
		e.t.Fatalf("open %s: %d %s", appPath, r.code, r.raw)
	}
	if r = e.do(uid, http.MethodPut, appPath+"/dsl", putBody(dsl), map[string]string{"If-Match": `"` + strconv.Itoa(r.version()) + `"`}); r.code != http.StatusCreated {
		e.t.Fatalf("edit %s: %d %s", appPath, r.code, r.raw)
	}
}

// withExtraEntity adds a copy of entity "expense" as id to raw.
func withExtraEntity(t *testing.T, raw []byte, id string) []byte {
	t.Helper()
	var a map[string]any
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatal(err)
	}
	ents := a["entities"].([]any)
	b, _ := json.Marshal(ents[0])
	var c map[string]any
	_ = json.Unmarshal(b, &c)
	c["id"] = id
	a["entities"] = append(ents, c)
	out, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Who may update a public record follows the creator's published DSL, not
// the caller's own copy of it.
func TestRecordEditFollowsPublishedDSL(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/SCOPE-EDIT-1"
	recs := app + "/entities/expense/records"
	e.ownApp("owner", app, sharedKakeibo(t))
	r := e.do("owner", http.MethodPost, recs, `{"data":{"amount":100,"memo":"owner"}}`, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("owner post: %d %s", r.code, r.raw)
	}
	id := r.body["id"].(string)

	e.editCopy("anon1", app, membersKakeibo(t))
	if r = e.do("anon1", http.MethodPatch, recs+"/"+id, `{"data":{"amount":1}}`, nil); r.code != http.StatusForbidden {
		t.Fatalf("patch of another's record: %d %s", r.code, r.raw)
	}
	if r = e.do("anon1", http.MethodDelete, recs+"/"+id, nil, nil); r.code != http.StatusForbidden {
		t.Fatalf("delete of another's record: %d %s", r.code, r.raw)
	}
	if r = e.do("owner", http.MethodGet, recs+"/"+id, nil, nil); r.code != http.StatusOK || r.body["data"].(map[string]any)["amount"] != float64(100) {
		t.Fatalf("record must be unchanged: %d %s", r.code, r.raw)
	}

	// the caller's own posts stay editable and deletable
	r = e.do("anon1", http.MethodPost, recs, `{"data":{"amount":5}}`, nil)
	if r.code != http.StatusCreated || r.body["author"] != "anon1" {
		t.Fatalf("anon1 post: %d %s", r.code, r.raw)
	}
	mine := r.body["id"].(string)
	if r = e.do("anon1", http.MethodPatch, recs+"/"+mine, `{"data":{"amount":6}}`, nil); r.code != http.StatusOK {
		t.Fatalf("anon1 patch own: %d %s", r.code, r.raw)
	}
	if r = e.do("anon1", http.MethodDelete, recs+"/"+mine, nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("anon1 delete own: %d %s", r.code, r.raw)
	}
}

// The creator's edit:members still lets everyone update, whatever their copy says.
func TestRecordEditMembersFromPublishedDSL(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/SCOPE-EDIT-2"
	recs := app + "/entities/expense/records"
	e.ownApp("owner", app, membersKakeibo(t))
	r := e.do("owner", http.MethodPost, recs, `{"data":{"amount":100}}`, nil)
	id := r.body["id"].(string)

	e.editCopy("u2", app, sharedKakeibo(t)) // edit back to the default in u2's copy
	if r = e.do("u2", http.MethodPatch, recs+"/"+id, `{"data":{"amount":120}}`, nil); r.code != http.StatusOK || r.body["author"] != "owner" {
		t.Fatalf("members patch: %d %s", r.code, r.raw)
	}
	if r = e.do("u2", http.MethodDelete, recs+"/"+id, nil, nil); r.code != http.StatusForbidden {
		t.Fatalf("members delete: %d %s", r.code, r.raw)
	}
}

// Participation in a scope:"shared" entity follows the creator's published
// DSL: a copy that calls the entity public does not skip it.
func TestSharedScopeFollowsPublishedDSL(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/SCOPE-SHARED-1"
	recs := app + "/entities/expense/records"
	e.ownApp("owner", app, familyKakeibo(t))
	r := e.do("owner", http.MethodPost, recs, `{"data":{"amount":100}}`, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("owner post: %d %s", r.code, r.raw)
	}
	id := r.body["id"].(string)

	e.editCopy("stranger", app, sharedKakeibo(t))
	wantNotMember(t, e.do("stranger", http.MethodGet, recs, nil, nil), "list")
	wantNotMember(t, e.do("stranger", http.MethodGet, recs+"/"+id, nil, nil), "get")
	wantNotMember(t, e.do("stranger", http.MethodPost, recs, `{"data":{"amount":1}}`, nil), "post")

	e.editCopy("stranger2", app, membersKakeibo(t))
	wantNotMember(t, e.do("stranger2", http.MethodPatch, recs+"/"+id, `{"data":{"amount":1}}`, nil), "patch")
}

// An entity the creator did not share stays in the caller's own tree.
func TestUnpublishedEntityStaysPrivate(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/SCOPE-PRIV-1"
	e.ownApp("owner", app, sharedKakeibo(t))
	both := withExtraEntity(t, sharedKakeibo(t), "expense2") // a copy that adds its own public entity
	e.editCopy("u2", app, both)
	recs := app + "/entities/expense2/records"
	if r := e.do("u2", http.MethodPost, recs, `{"data":{"amount":1}}`, nil); r.code != http.StatusCreated || r.body["author"] != nil {
		t.Fatalf("u2 post: %d %s", r.code, r.raw)
	}
	e.editCopy("u3", app, both)
	if r := e.do("u3", http.MethodGet, recs, nil, nil); r.code != http.StatusOK || len(r.body["items"].([]any)) != 0 {
		t.Fatalf("u3 must not see u2's records: %d %s", r.code, r.raw)
	}
}

// The events stream skips shared entities by the published DSL too.
func TestEventsSharingFollowsPublishedDSL(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	t.Setenv("EVENTS_MAX_STREAM", "10s")
	srv := httptest.NewServer(auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory())))
	defer srv.Close()
	e := &env{t: t, h: srv.Config.Handler}
	const appID = "SCOPE-EVENTS-1"
	e.ownApp("owner", "/v1/apps/"+appID, familyKakeibo(t))
	e.editCopy("stranger", "/v1/apps/"+appID, sharedKakeibo(t))

	events, stop := openEvents(t, srv, "stranger", appID)
	hello := next(t, events)
	stop()
	if ents, _ := hello.data["entities"].([]any); hello.name != "hello" || len(ents) != 0 {
		t.Fatalf("stranger hello: %+v", hello)
	}
}

func TestSharingOf(t *testing.T) {
	own := &dsl.Entity{ID: "e", Scope: dsl.ScopePublic, Edit: dsl.EditMembers, Marks: []dsl.Mark{{ID: "x"}}}
	pub := &dsl.App{Entities: []dsl.Entity{{ID: "e", Scope: dsl.ScopePublic, Marks: []dsl.Mark{{ID: "like"}}}}}
	if got := sharingOf(own, pub, "u1", "u1"); got != own {
		t.Fatal("the publisher keeps their own entity")
	}
	if got := sharingOf(own, nil, "", "u2"); got != own {
		t.Fatal("unpublished apps keep the caller's entity")
	}
	got := sharingOf(own, pub, "u1", "u2")
	if got.Scope != dsl.ScopePublic || got.Edit != "" || len(got.Marks) != 1 || got.Marks[0].ID != "like" || own.Edit != dsl.EditMembers {
		t.Fatalf("published sharing: %+v", got)
	}
	if got = sharingOf(&dsl.Entity{ID: "other", Scope: dsl.ScopePublic}, pub, "u1", "u2"); got.IsShared() {
		t.Fatalf("an entity the publisher did not share stays private: %+v", got)
	}
}
