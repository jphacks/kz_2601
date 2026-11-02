package appconfig

import (
	"context"
	"net/http"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

func newEnvWith(t *testing.T) (*env, *store.Memory) {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	m := store.NewMemory()
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(m))}, m
}

func listingIDs(r resp) map[string]map[string]any {
	out := map[string]map[string]any{}
	items, _ := r.body["items"].([]any)
	for _, it := range items {
		m := it.(map[string]any)
		out[m["id"].(string)] = m
	}
	return out
}

// #321 done criteria: report -> operator unpublishes -> others stop seeing it.
func TestReportAndUnpublishListing(t *testing.T) {
	e, st := newEnvWith(t)
	const id = "OWNER-KAKEIBO"
	e.ownApp("owner", "/v1/apps/"+id, kakeibo(t))
	if r := e.do("owner", http.MethodPost, "/v1/apps/"+id+"/publish", `{"authorName":"こた"}`, nil); r.code != http.StatusOK {
		t.Fatalf("publish: %d %s", r.code, r.raw)
	}

	// validation
	for _, body := range []string{
		`{"kind":"listing","id":"` + id + `","reason":"nope"}`,
		`{"kind":"what","id":"` + id + `","reason":"spam"}`,
		`{"kind":"listing","id":"../x","reason":"spam"}`,
	} {
		if r := e.do("viewer", http.MethodPost, "/v1/reports", body, nil); r.code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", body, r.code, r.raw)
		}
	}
	if r := e.do("viewer", http.MethodPost, "/v1/reports", `{"kind":"listing","id":"MISSING","reason":"spam"}`, nil); r.code != http.StatusNotFound {
		t.Fatalf("missing target: %d %s", r.code, r.raw)
	}
	if r := e.do("owner", http.MethodPost, "/v1/reports", `{"kind":"listing","id":"`+id+`","reason":"spam"}`, nil); r.code != http.StatusBadRequest {
		t.Fatalf("own listing: %d %s", r.code, r.raw)
	}

	// report: 201 then 200 created:false
	body := `{"kind":"listing","id":"` + id + `","reason":"sexual"}`
	r := e.do("viewer", http.MethodPost, "/v1/reports", body, nil)
	if r.code != http.StatusCreated || r.body["created"] != true {
		t.Fatalf("report: %d %s", r.code, r.raw)
	}
	if r = e.do("viewer", http.MethodPost, "/v1/reports", body, nil); r.code != http.StatusOK || r.body["created"] != false {
		t.Fatalf("again: %d %s", r.code, r.raw)
	}
	reps, _ := st.ListReports(context.Background(), 10)
	if len(reps) != 1 || reps[0].ReporterUID != "viewer" || reps[0].TargetUID != "owner" || reps[0].Reason != "sexual" || reps[0].Status != "open" {
		t.Fatalf("stored: %+v", reps)
	}

	// operator unpublishes (cmd/moderate)
	if err := st.SetCatalogHidden(context.Background(), id, true, "不適切な内容のため"); err != nil {
		t.Fatal(err)
	}
	if got := listingIDs(e.do("viewer", http.MethodGet, "/v1/public-apps", nil, nil)); got[id] != nil {
		t.Fatalf("hidden listing must not be listed: %v", got)
	}
	r = e.do("viewer", http.MethodPost, "/v1/public-apps/"+id+"/install", nil, nil)
	if r.code != http.StatusForbidden || r.errCode() != CodeUnpublished {
		t.Fatalf("install hidden: %d %s", r.code, r.raw)
	}
	// the publisher still sees it, with the reason; republishing keeps it hidden
	e.do("owner", http.MethodPost, "/v1/apps/"+id+"/publish", `{}`, nil)
	mine := listingIDs(e.do("owner", http.MethodGet, "/v1/public-apps", nil, nil))[id]
	if mine == nil || mine["hidden"] != true || mine["hiddenReason"] != "不適切な内容のため" {
		t.Fatalf("owner view: %v", mine)
	}
	if err := st.SetCatalogHidden(context.Background(), id, false, ""); err != nil {
		t.Fatal(err)
	}
	if got := listingIDs(e.do("viewer", http.MethodGet, "/v1/public-apps", nil, nil)); got[id] == nil || got[id]["hidden"] != nil {
		t.Fatalf("restored: %v", got)
	}
}

func TestReportQuota(t *testing.T) {
	e, st := newEnvWith(t)
	for i := range ReportsPerDay + 1 {
		id := "L" + string(rune('A'+i%26)) + string(rune('A'+i/26))
		if _, err := st.PutCatalogApp(context.Background(), "owner", store.CatalogApp{ID: id, DSL: []byte(`{}`)}); err != nil {
			t.Fatal(err)
		}
		r := e.do("viewer", http.MethodPost, "/v1/reports", `{"kind":"listing","id":"`+id+`","reason":"spam"}`, nil)
		want := http.StatusCreated
		if i == ReportsPerDay {
			want = http.StatusTooManyRequests
		}
		if r.code != want {
			t.Fatalf("report %d: %d %s", i, r.code, r.raw)
		}
	}
}

func TestBlockHidesPostsAndListings(t *testing.T) {
	e, _ := newEnvWith(t)
	const app = "/v1/apps/SHARED-APP-1"
	recs := app + "/entities/expense/records"
	e.ownApp("u1", app, sharedKakeibo(t))
	r := e.do("u1", http.MethodPost, recs, `{"data":{"amount":100},"authorName":"いち"}`, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("u1 post: %d %s", r.code, r.raw)
	}
	recID := r.body["id"].(string)
	e.do("u2", http.MethodGet, app+"/dsl", nil, nil)
	e.do("u2", http.MethodPost, recs, `{"data":{"amount":200}}`, nil)
	count := func(uid string) int {
		r := e.do(uid, http.MethodGet, recs, nil, nil)
		return len(r.body["items"].([]any))
	}
	if count("u2") != 2 {
		t.Fatal("before block")
	}

	// report a post
	r = e.do("u2", http.MethodPost, "/v1/reports", `{"kind":"record","appId":"SHARED-APP-1","entityId":"expense","recordId":"`+recID+`","reason":"harassment"}`, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("report post: %d %s", r.code, r.raw)
	}

	// block
	for _, body := range []string{`{}`, `{"uid":"u2"}`, `{"uid":"u1","listingId":"x"}`} {
		if r = e.do("u2", http.MethodPost, "/v1/me/blocks", body, nil); r.code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", body, r.code, r.raw)
		}
	}
	if r = e.do("u2", http.MethodPost, "/v1/me/blocks", `{"uid":"u1","name":"いち"}`, nil); r.code != http.StatusCreated || r.body["uid"] != "u1" {
		t.Fatalf("block: %d %s", r.code, r.raw)
	}
	if count("u2") != 1 || count("u1") != 2 {
		t.Fatalf("after block: u2 %d u1 %d", count("u2"), count("u1"))
	}
	r = e.do("u2", http.MethodGet, "/v1/me/blocks", nil, nil)
	if items := r.body["items"].([]any); len(items) != 1 || items[0].(map[string]any)["name"] != "いち" {
		t.Fatalf("list: %s", r.raw)
	}

	// block by listing: the publisher's listings disappear
	const pub = "U3-APP"
	e.ownApp("u3", "/v1/apps/"+pub, kakeibo(t))
	e.do("u3", http.MethodPost, "/v1/apps/"+pub+"/publish", `{"authorName":"さん"}`, nil)
	if r = e.do("u2", http.MethodPost, "/v1/me/blocks", `{"listingId":"`+pub+`"}`, nil); r.code != http.StatusCreated || r.body["name"] != "さん" {
		t.Fatalf("block by listing: %d %s", r.code, r.raw)
	}
	if got := listingIDs(e.do("u2", http.MethodGet, "/v1/public-apps", nil, nil)); got[pub] != nil {
		t.Fatalf("blocked publisher listed: %v", got)
	}

	// unblock
	if r = e.do("u2", http.MethodDelete, "/v1/me/blocks/u1", nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("unblock: %d", r.code)
	}
	if count("u2") != 2 {
		t.Fatal("after unblock")
	}
}

func TestUnpublishSharedApp(t *testing.T) {
	e, st := newEnvWith(t)
	const app = "/v1/apps/SHARED-APP-2"
	recs := app + "/entities/expense/records"
	e.ownApp("u1", app, sharedKakeibo(t))
	e.do("u1", http.MethodPost, recs, `{"data":{"amount":100}}`, nil)
	tok, _ := e.do("u1", http.MethodPost, app+"/share", nil, nil).body["token"].(string)
	e.do("u2", http.MethodGet, app+"/dsl", nil, nil) // u2 joined before the operator stepped in
	if r := e.do("u2", http.MethodPost, "/v1/reports", `{"kind":"app","appId":"SHARED-APP-2","reason":"illegal"}`, nil); r.code != http.StatusCreated {
		t.Fatalf("report app: %d %s", r.code, r.raw)
	}
	if err := st.SetPublicAppHidden(context.Background(), "SHARED-APP-2", true, "違法な内容のため"); err != nil {
		t.Fatal(err)
	}
	for _, uid := range []string{"u1", "u2"} {
		r := e.do(uid, http.MethodGet, recs, nil, nil)
		if r.code != http.StatusForbidden || r.errCode() != CodeUnpublished {
			t.Fatalf("%s records: %d %s", uid, r.code, r.raw)
		}
	}
	if r := e.do("u3", http.MethodGet, "/v1/shares/"+tok, nil, nil); r.code != http.StatusForbidden {
		t.Fatalf("share preview: %d %s", r.code, r.raw)
	}
	// a new user opening the appId does not get the unpublished DSL
	r := e.do("u4", http.MethodGet, app+"/dsl", nil, nil)
	if d, _ := r.body["dsl"].(map[string]any); len(d["entities"].([]any)) != 0 {
		t.Fatalf("seeded from unpublished app: %s", r.raw)
	}
}
