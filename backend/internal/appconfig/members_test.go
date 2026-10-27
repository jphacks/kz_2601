package appconfig

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// familyKakeibo is the kakeibo example with expense as scope:"shared".
func familyKakeibo(t *testing.T) []byte {
	t.Helper()
	return []byte(strings.Replace(string(sharedKakeibo(t)), `"scope": "public"`, `"scope": "shared"`, 1))
}

func (r resp) reason() string {
	d, _ := r.body["error"].(map[string]any)["details"].(map[string]any)
	s, _ := d["reason"].(string)
	return s
}

func wantNotMember(t *testing.T, r resp, what string) {
	t.Helper()
	if r.code != http.StatusForbidden || r.reason() != notMemberReason {
		t.Fatalf("%s: want 403 not_member, got %d %s", what, r.code, r.raw)
	}
}

func (e *env) share(uid, appPath string) string {
	e.t.Helper()
	r := e.do(uid, http.MethodPost, appPath+"/share", nil, nil)
	if r.code != http.StatusCreated {
		e.t.Fatalf("share by %s: %d %s", uid, r.code, r.raw)
	}
	return r.body["token"].(string)
}

// #254 完了の条件: リンクで参加した人だけが shared のデータを読める・それ以外は 403。
func TestSharedMembership(t *testing.T) { ownerStores(t, testSharedMembership) }

func testSharedMembership(t *testing.T, e *env, sfx string) {
	id := "FAMILY-1" + sfx
	app := "/v1/apps/" + id
	recs := app + "/entities/expense/records"
	e.ownApp("owner", app, familyKakeibo(t))
	if r := e.do("owner", http.MethodPost, recs, `{"data":{"amount":100}}`, nil); r.code != http.StatusCreated {
		t.Fatalf("owner post: %d %s", r.code, r.raw)
	}

	// knowing the appId is not enough: the DSL installs, the shared records do not
	if r := e.do("stranger", http.MethodGet, app+"/dsl", nil, nil); r.code != http.StatusOK {
		t.Fatalf("stranger dsl: %d %s", r.code, r.raw)
	}
	wantNotMember(t, e.do("stranger", http.MethodGet, recs, nil, nil), "stranger list")
	wantNotMember(t, e.do("stranger", http.MethodPost, recs, `{"data":{"amount":1}}`, nil), "stranger post")
	wantNotMember(t, e.do("stranger", http.MethodPost, app+"/share", nil, nil), "stranger share")
	wantNotMember(t, e.do("stranger", http.MethodGet, app+"/members", nil, nil), "stranger members")
	if r := e.do("stranger", http.MethodGet, app+"/entities/expense", nil, nil); r.code != http.StatusOK {
		t.Fatalf("entity definition stays readable (it is in the DSL): %d %s", r.code, r.raw)
	}

	// joining through the owner's link
	tok := e.share("owner", app)
	r := e.do("mom", http.MethodPost, "/v1/shares/"+tok+"/import", `{"displayName":"  ママ "}`, nil)
	if r.code != http.StatusCreated || r.body["joined"] != true || r.body["member"] != true || r.body["id"] != id {
		t.Fatalf("mom import: %d %s", r.code, r.raw)
	}
	if r = e.do("mom", http.MethodGet, recs, nil, nil); r.code != http.StatusOK || len(r.body["items"].([]any)) != 1 {
		t.Fatalf("mom reads owner's record: %d %s", r.code, r.raw)
	}
	if r = e.do("mom", http.MethodPost, recs, `{"data":{"amount":200}}`, nil); r.code != http.StatusCreated {
		t.Fatalf("mom post: %d %s", r.code, r.raw)
	}
	// a participant may invite (#118); the stranger who already opened the appId joins through it
	momTok := e.share("mom", app)
	if r = e.do("stranger", http.MethodPost, "/v1/shares/"+momTok+"/import", nil, nil); r.code != http.StatusCreated || r.body["created"] != false || r.body["member"] != true {
		t.Fatalf("join with an existing copy: %d %s", r.code, r.raw)
	}
	if r = e.do("stranger", http.MethodGet, recs, nil, nil); r.code != http.StatusOK || len(r.body["items"].([]any)) != 2 {
		t.Fatalf("joined stranger reads: %d %s", r.code, r.raw)
	}
	wantNotMember(t, e.do("kid", http.MethodGet, recs, nil, nil), "third person")

	// member list: owner first, then members by join order
	r = e.do("mom", http.MethodGet, app+"/members", nil, nil)
	items, _ := r.body["items"].([]any)
	if r.code != http.StatusOK || r.body["role"] != "member" || len(items) != 3 {
		t.Fatalf("members: %d %s", r.code, r.raw)
	}
	first, second := items[0].(map[string]any), items[1].(map[string]any)
	if first["uid"] != "owner" || first["role"] != "owner" || first["you"] != false ||
		second["uid"] != "mom" || second["displayName"] != "ママ" || second["via"] != "share" || second["you"] != true {
		t.Fatalf("member items: %s", r.raw)
	}

	// removing: creator only; the creator cannot leave
	if r = e.do("mom", http.MethodDelete, app+"/members/stranger", nil, nil); r.code != http.StatusForbidden {
		t.Fatalf("member removes member: want 403, got %d %s", r.code, r.raw)
	}
	if r = e.do("owner", http.MethodDelete, app+"/members/me", nil, nil); r.code != http.StatusBadRequest {
		t.Fatalf("owner leaves: want 400, got %d %s", r.code, r.raw)
	}
	if r = e.do("owner", http.MethodDelete, app+"/members/nobody", nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("remove unknown: want 404, got %d %s", r.code, r.raw)
	}
	if r = e.do("owner", http.MethodDelete, app+"/members/mom", nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("owner removes mom: %d %s", r.code, r.raw)
	}
	wantNotMember(t, e.do("mom", http.MethodGet, recs, nil, nil), "removed member")
	// links issued by a removed member stop working
	r = e.do("kid", http.MethodPost, "/v1/shares/"+momTok+"/import", nil, nil)
	if r.code != http.StatusForbidden || r.reason() != "inviter_left" {
		t.Fatalf("removed member's link: %d %s", r.code, r.raw)
	}
	// kid only has the version-1 seed from the 403 above (GET seeds any appId)
	if r = e.do("kid", http.MethodGet, app, nil, nil); r.body["currentVersion"] != float64(1) {
		t.Fatalf("a refused join must not install a copy: %s", r.raw)
	}
	// the owner's live link still works until revoked
	e.do("owner", http.MethodDelete, "/v1/shares/"+tok, nil, nil)
	if r = e.do("mom", http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("revoked link: %d %s", r.code, r.raw)
	}

	// leaving (idempotent)
	for i := 0; i < 2; i++ {
		if r = e.do("stranger", http.MethodDelete, app+"/members/me", nil, nil); r.code != http.StatusNoContent {
			t.Fatalf("leave #%d: %d %s", i, r.code, r.raw)
		}
	}
	wantNotMember(t, e.do("stranger", http.MethodGet, recs, nil, nil), "after leaving")
	if r = e.do("owner", http.MethodGet, app+"/members", nil, nil); r.body["role"] != "owner" || len(r.body["items"].([]any)) != 1 {
		t.Fatalf("members after leaving: %s", r.raw)
	}
}

// scope:"public" is unchanged: anyone who opens the appId reads and posts.
func TestPublicNeedsNoMembership(t *testing.T) {
	e := newEnv(t)
	app := "/v1/apps/BOARD-1"
	e.ownApp("owner", app, sharedKakeibo(t))
	if r := e.do("anyone", http.MethodPost, app+"/entities/expense/records", `{"data":{"amount":1}}`, nil); r.code != http.StatusCreated {
		t.Fatalf("public post: %d %s", r.code, r.raw)
	}
	tok := e.share("anyone", app) // no membership needed to pass a public app on
	if r := e.do("friend", http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil); r.code != http.StatusCreated || r.body["member"] != false {
		t.Fatalf("public import: %d %s", r.code, r.raw)
	}
}

// Migration: copies created before SHARED_MEMBERS_LEGACY_BEFORE join on first access.
func TestLegacyMembers(t *testing.T) {
	t.Setenv(LegacyMembersEnv, time.Now().Add(time.Hour).Format(time.RFC3339))
	e := newEnv(t)
	app := "/v1/apps/OLD-FAMILY"
	e.ownApp("owner", app, familyKakeibo(t))
	e.do("old", http.MethodGet, app+"/dsl", nil, nil) // joined the old way (open the appId)
	if r := e.do("old", http.MethodGet, app+"/entities/expense/records", nil, nil); r.code != http.StatusOK {
		t.Fatalf("legacy copy: %d %s", r.code, r.raw)
	}
	r := e.do("owner", http.MethodGet, app+"/members", nil, nil)
	if items := r.body["items"].([]any); len(items) != 2 || items[1].(map[string]any)["via"] != "legacy" {
		t.Fatalf("legacy member recorded: %s", r.raw)
	}

	t.Setenv(LegacyMembersEnv, time.Now().Add(-time.Hour).Format(time.RFC3339))
	e2 := newEnv(t)
	e2.ownApp("owner", app, familyKakeibo(t))
	e2.do("new", http.MethodGet, app+"/dsl", nil, nil)
	wantNotMember(t, e2.do("new", http.MethodGet, app+"/entities/expense/records", nil, nil), "copy after cutover")
}

// The catalog is for everyone, so a shared-only app installs as an
// independent copy instead of joining the publisher's family data.
func TestCatalogInstallOfSharedApp(t *testing.T) {
	e := newEnv(t)
	app := "/v1/apps/FAMILY-PUB"
	e.ownApp("owner", app, familyKakeibo(t))
	if r := e.do("owner", http.MethodPost, app+"/publish", `{}`, nil); r.code != http.StatusOK {
		t.Fatalf("publish: %d %s", r.code, r.raw)
	}
	r := e.do("user", http.MethodPost, "/v1/public-apps/FAMILY-PUB/install", nil, nil)
	newID, _ := r.body["id"].(string)
	if r.code != http.StatusCreated || r.body["joined"] != false || newID == "FAMILY-PUB" {
		t.Fatalf("install: %d %s", r.code, r.raw)
	}
	// the installer created the copy, so its shared data is theirs
	if r = e.do("user", http.MethodPost, "/v1/apps/"+newID+"/entities/expense/records", `{"data":{"amount":1}}`, nil); r.code != http.StatusCreated {
		t.Fatalf("own copy: %d %s", r.code, r.raw)
	}
	wantNotMember(t, e.do("user", http.MethodGet, app+"/entities/expense/records", nil, nil), "publisher's data")
}

// Non-participants get no change signals from shared entities.
func TestEventsSkipSharedForNonMembers(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	t.Setenv("EVENTS_MAX_STREAM", "10s")
	srv := httptest.NewServer(auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory())))
	defer srv.Close()
	e := &env{t: t, h: srv.Config.Handler}
	const appID = "FAMILY-RT"
	e.ownApp("owner", "/v1/apps/"+appID, familyKakeibo(t))

	for uid, want := range map[string]int{"stranger": 0, "owner": 1} {
		events, stop := openEvents(t, srv, uid, appID)
		hello := next(t, events)
		stop()
		if ents, _ := hello.data["entities"].([]any); hello.name != "hello" || len(ents) != want {
			t.Fatalf("%s hello: %+v", uid, hello)
		}
	}
}
