package appconfig

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
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

// A shared entity the creator did not publish is refused.
func TestUnpublishedEntityIsRefused(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/SCOPE-PRIV-1"
	e.ownApp("owner", app, sharedKakeibo(t))
	e.editCopy("u2", app, withExtraEntity(t, sharedKakeibo(t), "expense2"))
	recs := app + "/entities/expense2/records"
	if r := e.do("u2", http.MethodPost, recs, `{"data":{"amount":1}}`, nil); r.code != http.StatusForbidden || r.reason() != notPublishedReason {
		t.Fatalf("u2 post: %d %s", r.code, r.raw)
	}
	if r := e.do("u2", http.MethodPost, app+"/entities/expense/records", `{"data":{"amount":1}}`, nil); r.code != http.StatusCreated {
		t.Fatalf("published entity: %d %s", r.code, r.raw)
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
	e.ownApp("solo", "/v1/apps/SCOPE-EVENTS-2", kakeibo(t)) // nothing published
	e.editCopy("other", "/v1/apps/SCOPE-EVENTS-2", sharedKakeibo(t))

	for _, c := range []struct {
		uid, app string
		want     int
	}{{"stranger", appID, 0}, {"owner", appID, 1}, {"other", "SCOPE-EVENTS-2", 0}, {"solo", "SCOPE-EVENTS-2", 1}} {
		events, stop := openEvents(t, srv, c.uid, c.app)
		hello := next(t, events)
		stop()
		if ents, _ := hello.data["entities"].([]any); hello.name != "hello" || len(ents) != c.want {
			t.Fatalf("%s hello: %+v", c.uid, hello)
		}
	}
}

func withScope(t *testing.T, raw []byte, from, to string) []byte {
	t.Helper()
	if !strings.Contains(string(raw), from) {
		t.Fatalf("dsl has no %s", from)
	}
	return []byte(strings.Replace(string(raw), from, to, 1))
}

// Copies that rename the scope of a shared entity, for outsiders and participants.
func TestSharedScopeCopiesOfParticipants(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		owner, mom, other := "owner"+sfx, "mom"+sfx, "other"+sfx
		app := "/v1/apps/SCOPE-FAMILY" + sfx
		recs := app + "/entities/expense/records"
		e.ownApp(owner, app, familyKakeibo(t))
		r := e.do(owner, http.MethodPost, recs, `{"data":{"amount":100}}`, nil)
		if r.code != http.StatusCreated {
			t.Fatalf("owner post: %d %s", r.code, r.raw)
		}
		rid := r.body["id"].(string)
		tok := e.share(owner, app)
		if r := e.do(mom, http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil); r.code != http.StatusCreated || r.body["member"] != true {
			t.Fatalf("mom import: %d %s", r.code, r.raw)
		}

		for _, scope := range []string{`"scope": "public", "edit": "members"`, `"scope": "private"`} {
			e.editCopy(other, app, withScope(t, familyKakeibo(t), `"scope": "shared"`, scope))
			wantNotMember(t, e.do(other, http.MethodGet, recs, nil, nil), scope+": list")
			wantNotMember(t, e.do(other, http.MethodGet, recs+"/"+rid, nil, nil), scope+": get")
			wantNotMember(t, e.do(other, http.MethodPost, recs, `{"data":{"amount":1}}`, nil), scope+": create")
			wantNotMember(t, e.do(other, http.MethodPatch, recs+"/"+rid, `{"data":{"amount":2}}`, nil), scope+": update")
			wantNotMember(t, e.do(other, http.MethodDelete, recs+"/"+rid, nil, nil), scope+": delete")
			wantNotMember(t, e.do(other, http.MethodDelete, recs+"?confirm=true", nil, nil), scope+": delete all")
			wantNotMember(t, e.do(other, http.MethodPost, app+"/share", nil, nil), scope+": share")
		}

		e.editCopy(mom, app, withScope(t, familyKakeibo(t), `"scope": "shared"`, `"scope": "shared", "edit": "members"`))
		if r := e.do(mom, http.MethodPatch, recs+"/"+rid, `{"data":{"amount":2}}`, nil); r.code != http.StatusForbidden {
			t.Fatalf("mom updates owner's record: %d %s", r.code, r.raw)
		}
		if r := e.do(mom, http.MethodDelete, recs+"/"+rid, nil, nil); r.code != http.StatusForbidden {
			t.Fatalf("mom deletes owner's record: %d %s", r.code, r.raw)
		}
		// a participant whose copy says private still reads the shared records
		e.editCopy(mom, app, withScope(t, familyKakeibo(t), `"scope": "shared"`, `"scope": "private"`))
		if r := e.do(mom, http.MethodGet, recs, nil, nil); r.code != http.StatusOK || len(r.body["items"].([]any)) != 1 {
			t.Fatalf("mom list: %d %s", r.code, r.raw)
		}

		r = e.do(owner, http.MethodGet, recs+"/"+rid, nil, nil)
		if r.code != http.StatusOK || r.body["data"].(map[string]any)["amount"] != float64(100) {
			t.Fatalf("owner's record changed: %d %s", r.code, r.raw)
		}
	})
}

// The creator's later edit:members applies to participants whose copy predates it.
func TestSharedEditFollowsCreatorUpdate(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/SCOPE-EDIT-3"
	recs := app + "/entities/expense/records"
	e.ownApp("owner", app, familyKakeibo(t))
	rid := e.do("owner", http.MethodPost, recs, `{"data":{"amount":100}}`, nil).body["id"].(string)
	tok := e.share("owner", app)
	e.do("mom", http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil)
	if r := e.do("mom", http.MethodPatch, recs+"/"+rid, `{"data":{"amount":2}}`, nil); r.code != http.StatusForbidden {
		t.Fatalf("edit:author: %d %s", r.code, r.raw)
	}
	e.editCopy("owner", app, withScope(t, familyKakeibo(t), `"scope": "shared"`, `"scope": "shared", "edit": "members"`))
	if r := e.do("mom", http.MethodPatch, recs+"/"+rid, `{"data":{"amount":2}}`, nil); r.code != http.StatusOK {
		t.Fatalf("edit:members: %d %s", r.code, r.raw)
	}
}

// Marks are the published entity's: a copy cannot open them to outsiders or
// add marks the creator did not declare.
func TestMarksFollowPublishedDSL(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		owner, mom, other := "mo"+sfx, "mm"+sfx, "mx"+sfx
		app := "/v1/apps/SCOPE-MARKS" + sfx
		recs := app + "/entities/recipes/records"
		shared := withScope(t, recipes(t), `"scope": "public"`, `"scope": "shared"`)
		e.ownApp(owner, app, shared)
		r := e.do(owner, http.MethodPost, recs, `{"data":{"title":"肉じゃが","minutes":30}}`, nil)
		if r.code != http.StatusCreated {
			t.Fatalf("create: %d %s", r.code, r.raw)
		}
		mark := recs + "/" + r.body["id"].(string) + "/marks/"
		tok := e.share(owner, app)
		e.do(mom, http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil)

		e.editCopy(other, app, recipes(t)) // public in the copy
		wantNotMember(t, e.do(other, http.MethodPut, mark+"save", nil, nil), "outsider mark")

		extra := withScope(t, shared, `"marks": [{ "id": "save"`, `"marks": [{ "id": "keep", "icon": "bookmark", "label": "あとで" }, { "id": "save"`)
		e.editCopy(mom, app, extra)
		if r := e.do(mom, http.MethodPut, mark+"keep", nil, nil); r.code != http.StatusNotFound {
			t.Fatalf("undeclared mark: %d %s", r.code, r.raw)
		}
		if r := e.do(mom, http.MethodPut, mark+"save", nil, nil); r.code != http.StatusOK || r.body["count"] != float64(1) {
			t.Fatalf("declared mark: %d %s", r.code, r.raw)
		}
	})
}

// Without the creator's published DSL, a copy that declares an entity shared
// gets no shared data, and does not take over the app by publishing first.
func TestSharedNeedsCreatorsPublishedDSL(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		owner, other := "po"+sfx, "px"+sfx
		app := "/v1/apps/SCOPE-NOPUB" + sfx
		recs := app + "/entities/expense/records"
		e.ownApp(owner, app, kakeibo(t))
		if r := e.do(owner, http.MethodPost, recs, `{"data":{"amount":100}}`, nil); r.code != http.StatusCreated {
			t.Fatalf("owner post: %d %s", r.code, r.raw)
		}
		e.editCopy(other, app, sharedKakeibo(t))
		for _, c := range []struct{ method, path, body string }{
			{http.MethodGet, recs, ""},
			{http.MethodPost, recs, `{"data":{"amount":1}}`},
		} {
			var body any
			if c.body != "" {
				body = c.body
			}
			if r := e.do(other, c.method, c.path, body, nil); r.code != http.StatusForbidden || r.reason() != notPublishedReason {
				t.Fatalf("%s %s: %d %s", c.method, c.path, r.code, r.raw)
			}
		}
		if r := e.do(owner, http.MethodGet, recs, nil, nil); r.code != http.StatusOK || len(r.body["items"].([]any)) != 1 {
			t.Fatalf("owner list: %d %s", r.code, r.raw)
		}
	})
}

// The creator's own copy decides for the creator, also after the published
// DSL stops being updated (no shared entity left).
func TestCreatorCopyDecidesForCreator(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/SCOPE-CREATOR"
	recs := app + "/entities/expense/records"
	e.ownApp("owner", app, familyKakeibo(t))
	e.do("owner", http.MethodPost, recs, `{"data":{"amount":100}}`, nil)
	e.editCopy("owner", app, kakeibo(t)) // private again
	if r := e.do("owner", http.MethodPost, recs, `{"data":{"amount":7}}`, nil); r.code != http.StatusCreated || r.body["author"] != nil {
		t.Fatalf("private post: %d %s", r.code, r.raw)
	}
	if r := e.do("owner", http.MethodGet, recs, nil, nil); r.code != http.StatusOK || len(r.body["items"].([]any)) != 1 {
		t.Fatalf("private list: %d %s", r.code, r.raw)
	}
}
