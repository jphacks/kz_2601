package appconfig

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

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
