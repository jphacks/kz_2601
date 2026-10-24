package appconfig

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

func walk(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../../docs/dsl/examples/walk.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// ownApp creates appPath for uid with dsl at version 2.
func (e *env) ownApp(uid, appPath string, dsl []byte) {
	e.t.Helper()
	e.do(uid, http.MethodGet, appPath+"/dsl", nil, nil)
	if r := e.do(uid, http.MethodPut, appPath+"/dsl", putBody(dsl), map[string]string{"If-Match": `"1"`}); r.code != http.StatusCreated {
		e.t.Fatalf("put %s: %d %s", appPath, r.code, r.raw)
	}
}

func TestShareImportRoundTrip(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/OWNER-KAKEIBO"
	e.ownApp("owner", app, kakeibo(t))
	e.do("owner", http.MethodPost, app+"/entities/expense/records", `{"data":{"amount":100}}`, nil)

	// only apps in the caller's own tree can be shared
	if r := e.do("other", http.MethodPost, app+"/share", nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("share someone else's app: want 404, got %d %s", r.code, r.raw)
	}
	r := e.do("owner", http.MethodPost, app+"/share", nil, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("share: %d %s", r.code, r.raw)
	}
	tok, _ := r.body["token"].(string)
	if len(tok) < 32 || r.body["url"] != "tailorit://import/"+tok || !strings.HasSuffix(r.body["webUrl"].(string), "/s/"+tok) || r.body["dslVersion"] != float64(2) {
		t.Fatalf("share body: %s", r.raw)
	}

	// preview + import by another user: new appId, version 2 (not blank), no records
	if r = e.do("friend", http.MethodGet, "/v1/shares/"+tok, nil, nil); r.code != http.StatusOK || r.body["name"] != "家計簿" || r.body["sharedData"] != false {
		t.Fatalf("preview: %d %s", r.code, r.raw)
	}
	r = e.do("friend", http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil)
	if r.code != http.StatusCreated || r.body["joined"] != false || r.body["created"] != true || r.body["currentVersion"] != float64(2) {
		t.Fatalf("import: %d %s", r.code, r.raw)
	}
	newID, _ := r.body["id"].(string)
	if newID == "" || newID == "OWNER-KAKEIBO" {
		t.Fatalf("import must get a new appId: %s", r.raw)
	}
	if d, _ := r.body["dsl"].(map[string]any); d["initialScreen"] != "home" {
		t.Fatalf("imported dsl: %s", r.raw)
	}
	if r = e.do("friend", http.MethodGet, "/v1/apps/"+newID+"/entities/expense/records", nil, nil); r.code != http.StatusOK || len(r.body["items"].([]any)) != 0 {
		t.Fatalf("records must not be copied: %d %s", r.code, r.raw)
	}
	if r = e.do("friend", http.MethodGet, "/v1/apps", nil, nil); len(r.body["items"].([]any)) != 1 {
		t.Fatalf("friend's apps: %s", r.raw)
	}
	r = e.do("friend", http.MethodGet, "/v1/apps/"+newID+"/versions", nil, nil)
	if src := r.body["items"].([]any)[0].(map[string]any)["source"].(map[string]any); src["kind"] != "import" {
		t.Fatalf("import source: %s", r.raw)
	}

	// revoke: owner only, then import / preview are 404
	if r = e.do("friend", http.MethodDelete, "/v1/shares/"+tok, nil, nil); r.code != http.StatusForbidden {
		t.Fatalf("revoke by non-owner: want 403, got %d %s", r.code, r.raw)
	}
	if r = e.do("owner", http.MethodDelete, "/v1/shares/"+tok, nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("revoke: %d %s", r.code, r.raw)
	}
	if r = e.do("friend", http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("import after revoke: want 404, got %d %s", r.code, r.raw)
	}
	if r = e.do("friend", http.MethodPost, "/v1/shares/nope/import", nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("unknown token: %d", r.code)
	}
	if r = e.do("", http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil); r.code != http.StatusUnauthorized {
		t.Fatalf("import requires auth: %d", r.code)
	}
}

func TestShareImportJoinsSharedApp(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/BOARD-1"
	e.ownApp("owner", app, sharedKakeibo(t))
	e.do("owner", http.MethodPost, app+"/entities/expense/records", `{"data":{"amount":100,"memo":"みんなへ"}}`, nil)
	tok, _ := e.do("owner", http.MethodPost, app+"/share", nil, nil).body["token"].(string)

	r := e.do("friend", http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil)
	if r.code != http.StatusCreated || r.body["id"] != "BOARD-1" || r.body["joined"] != true || r.body["created"] != true {
		t.Fatalf("join import: %d %s", r.code, r.raw)
	}
	// the receiver sees the same public records (#93)
	if r = e.do("friend", http.MethodGet, app+"/entities/expense/records", nil, nil); len(r.body["items"].([]any)) != 1 {
		t.Fatalf("shared records: %s", r.raw)
	}
	// importing again is idempotent
	if r = e.do("friend", http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil); r.code != http.StatusCreated || r.body["created"] != false {
		t.Fatalf("re-import: %d %s", r.code, r.raw)
	}
}

func TestPublishSearchInstall(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/OWNER-KAKEIBO"
	e.ownApp("owner", app, kakeibo(t))

	if r := e.do("other", http.MethodPost, app+"/publish", `{}`, nil); r.code != http.StatusNotFound {
		t.Fatalf("publish someone else's app: %d %s", r.code, r.raw)
	}
	if r := e.do("owner", http.MethodPost, app+"/publish", `{"tags":["`+strings.Repeat("x", 21)+`"]}`, nil); r.code != http.StatusBadRequest {
		t.Fatalf("long tag: %d %s", r.code, r.raw)
	}
	r := e.do("owner", http.MethodPost, app+"/publish", `{"description":"毎日の支出をサッと記録","tags":["#お金","節約"],"authorName":"こた"}`, nil)
	if r.code != http.StatusOK || r.body["name"] != "家計簿" || r.body["installs"] != float64(0) || r.body["mine"] != true {
		t.Fatalf("publish: %d %s", r.code, r.raw)
	}
	if tags := r.body["tags"].([]any); len(tags) != 2 || tags[0] != "お金" {
		t.Fatalf("tags normalized: %s", r.raw)
	}
	if comps := r.body["components"].([]any); len(comps) == 0 {
		t.Fatalf("components: %s", r.raw)
	}
	// a second listing to check ordering / filtering
	e.ownApp("owner2", "/v1/apps/OTHER-WALK", walk(t))
	e.do("owner2", http.MethodPost, "/v1/apps/OTHER-WALK/publish", `{"tags":["健康"]}`, nil)

	search := func(q string) []any {
		t.Helper()
		r := e.do("finder", http.MethodGet, "/v1/public-apps?q="+q, nil, nil)
		if r.code != http.StatusOK {
			t.Fatalf("search %q: %d %s", q, r.code, r.raw)
		}
		return r.body["items"].([]any)
	}
	if got := search(""); len(got) != 2 {
		t.Fatalf("all: %v", got)
	}
	for _, q := range []string{"%E5%AE%B6%E8%A8%88", "%E7%AF%80%E7%B4%84", "%E6%94%AF%E5%87%BA"} { // 家計 / 節約(tag) / 支出(description)
		if got := search(q); len(got) != 1 || got[0].(map[string]any)["id"] != "OWNER-KAKEIBO" {
			t.Fatalf("search %s: %v", q, got)
		}
	}
	if got := search("zzz"); len(got) != 0 {
		t.Fatalf("no hit: %v", got)
	}

	// install: new appId, installs+1, then popular order puts it first
	r = e.do("finder", http.MethodPost, "/v1/public-apps/OWNER-KAKEIBO/install", nil, nil)
	if r.code != http.StatusCreated || r.body["id"] == "OWNER-KAKEIBO" || r.body["currentVersion"] != float64(2) {
		t.Fatalf("install: %d %s", r.code, r.raw)
	}
	e.do("owner", http.MethodPost, "/v1/public-apps/OWNER-KAKEIBO/install", nil, nil) // owner's own install is not counted
	r = e.do("finder", http.MethodGet, "/v1/public-apps?sort=popular", nil, nil)
	first := r.body["items"].([]any)[0].(map[string]any)
	if first["id"] != "OWNER-KAKEIBO" || first["installs"] != float64(1) || first["mine"] != false {
		t.Fatalf("popular: %s", r.raw)
	}
	if r = e.do("finder", http.MethodGet, "/v1/public-apps?sort=bogus", nil, nil); r.code != http.StatusBadRequest {
		t.Fatalf("bad sort: %d", r.code)
	}

	// unpublish: owner only
	if r = e.do("finder", http.MethodDelete, app+"/publish", nil, nil); r.code != http.StatusForbidden {
		t.Fatalf("unpublish by non-owner: want 403, got %d %s", r.code, r.raw)
	}
	if r = e.do("owner", http.MethodDelete, app+"/publish", nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("unpublish: %d %s", r.code, r.raw)
	}
	if got := search(""); len(got) != 1 {
		t.Fatalf("after unpublish: %v", got)
	}
	if r = e.do("finder", http.MethodPost, "/v1/public-apps/OWNER-KAKEIBO/install", nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("install unpublished: %d", r.code)
	}
	// a user who joined the same appId cannot hijack the listing
	e.ownApp("owner2", "/v1/apps/OWNER-KAKEIBO", kakeibo(t))
	e.do("owner", http.MethodPost, app+"/publish", `{}`, nil)
	if r = e.do("owner2", http.MethodPost, app+"/publish", `{}`, nil); r.code != http.StatusForbidden {
		t.Fatalf("publish over another owner's listing: want 403, got %d %s", r.code, r.raw)
	}
}

func TestSharePage(t *testing.T) {
	st := store.NewMemory()
	if err := st.CreateShare(t.Context(), store.Share{Token: "tok_abc", Name: "<家計簿>", AppID: "a"}); err != nil {
		t.Fatal(err)
	}
	h := SharePage(st)
	mux := http.NewServeMux()
	mux.Handle("GET /s/{token}", h)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/s/tok_abc", nil))
	body := rr.Body.String()
	if rr.Code != http.StatusOK || !strings.Contains(body, "tailorit://import/tok_abc") || !strings.Contains(body, "&lt;家計簿&gt;") {
		t.Fatalf("page: %d %s", rr.Code, body)
	}
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/s/missing", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing: %d", rr.Code)
	}
}

// ownerStores runs f against Memory and, when FIRESTORE_EMULATOR_HOST is set,
// against the Firestore emulator (ids get a per-run suffix there).
func ownerStores(t *testing.T, f func(t *testing.T, e *env, sfx string)) {
	t.Run("memory", func(t *testing.T) {
		t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
		f(t, &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory()))}, "")
	})
	t.Run("firestore", func(t *testing.T) {
		if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
			t.Skip("FIRESTORE_EMULATOR_HOST not set")
		}
		st, err := store.NewFirestore(t.Context(), "demo-kz2601")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.Close() })
		t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
		f(t, &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}, "-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	})
}

// #153: opening someone's appId seeds it into your tree, but only its creator
// can publish it; a blank app cannot be published or shared.
func TestPublishCreatorOnly(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		owner, squatter := "owner"+sfx, "squatter"+sfx
		app := "/v1/apps/SQUAT-ME" + sfx
		e.ownApp(owner, app, kakeibo(t))

		// GET /dsl by another user still works (seed = join, #47 / #93) ...
		if r := e.do(squatter, http.MethodGet, app+"/dsl", nil, nil); r.code != http.StatusOK || r.body["version"] != float64(1) {
			t.Fatalf("seed: %d %s", r.code, r.raw)
		}
		// ... but the seed cannot be published
		if r := e.do(squatter, http.MethodPost, app+"/publish", `{"description":"x"}`, nil); r.code != http.StatusForbidden {
			t.Fatalf("squat with seed: want 403, got %d %s", r.code, r.raw)
		}
		// nor after writing real content to the same appId
		if r := e.do(squatter, http.MethodPut, app+"/dsl", putBody(walk(t)), map[string]string{"If-Match": `"1"`}); r.code != http.StatusCreated {
			t.Fatalf("squatter put: %d %s", r.code, r.raw)
		}
		if r := e.do(squatter, http.MethodPost, app+"/publish", `{}`, nil); r.code != http.StatusForbidden {
			t.Fatalf("squat with content: want 403, got %d %s", r.code, r.raw)
		}
		if r := e.do(owner, http.MethodPost, app+"/publish", `{}`, nil); r.code != http.StatusOK || r.body["mine"] != true {
			t.Fatalf("owner publish: %d %s", r.code, r.raw)
		}
		if r := e.do(squatter, http.MethodDelete, app+"/publish", nil, nil); r.code != http.StatusForbidden {
			t.Fatalf("unpublish by non-owner: %d %s", r.code, r.raw)
		}

		// a blank app (initial DSL only) is 400 for publish and share
		fresh := "/v1/apps/FRESH" + sfx
		if r := e.do(owner, http.MethodGet, fresh+"/dsl", nil, nil); r.code != http.StatusOK {
			t.Fatalf("fresh: %d %s", r.code, r.raw)
		}
		if r := e.do(owner, http.MethodPost, fresh+"/publish", `{}`, nil); r.code != http.StatusBadRequest {
			t.Fatalf("publish blank: want 400, got %d %s", r.code, r.raw)
		}
		if r := e.do(owner, http.MethodPost, fresh+"/share", nil, nil); r.code != http.StatusBadRequest {
			t.Fatalf("share blank: want 400, got %d %s", r.code, r.raw)
		}
		// the blank attempt did not claim it: generating then publishing works
		e.ownApp(owner, fresh, walk(t))
		if r := e.do(owner, http.MethodPost, fresh+"/publish", `{}`, nil); r.code != http.StatusOK {
			t.Fatalf("publish after generate: %d %s", r.code, r.raw)
		}
	})
}

// #153 / #118: a member who joined a shared app keeps using and sharing it,
// but cannot publish it.
func TestJoinedMemberCannotPublish(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		owner, friend, guest := "owner"+sfx, "friend"+sfx, "guest"+sfx
		app := "/v1/apps/BOARD" + sfx
		e.ownApp(owner, app, sharedKakeibo(t))
		tok, _ := e.do(owner, http.MethodPost, app+"/share", nil, nil).body["token"].(string)
		if r := e.do(friend, http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil); r.code != http.StatusCreated || r.body["joined"] != true {
			t.Fatalf("join: %d %s", r.code, r.raw)
		}
		if r := e.do(friend, http.MethodPost, app+"/publish", `{}`, nil); r.code != http.StatusForbidden {
			t.Fatalf("member publish: want 403, got %d %s", r.code, r.raw)
		}
		r := e.do(friend, http.MethodPost, app+"/share", nil, nil)
		if r.code != http.StatusCreated {
			t.Fatalf("member share: %d %s", r.code, r.raw)
		}
		tok2, _ := r.body["token"].(string)
		if r = e.do(guest, http.MethodPost, "/v1/shares/"+tok2+"/import", nil, nil); r.code != http.StatusCreated || r.body["id"] != "BOARD"+sfx || r.body["joined"] != true {
			t.Fatalf("guest join via member's share: %d %s", r.code, r.raw)
		}
		if r = e.do(owner, http.MethodPost, app+"/publish", `{}`, nil); r.code != http.StatusOK {
			t.Fatalf("owner publish: %d %s", r.code, r.raw)
		}
	})
}

// #154: installing again returns the first copy and installs counts uids.
func TestInstallTwiceCountsOnce(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		owner, finder := "owner"+sfx, "finder"+sfx
		id := "TWICE" + sfx
		e.ownApp(owner, "/v1/apps/"+id, kakeibo(t))
		e.do(owner, http.MethodPost, "/v1/apps/"+id+"/publish", `{}`, nil)

		r1 := e.do(finder, http.MethodPost, "/v1/public-apps/"+id+"/install", nil, nil)
		if r1.code != http.StatusCreated || r1.body["created"] != true {
			t.Fatalf("install: %d %s", r1.code, r1.raw)
		}
		copyID, _ := r1.body["id"].(string)
		e.do(finder, http.MethodDelete, "/v1/apps/"+copyID, nil, nil) // archived; re-install brings it back
		r2 := e.do(finder, http.MethodPost, "/v1/public-apps/"+id+"/install", nil, nil)
		if r2.code != http.StatusCreated || r2.body["created"] != false || r2.body["id"] != copyID || r2.body["status"] != "active" {
			t.Fatalf("re-install: %d %s", r2.code, r2.raw)
		}
		e.do(owner, http.MethodPost, "/v1/public-apps/"+id+"/install", nil, nil)
		r := e.do(finder, http.MethodGet, "/v1/public-apps?pageSize=200", nil, nil)
		var installs any
		for _, it := range r.body["items"].([]any) {
			if m := it.(map[string]any); m["id"] == id {
				installs = m["installs"]
			}
		}
		if installs != float64(1) {
			t.Fatalf("installs: want 1, got %v (%s)", installs, r.raw)
		}
	})
}

// #153: listings made before app_owners existed keep their publisher as creator.
func TestPublishLegacyListingOwner(t *testing.T) {
	ctx := t.Context()
	st := store.NewMemory()
	raw := kakeibo(t)
	if _, _, err := st.EnsureApp(ctx, "legacy", "OLD", raw, "家計簿"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutCatalogApp(ctx, "legacy", store.CatalogApp{ID: "OLD", Name: "家計簿", DSL: raw, Version: 1}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}
	e.ownApp("squatter", "/v1/apps/OLD", walk(t))
	if r := e.do("squatter", http.MethodPost, "/v1/apps/OLD/publish", `{}`, nil); r.code != http.StatusForbidden {
		t.Fatalf("squat legacy: want 403, got %d %s", r.code, r.raw)
	}
	if r := e.do("legacy", http.MethodPost, "/v1/apps/OLD/publish", `{}`, nil); r.code != http.StatusOK {
		t.Fatalf("legacy owner republish: %d %s", r.code, r.raw)
	}
	if o, _ := st.AppOwner(ctx, "OLD"); o != "legacy" {
		t.Fatalf("owner backfilled: %q", o)
	}
}

// #154: placeholder strings are not appIds; existing id styles still are.
func TestInvalidAppID(t *testing.T) {
	e := newEnv(t)
	for _, id := range []string{"null", "undefined", "NULL", "a%20b", strings.Repeat("x", 129)} {
		if r := e.do("u", http.MethodGet, "/v1/apps/"+id+"/dsl", nil, nil); r.code != http.StatusBadRequest {
			t.Fatalf("GET /apps/%s/dsl: want 400, got %d %s", id, r.code, r.raw)
		}
	}
	if r := e.do("u", http.MethodPost, "/v1/apps", `{"id":"undefined"}`, nil); r.code != http.StatusBadRequest {
		t.Fatalf("create undefined: %d %s", r.code, r.raw)
	}
	for _, id := range []string{"8F2C1A3E-0B6D-4E1F-9A7C-2D5B8E4F6A10", "catalog-kakeibo", "RT-SMOKE-1761300000", "app_01JB0000000000000000000000", "nullable"} {
		if r := e.do("u", http.MethodGet, "/v1/apps/"+id+"/dsl", nil, nil); r.code != http.StatusOK {
			t.Fatalf("GET /apps/%s/dsl: want 200, got %d %s", id, r.code, r.raw)
		}
	}
	if r := e.do("u", http.MethodGet, "/v1/apps", nil, nil); len(r.body["items"].([]any)) != 5 {
		t.Fatalf("only valid ids seeded: %s", r.raw)
	}
}
