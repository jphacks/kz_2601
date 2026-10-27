package appconfig

import (
	"net/http"
	"strings"
	"testing"
)

func appItems(e *env, uid string) map[string]map[string]any {
	e.t.Helper()
	r := e.do(uid, http.MethodGet, "/v1/apps", nil, nil)
	out := map[string]map[string]any{}
	for _, it := range r.body["items"].([]any) {
		m := it.(map[string]any)
		out[m["id"].(string)] = m
	}
	return out
}

func dslName(e *env, uid, app string) (string, int) {
	e.t.Helper()
	r := e.do(uid, http.MethodGet, app+"/dsl", nil, nil)
	d, _ := r.body["dsl"].(map[string]any)
	a, _ := d["app"].(map[string]any)
	n, _ := a["name"].(string)
	return n, r.version()
}

// #277: a rename is written into dsl.app.name as a new version, so the next
// voice edit (which copies dsl.app.name into the app document) keeps it.
func TestRenameApp(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		uid, id := "renamer"+sfx, "RENAME-KAKEIBO"+sfx
		app := "/v1/apps/" + id
		e.ownApp(uid, app, kakeibo(t))

		r := e.do(uid, http.MethodPatch, app, `{"name":"  仕事の家計簿 "}`, nil)
		if r.code != http.StatusOK || r.body["name"] != "仕事の家計簿" {
			t.Fatalf("rename: %d %s", r.code, r.raw)
		}
		name, v := dslName(e, uid, app)
		if name != "仕事の家計簿" || v != 3 {
			t.Fatalf("dsl after rename: name=%q version=%d", name, v)
		}
		// same name again: no new version
		e.do(uid, http.MethodPatch, app, `{"name":"仕事の家計簿"}`, nil)
		if _, v2 := dslName(e, uid, app); v2 != v {
			t.Fatalf("no-op rename bumped version %d -> %d", v, v2)
		}
		// a later edit keeps the new name
		if r := e.do(uid, http.MethodPatch, app+"/dsl", `{"patch":[{"op":"test","path":"/app/name","value":"仕事の家計簿"}]}`,
			map[string]string{"If-Match": `"3"`}); r.code != http.StatusCreated {
			t.Fatalf("patch dsl: %d %s", r.code, r.raw)
		}
		if got := appItems(e, uid)[id]["name"]; got != "仕事の家計簿" {
			t.Fatalf("name after edit: %v", got)
		}

		for _, bad := range []string{`{"name":"   "}`, `{"name":"` + strings.Repeat("あ", 41) + `"}`} {
			if r := e.do(uid, http.MethodPatch, app, bad, nil); r.code != http.StatusBadRequest {
				t.Fatalf("bad name %s: want 400, got %d", bad, r.code)
			}
		}
		if r := e.do("other"+sfx, http.MethodPatch, app, `{"name":"x"}`, nil); r.code != http.StatusNotFound {
			t.Fatalf("rename someone else's app: want 404, got %d", r.code)
		}
		// status-only patch still works and keeps the name
		if r := e.do(uid, http.MethodPatch, app, `{"status":"archived"}`, nil); r.code != http.StatusOK ||
			r.body["status"] != "archived" || r.body["name"] != "仕事の家計簿" {
			t.Fatalf("archive: %d %s", r.code, r.raw)
		}
	})
}

// #277: duplicate copies the screens to a new appId without records, even for
// apps with shared entities (unlike share import, which joins the same appId).
func TestDuplicateApp(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		uid := "dup" + sfx
		for _, tc := range []struct {
			id  string
			dsl []byte
		}{{"DUP-KAKEIBO" + sfx, kakeibo(t)}, {"DUP-SHARED" + sfx, sharedKakeibo(t)}} {
			app := "/v1/apps/" + tc.id
			e.ownApp(uid, app, tc.dsl)
			e.do(uid, http.MethodPost, app+"/entities/expense/records", `{"data":{"amount":100}}`, nil)

			r := e.do(uid, http.MethodPost, app+"/duplicate", `{"name":"家計簿 (仕事)"}`, nil)
			if r.code != http.StatusCreated {
				t.Fatalf("duplicate %s: %d %s", tc.id, r.code, r.raw)
			}
			newID, _ := r.body["id"].(string)
			if newID == "" || newID == tc.id || r.body["name"] != "家計簿 (仕事)" || r.body["currentVersion"] != float64(2) {
				t.Fatalf("duplicate %s: %s", tc.id, r.raw)
			}
			if name, _ := dslName(e, uid, "/v1/apps/"+newID); name != "家計簿 (仕事)" {
				t.Fatalf("copy dsl name: %q", name)
			}
			if r := e.do(uid, http.MethodGet, "/v1/apps/"+newID+"/entities/expense/records", nil, nil); len(r.body["items"].([]any)) != 0 {
				t.Fatalf("copy must start without records: %s", r.raw)
			}
			if r := e.do(uid, http.MethodGet, app+"/entities/expense/records", nil, nil); len(r.body["items"].([]any)) != 1 {
				t.Fatalf("source records must stay: %s", r.raw)
			}
		}
		// no name: keeps the source name
		r := e.do(uid, http.MethodPost, "/v1/apps/DUP-KAKEIBO"+sfx+"/duplicate", nil, nil)
		if r.code != http.StatusCreated || r.body["name"] != "家計簿" {
			t.Fatalf("duplicate without name: %d %s", r.code, r.raw)
		}
		if r := e.do("other"+sfx, http.MethodPost, "/v1/apps/DUP-KAKEIBO"+sfx+"/duplicate", nil, nil); r.code != http.StatusNotFound {
			t.Fatalf("duplicate someone else's app: want 404, got %d", r.code)
		}
		if n := len(appItems(e, uid)); n != 5 {
			t.Fatalf("want 2 sources + 3 copies, got %d", n)
		}
	})
}

// #277: the manual order is stored as sortIndex and does not touch updatedAt.
func TestReorderApps(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		uid := "sorter" + sfx
		a, b, c := "ORD-A"+sfx, "ORD-B"+sfx, "ORD-C"+sfx
		for _, id := range []string{a, b, c} {
			e.ownApp(uid, "/v1/apps/"+id, kakeibo(t))
		}
		before := appItems(e, uid)

		if r := e.do(uid, http.MethodPut, "/v1/apps/order", map[string]any{"ids": []string{c, a, "MISSING" + sfx, b}}, nil); r.code != http.StatusNoContent {
			t.Fatalf("reorder: %d %s", r.code, r.raw)
		}
		after := appItems(e, uid)
		for id, want := range map[string]float64{c: 1, a: 2, b: 4} {
			if got := after[id]["sortIndex"]; got != want {
				t.Fatalf("%s sortIndex: want %v, got %v", id, want, got)
			}
			if after[id]["updatedAt"] != before[id]["updatedAt"] {
				t.Fatalf("%s updatedAt changed by reorder", id)
			}
		}
		// other users are unaffected
		if r := e.do("other"+sfx, http.MethodPut, "/v1/apps/order", map[string]any{"ids": []string{a}}, nil); r.code != http.StatusNoContent {
			t.Fatalf("reorder other: %d", r.code)
		}
		if got := appItems(e, uid)[a]["sortIndex"]; got != float64(2) {
			t.Fatalf("another user's reorder touched mine: %v", got)
		}
		for _, bad := range []any{map[string]any{"ids": []string{a, a}}, map[string]any{"ids": []string{"null"}}} {
			if r := e.do(uid, http.MethodPut, "/v1/apps/order", bad, nil); r.code != http.StatusBadRequest {
				t.Fatalf("bad ids %v: want 400, got %d", bad, r.code)
			}
		}
	})
}
