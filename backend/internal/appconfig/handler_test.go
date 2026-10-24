package appconfig

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

type env struct {
	t *testing.T
	h http.Handler
}

func newEnv(t *testing.T) *env {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory()))}
}

type resp struct {
	code int
	hdr  http.Header
	body map[string]any
	raw  []byte
}

func (e *env) do(uid, method, path string, body any, hdr map[string]string) resp {
	e.t.Helper()
	var rd *bytes.Reader
	switch b := body.(type) {
	case nil:
		rd = bytes.NewReader(nil)
	case string:
		rd = bytes.NewReader([]byte(b))
	case []byte:
		rd = bytes.NewReader(b)
	default:
		buf, err := json.Marshal(b)
		if err != nil {
			e.t.Fatal(err)
		}
		rd = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, rd)
	if uid != "" {
		req.Header.Set(auth.DebugUIDHeader, uid)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	e.h.ServeHTTP(rr, req)
	out := resp{code: rr.Code, hdr: rr.Header(), raw: rr.Body.Bytes()}
	if len(out.raw) > 0 {
		_ = json.Unmarshal(out.raw, &out.body)
	}
	return out
}

func (r resp) errCode() string {
	e, _ := r.body["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

func (r resp) version() int {
	v, _ := r.body["version"].(float64)
	return int(v)
}

func kakeibo(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../../docs/dsl/examples/kakeibo.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func putBody(dsl []byte) string {
	return `{"dsl":` + string(dsl) + `,"source":{"kind":"ai","mode":"full","transcript":"家計簿","summary":"家計簿を作成","model":"test"}}`
}

func TestAuthRequired(t *testing.T) {
	e := newEnv(t)
	r := e.do("", http.MethodGet, "/v1/apps", nil, nil)
	if r.code != http.StatusUnauthorized || r.errCode() != "unauthenticated" {
		t.Fatalf("got %d %s", r.code, r.raw)
	}
}

func TestDSLLifecycle(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/DEVICE-UUID-1"

	// unknown appId -> auto-created at version 1 with the initial DSL
	r := e.do("u1", http.MethodGet, app+"/dsl", nil, nil)
	if r.code != http.StatusOK || r.version() != 1 || r.hdr.Get("ETag") != `"1"` {
		t.Fatalf("initial get: %d etag=%q body=%s", r.code, r.hdr.Get("ETag"), r.raw)
	}
	if d, _ := r.body["dsl"].(map[string]any); d["initialScreen"] != "home" {
		t.Fatalf("initial dsl: %s", r.raw)
	}
	if r = e.do("u1", http.MethodGet, app+"/dsl", nil, map[string]string{"If-None-Match": `"1"`}); r.code != http.StatusNotModified {
		t.Fatalf("If-None-Match: want 304, got %d", r.code)
	}

	// PUT: missing If-Match 412, stale 409, invalid 422, ok 201 -> version 2
	if r = e.do("u1", http.MethodPut, app+"/dsl", putBody(kakeibo(t)), nil); r.code != http.StatusPreconditionFailed {
		t.Fatalf("no If-Match: want 412, got %d %s", r.code, r.raw)
	}
	r = e.do("u1", http.MethodPut, app+"/dsl", putBody(kakeibo(t)), map[string]string{"If-Match": `"9"`})
	if r.code != http.StatusConflict || r.errCode() != "version_conflict" {
		t.Fatalf("stale: %d %s", r.code, r.raw)
	}
	if d := r.body["error"].(map[string]any)["details"].(map[string]any); d["currentVersion"] != float64(1) {
		t.Fatalf("409 must carry currentVersion: %s", r.raw)
	}
	bad := strings.Replace(string(kakeibo(t)), `"initialScreen": "home"`, `"initialScreen": "nope"`, 1)
	r = e.do("u1", http.MethodPut, app+"/dsl", putBody([]byte(bad)), map[string]string{"If-Match": `"1"`})
	if r.code != http.StatusUnprocessableEntity || r.errCode() != "invalid_dsl" {
		t.Fatalf("invalid dsl: %d %s", r.code, r.raw)
	}
	r = e.do("u1", http.MethodPut, app+"/dsl", putBody(kakeibo(t)), map[string]string{"If-Match": `"1"`})
	if r.code != http.StatusCreated || r.version() != 2 || r.hdr.Get("ETag") != `"2"` {
		t.Fatalf("put: %d %s", r.code, r.raw)
	}
	if r = e.do("u1", http.MethodGet, app+"/dsl", nil, map[string]string{"If-None-Match": `"1"`}); r.code != http.StatusOK || r.version() != 2 {
		t.Fatalf("get after put: %d %s", r.code, r.raw)
	}

	// PATCH (json-patch+json body) -> version 3, envelope carries the new field
	patch := `[{"op":"add","path":"/entities/0/fields/-","value":{"id":"shop","name":"店名","type":"string"}}]`
	r = e.do("u1", http.MethodPatch, app+"/dsl", patch, map[string]string{"If-Match": `"2"`, "Content-Type": "application/json-patch+json"})
	if r.code != http.StatusCreated || r.version() != 3 {
		t.Fatalf("patch: %d %s", r.code, r.raw)
	}
	if !strings.Contains(string(r.raw), `"shop"`) {
		t.Fatalf("patched dsl must contain shop: %s", r.raw)
	}
	// PATCH (application/json {patch, source} as the ai client sends) with a stale base -> 409
	r = e.do("u1", http.MethodPatch, app+"/dsl", `{"patch":`+patch+`,"source":{"kind":"ai","mode":"patch"}}`, map[string]string{"If-Match": `"2"`})
	if r.code != http.StatusConflict {
		t.Fatalf("stale patch: %d %s", r.code, r.raw)
	}
	// invalid patch -> 422 with problems
	r = e.do("u1", http.MethodPatch, app+"/dsl", `{"patch":[{"op":"remove","path":"/screens/0"}]}`, map[string]string{"If-Match": `"3"`})
	if r.code != http.StatusUnprocessableEntity || r.errCode() != "invalid_dsl" {
		t.Fatalf("invalid patch: %d %s", r.code, r.raw)
	}

	// versions: newest first, no dsl body
	r = e.do("u1", http.MethodGet, app+"/versions", nil, nil)
	items, _ := r.body["items"].([]any)
	if r.code != http.StatusOK || len(items) != 3 || items[0].(map[string]any)["version"] != float64(3) {
		t.Fatalf("versions: %d %s", r.code, r.raw)
	}
	if _, has := items[0].(map[string]any)["dsl"]; has {
		t.Fatalf("version summary must not include dsl")
	}
	if items[0].(map[string]any)["patch"] == nil {
		t.Fatalf("patched version must record its patch: %s", r.raw)
	}
	if r = e.do("u1", http.MethodGet, app+"/versions/2", nil, nil); r.code != http.StatusOK || r.version() != 2 {
		t.Fatalf("get version: %d %s", r.code, r.raw)
	}

	// restore 1 -> version 4 with the initial dsl
	r = e.do("u1", http.MethodPost, app+"/versions/1/restore", nil, nil)
	if r.code != http.StatusCreated || r.version() != 4 {
		t.Fatalf("restore: %d %s", r.code, r.raw)
	}
	r = e.do("u1", http.MethodGet, app+"/dsl", nil, nil)
	if d, _ := r.body["dsl"].(map[string]any); r.version() != 4 || len(d["entities"].([]any)) != 0 {
		t.Fatalf("after restore: %s", r.raw)
	}
	if r = e.do("u1", http.MethodPost, app+"/versions/99/restore", nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("restore unknown: %d", r.code)
	}

	// uid isolation + apps list
	if r = e.do("u2", http.MethodGet, app, nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("other uid must not see the app: %d", r.code)
	}
	r = e.do("u1", http.MethodGet, "/v1/apps", nil, nil)
	if items, _ := r.body["items"].([]any); len(items) != 1 || items[0].(map[string]any)["currentVersion"] != float64(4) {
		t.Fatalf("list apps: %s", r.raw)
	}
}

func TestCreateApp(t *testing.T) {
	e := newEnv(t)
	r := e.do("u1", http.MethodPost, "/v1/apps", `{"name":"x"}`, nil)
	if r.code != http.StatusCreated || r.body["currentVersion"] != float64(1) || r.body["dsl"] == nil {
		t.Fatalf("create: %d %s", r.code, r.raw)
	}
	id, _ := r.body["id"].(string)
	if r = e.do("u1", http.MethodGet, "/v1/apps/"+id, nil, nil); r.code != http.StatusOK || r.body["name"] != "x" {
		t.Fatalf("get: %d %s", r.code, r.raw)
	}
	if r = e.do("u1", http.MethodDelete, "/v1/apps/"+id, nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("archive: %d", r.code)
	}
	if r = e.do("u1", http.MethodGet, "/v1/apps/"+id, nil, nil); r.body["status"] != "archived" {
		t.Fatalf("status: %s", r.raw)
	}
}

func TestRecords(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/dev-app"
	recs := app + "/entities/expense/records"

	// no entity yet (initial dsl) -> 404
	if r := e.do("u1", http.MethodPost, recs, `{"data":{"amount":1}}`, nil); r.code != http.StatusNotFound {
		t.Fatalf("unknown entity: %d %s", r.code, r.raw)
	}
	e.do("u1", http.MethodGet, app+"/dsl", nil, nil)
	if r := e.do("u1", http.MethodPut, app+"/dsl", putBody(kakeibo(t)), map[string]string{"If-Match": `"1"`}); r.code != http.StatusCreated {
		t.Fatalf("put: %d %s", r.code, r.raw)
	}

	// validation
	r := e.do("u1", http.MethodPost, recs, `{"data":{"amount":"abc","category":"食費","date":"2026-10-24"}}`, nil)
	if r.code != http.StatusUnprocessableEntity || r.errCode() != "invalid_record" {
		t.Fatalf("string amount: %d %s", r.code, r.raw)
	}
	if r = e.do("u1", http.MethodPost, recs, `{"data":{"category":"食費","date":"2026-10-24"}}`, nil); r.code != http.StatusUnprocessableEntity {
		t.Fatalf("missing required: %d %s", r.code, r.raw)
	}
	if r = e.do("u1", http.MethodPost, recs, `{"data":{"amount":1,"category":"謎","date":"2026-10-24"}}`, nil); r.code != http.StatusUnprocessableEntity {
		t.Fatalf("bad enum: %d %s", r.code, r.raw)
	}
	if r = e.do("u1", http.MethodPost, recs, `{"data":{"amount":1,"category":"食費","date":"2026-10-24","nope":1}}`, nil); r.code != http.StatusBadRequest {
		t.Fatalf("unknown key: %d %s", r.code, r.raw)
	}
	if r = e.do("u1", http.MethodPost, recs, `{"data":{"amount":1,"category":"食費","date":"2026-10-24","receipt":"ftp://x"}}`, nil); r.code != http.StatusUnprocessableEntity {
		t.Fatalf("bad photo: %d %s", r.code, r.raw)
	}

	// create with defaults (category default 食費, date default now) + clientId idempotency
	r = e.do("u1", http.MethodPost, recs, `{"data":{"amount":1200,"memo":"ランチ"},"clientId":"c-1"}`, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("create: %d %s", r.code, r.raw)
	}
	data := r.body["data"].(map[string]any)
	if data["category"] != "食費" || data["amount"] != float64(1200) || len(data["date"].(string)) != 10 {
		t.Fatalf("defaults: %s", r.raw)
	}
	id := r.body["id"].(string)
	r2 := e.do("u1", http.MethodPost, recs, `{"data":{"amount":9999,"memo":"replay"},"clientId":"c-1"}`, nil)
	if r2.body["id"] != id || r2.body["data"].(map[string]any)["amount"] != float64(1200) {
		t.Fatalf("clientId replay must return the original: %s", r2.raw)
	}
	e.do("u1", http.MethodPost, recs, `{"data":{"amount":300,"category":"交通","date":"2026-10-01","receipt":"data:image/jpeg;base64,AAAA"}}`, nil)
	e.do("u1", http.MethodPost, recs, `{"data":{"amount":800,"category":"食費","date":"2026-10-20T12:00:00+09:00"}}`, nil)

	// list: default createdAt desc, sort by date desc, filter by category, paging
	r = e.do("u1", http.MethodGet, recs, nil, nil)
	items := r.body["items"].([]any)
	if len(items) != 3 || items[0].(map[string]any)["data"].(map[string]any)["amount"] != float64(800) {
		t.Fatalf("list default: %s", r.raw)
	}
	r = e.do("u1", http.MethodGet, recs+"?sortField=amount&sortDir=asc", nil, nil)
	if items = r.body["items"].([]any); items[0].(map[string]any)["data"].(map[string]any)["amount"] != float64(300) {
		t.Fatalf("sort amount asc: %s", r.raw)
	}
	r = e.do("u1", http.MethodGet, recs+"?sortField=date&sortDir=desc", nil, nil)
	if items = r.body["items"].([]any); items[2].(map[string]any)["data"].(map[string]any)["date"] != "2026-10-01" {
		t.Fatalf("sort date desc: %s", r.raw)
	}
	r = e.do("u1", http.MethodGet, recs+"?filterField=category&filterOp=eq&filterValue=食費", nil, nil)
	if items = r.body["items"].([]any); len(items) != 2 {
		t.Fatalf("filter eq: %s", r.raw)
	}
	r = e.do("u1", http.MethodGet, recs+"?filterField=amount&filterOp=gte&filterValue=800", nil, nil)
	if items = r.body["items"].([]any); len(items) != 2 {
		t.Fatalf("filter gte: %s", r.raw)
	}
	r = e.do("u1", http.MethodGet, recs+"?pageSize=2", nil, nil)
	tok, _ := r.body["nextPageToken"].(string)
	if items = r.body["items"].([]any); len(items) != 2 || tok == "" {
		t.Fatalf("page 1: %s", r.raw)
	}
	r = e.do("u1", http.MethodGet, recs+"?pageSize=2&pageToken="+tok, nil, nil)
	if items = r.body["items"].([]any); len(items) != 1 || r.body["nextPageToken"] != nil {
		t.Fatalf("page 2: %s", r.raw)
	}

	// patch: merge, null removes optional key, required cannot be nulled
	r = e.do("u1", http.MethodPatch, recs+"/"+id, `{"data":{"amount":1500,"memo":null}}`, nil)
	data = r.body["data"].(map[string]any)
	if r.code != http.StatusOK || data["amount"] != float64(1500) || data["memo"] != nil || data["category"] != "食費" {
		t.Fatalf("patch: %d %s", r.code, r.raw)
	}
	if r = e.do("u1", http.MethodPatch, recs+"/"+id, `{"data":{"amount":null}}`, nil); r.code != http.StatusUnprocessableEntity {
		t.Fatalf("null required: %d %s", r.code, r.raw)
	}

	// other uid cannot see; delete; 404 afterwards; delete-all needs confirm
	if r = e.do("u2", http.MethodGet, recs+"/"+id, nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("isolation: %d", r.code)
	}
	if r = e.do("u1", http.MethodDelete, recs+"/"+id, nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", r.code, r.raw)
	}
	if r = e.do("u1", http.MethodGet, recs+"/"+id, nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("after delete: %d", r.code)
	}
	if r = e.do("u1", http.MethodDelete, recs, nil, nil); r.code != http.StatusBadRequest {
		t.Fatalf("delete all without confirm: %d", r.code)
	}
	if r = e.do("u1", http.MethodDelete, recs+"?confirm=true", nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("delete all: %d", r.code)
	}
	if r = e.do("u1", http.MethodGet, recs, nil, nil); len(r.body["items"].([]any)) != 0 {
		t.Fatalf("after delete all: %s", r.raw)
	}

	// entities view
	r = e.do("u1", http.MethodGet, app+"/entities", nil, nil)
	if r.code != http.StatusOK || len(r.body["items"].([]any)) != 1 || r.body["version"] != float64(2) {
		t.Fatalf("entities: %d %s", r.code, r.raw)
	}
}

// sharedKakeibo is the kakeibo example with the expense entity made public.
func sharedKakeibo(t *testing.T) []byte {
	t.Helper()
	const anchor = `"id": "expense",`
	raw := string(kakeibo(t))
	if !strings.Contains(raw, anchor) {
		t.Fatal("kakeibo example changed")
	}
	return []byte(strings.Replace(raw, anchor, anchor+` "scope": "public",`, 1))
}

func TestSharedRecords(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/SHARED-APP-1"
	recs := app + "/entities/expense/records"

	// reserved field on a public entity -> 422
	bad := strings.Replace(string(sharedKakeibo(t)), `{ "id": "memo", "name": "メモ", "type": "string" }`, `{ "id": "author", "name": "投稿者", "type": "string" }`, 1)
	e.do("u1", http.MethodGet, app+"/dsl", nil, nil)
	r := e.do("u1", http.MethodPut, app+"/dsl", putBody([]byte(bad)), map[string]string{"If-Match": `"1"`})
	if r.code != http.StatusUnprocessableEntity || !strings.Contains(string(r.raw), "reserved") {
		t.Fatalf("reserved field: %d %s", r.code, r.raw)
	}

	// u1 publishes an app with a public entity
	if r = e.do("u1", http.MethodPut, app+"/dsl", putBody(sharedKakeibo(t)), map[string]string{"If-Match": `"1"`}); r.code != http.StatusCreated {
		t.Fatalf("put: %d %s", r.code, r.raw)
	}

	// u1 posts: author/authorName are set by the server (匿名 when omitted; data cannot carry them)
	if r = e.do("u1", http.MethodPost, recs, `{"data":{"amount":100,"author":"x"}}`, nil); r.code != http.StatusBadRequest {
		t.Fatalf("author in data must be an unknown field: %d %s", r.code, r.raw)
	}
	r = e.do("u1", http.MethodPost, recs, `{"data":{"amount":100,"memo":"u1の投稿"},"clientId":"c-1"}`, nil)
	if r.code != http.StatusCreated || r.body["author"] != "u1" || r.body["authorName"] != "匿名" {
		t.Fatalf("u1 create: %d %s", r.code, r.raw)
	}
	id1 := r.body["id"].(string)

	// u2 opens the same appId: the published DSL is installed (not the initial one)
	r = e.do("u2", http.MethodGet, app+"/dsl", nil, nil)
	if r.code != http.StatusOK || r.version() != 1 {
		t.Fatalf("u2 install: %d %s", r.code, r.raw)
	}
	if d, _ := r.body["dsl"].(map[string]any); len(d["entities"].([]any)) != 1 {
		t.Fatalf("u2 must get the published dsl: %s", r.raw)
	}
	// u2 sees u1's post and adds their own (same clientId as u1 must not collide)
	r = e.do("u2", http.MethodGet, recs, nil, nil)
	if items, _ := r.body["items"].([]any); len(items) != 1 || items[0].(map[string]any)["author"] != "u1" {
		t.Fatalf("u2 list: %d %s", r.code, r.raw)
	}
	r = e.do("u2", http.MethodPost, recs, `{"data":{"amount":200,"memo":"u2の投稿"},"clientId":"c-1","authorName":"  さくら "}`, nil)
	if r.code != http.StatusCreated || r.body["author"] != "u2" || r.body["authorName"] != "さくら" || r.body["id"] == id1 {
		t.Fatalf("u2 create: %d %s", r.code, r.raw)
	}
	id2 := r.body["id"].(string)
	if r = e.do("u2", http.MethodPost, recs, `{"data":{"amount":1},"authorName":"`+strings.Repeat("あ", 65)+`"}`, nil); r.code != http.StatusBadRequest {
		t.Fatalf("long authorName: %d %s", r.code, r.raw)
	}
	r = e.do("u1", http.MethodGet, recs+"?filterField=author&filterOp=eq&filterValue=u2", nil, nil)
	if items, _ := r.body["items"].([]any); r.code != http.StatusOK || len(items) != 1 || items[0].(map[string]any)["id"] != id2 {
		t.Fatalf("filter by author: %d %s", r.code, r.raw)
	}

	// only the author may update/delete; anyone may read
	if r = e.do("u2", http.MethodPatch, recs+"/"+id1, `{"data":{"amount":999}}`, nil); r.code != http.StatusForbidden || r.errCode() != "forbidden" {
		t.Fatalf("u2 patch u1's: %d %s", r.code, r.raw)
	}
	if r = e.do("u2", http.MethodDelete, recs+"/"+id1, nil, nil); r.code != http.StatusForbidden {
		t.Fatalf("u2 delete u1's: %d %s", r.code, r.raw)
	}
	if r = e.do("u2", http.MethodGet, recs+"/"+id1, nil, nil); r.code != http.StatusOK || r.body["data"].(map[string]any)["amount"] != float64(100) {
		t.Fatalf("u2 read u1's: %d %s", r.code, r.raw)
	}
	r = e.do("u1", http.MethodPatch, recs+"/"+id1, `{"data":{"amount":150}}`, nil)
	if r.code != http.StatusOK || r.body["author"] != "u1" || r.body["data"].(map[string]any)["amount"] != float64(150) {
		t.Fatalf("u1 patch own: %d %s", r.code, r.raw)
	}

	// delete-all on a shared entity removes only the caller's posts
	if r = e.do("u2", http.MethodDelete, recs+"?confirm=true", nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("u2 delete all: %d %s", r.code, r.raw)
	}
	r = e.do("u1", http.MethodGet, recs, nil, nil)
	if items, _ := r.body["items"].([]any); len(items) != 1 || items[0].(map[string]any)["id"] != id1 {
		t.Fatalf("after u2 delete all: %s", r.raw)
	}
	if r = e.do("u1", http.MethodDelete, recs+"/"+id1, nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("u1 delete own: %d %s", r.code, r.raw)
	}

	// u2 editing their installed copy does not overwrite u1's published dsl
	r = e.do("u2", http.MethodPatch, app+"/dsl", `[{"op":"replace","path":"/app/name","value":"u2版"}]`, map[string]string{"If-Match": `"1"`, "Content-Type": "application/json-patch+json"})
	if r.code != http.StatusCreated {
		t.Fatalf("u2 patch dsl: %d %s", r.code, r.raw)
	}
	r = e.do("u3", http.MethodGet, app+"/dsl", nil, nil)
	if d, _ := r.body["dsl"].(map[string]any); d["app"].(map[string]any)["name"] == "u2版" {
		t.Fatalf("u3 must install u1's version: %s", r.raw)
	}

	// private apps are untouched: another uid still gets the initial dsl and no records
	const priv = "/v1/apps/PRIVATE-APP-1"
	e.do("u1", http.MethodGet, priv+"/dsl", nil, nil)
	e.do("u1", http.MethodPut, priv+"/dsl", putBody(kakeibo(t)), map[string]string{"If-Match": `"1"`})
	r = e.do("u1", http.MethodPost, priv+"/entities/expense/records", `{"data":{"amount":1}}`, nil)
	if r.code != http.StatusCreated || r.body["author"] != nil || r.body["authorName"] != nil {
		t.Fatalf("private create must not carry author: %d %s", r.code, r.raw)
	}
	r = e.do("u2", http.MethodGet, priv+"/dsl", nil, nil)
	if d, _ := r.body["dsl"].(map[string]any); r.code != http.StatusOK || len(d["entities"].([]any)) != 0 {
		t.Fatalf("private app must not be installed by others: %d %s", r.code, r.raw)
	}
}
