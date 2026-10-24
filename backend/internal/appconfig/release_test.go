package appconfig

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

func TestReleaseRequestRoundTrip(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	mem := store.NewMemory()
	e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(mem))}
	const app = "/v1/apps/OWNER-KAKEIBO"
	e.ownApp("owner", app, kakeibo(t))

	// validation
	if r := e.do("owner", http.MethodPost, app+"/release-requests", `{"purpose":"  "}`, nil); r.code != http.StatusBadRequest {
		t.Fatalf("empty purpose: want 400, got %d %s", r.code, r.raw)
	}
	if r := e.do("owner", http.MethodPost, app+"/release-requests", map[string]string{"purpose": strings.Repeat("あ", 201)}, nil); r.code != http.StatusBadRequest {
		t.Fatalf("long purpose: want 400, got %d %s", r.code, r.raw)
	}
	// only the owner: someone else's (or a missing) app is 404
	if r := e.do("other", http.MethodPost, app+"/release-requests", `{"purpose":"毎日使う"}`, nil); r.code != http.StatusNotFound {
		t.Fatalf("request on someone else's app: want 404, got %d %s", r.code, r.raw)
	}
	if r := e.do("other", http.MethodGet, app+"/release-requests", nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("list on someone else's app: want 404, got %d %s", r.code, r.raw)
	}

	r := e.do("owner", http.MethodPost, app+"/release-requests", `{"purpose":" 毎日の家計に使いたい ","contact":"me@example.com"}`, nil)
	if r.code != http.StatusCreated || r.body["status"] != "received" || r.body["purpose"] != "毎日の家計に使いたい" ||
		r.body["contact"] != "me@example.com" || r.body["dslVersion"] != float64(2) || r.body["name"] != "家計簿" || r.body["appId"] != "OWNER-KAKEIBO" {
		t.Fatalf("create: %d %s", r.code, r.raw)
	}
	id, _ := r.body["id"].(string)
	if !strings.HasPrefix(id, "rr_") || r.body["dsl"] != nil {
		t.Fatalf("id / no dsl in response: %s", r.raw)
	}
	// the snapshot is the DSL of that version
	snap, err := mem.ReleaseSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	var d map[string]any
	if err := json.Unmarshal(snap.DSL, &d); err != nil || d["initialScreen"] != "home" || snap.OwnerUID != "owner" {
		t.Fatalf("snapshot: %v %s", err, snap.DSL)
	}

	// idempotent while open: same request comes back with 200, no duplicate
	r = e.do("owner", http.MethodPost, app+"/release-requests", `{"purpose":"もう一回"}`, nil)
	if r.code != http.StatusOK || r.body["id"] != id || r.body["purpose"] != "毎日の家計に使いたい" {
		t.Fatalf("second create: want existing with 200, got %d %s", r.code, r.raw)
	}

	// the admin moves status in the console -> visible to the owner
	if err := mem.SetReleaseStatus(id, store.ReleasePlanned); err != nil {
		t.Fatal(err)
	}
	r = e.do("owner", http.MethodGet, app+"/release-requests", nil, nil)
	items, _ := r.body["items"].([]any)
	if r.code != http.StatusOK || len(items) != 1 || items[0].(map[string]any)["status"] != "planned" {
		t.Fatalf("list: %d %s", r.code, r.raw)
	}
	if r = e.do("owner", http.MethodPost, app+"/release-requests", `{"purpose":"x"}`, nil); r.code != http.StatusOK || r.body["id"] != id {
		t.Fatalf("planned is still open: %d %s", r.code, r.raw)
	}

	// declined closes it: a new request can be filed
	if err := mem.SetReleaseStatus(id, store.ReleaseDeclined); err != nil {
		t.Fatal(err)
	}
	r = e.do("owner", http.MethodPost, app+"/release-requests", `{"purpose":"改良したので再申請"}`, nil)
	if r.code != http.StatusCreated || r.body["id"] == id {
		t.Fatalf("re-request after declined: %d %s", r.code, r.raw)
	}

	// GET /v1/release-requests: all of mine, newest first; others see nothing
	e.ownApp("owner", "/v1/apps/OWNER-WALK", walk(t))
	e.do("owner", http.MethodPost, "/v1/apps/OWNER-WALK/release-requests", `{"purpose":"散歩"}`, nil)
	r = e.do("owner", http.MethodGet, "/v1/release-requests", nil, nil)
	if items, _ = r.body["items"].([]any); len(items) != 3 {
		t.Fatalf("mine: %s", r.raw)
	}
	r = e.do("other", http.MethodGet, "/v1/release-requests", nil, nil)
	if items, _ = r.body["items"].([]any); r.code != http.StatusOK || len(items) != 0 {
		t.Fatalf("other's list must be empty: %d %s", r.code, r.raw)
	}
}
