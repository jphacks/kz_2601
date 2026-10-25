package appconfig

import (
	"net/http"
	"testing"
)

func listed(e *env, id string) bool {
	r := e.do("finder", http.MethodGet, "/v1/public-apps?pageSize=200", nil, nil)
	for _, it := range r.body["items"].([]any) {
		if it.(map[string]any)["id"] == id {
			return true
		}
	}
	return false
}

// #180: deleting an app physically deletes the caller's app (versions,
// records) and their share links / "探す" listing; copies other users
// installed from the catalog stay. (Subcollection cleanup in Firestore is
// checked directly in store.TestFirestorePurgeApp.)
func TestDeleteAppPurgesAppSharesAndListing(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		owner, finder, other := "owner"+sfx, "finder"+sfx, "other"+sfx
		id := "DEL-KAKEIBO" + sfx
		app := "/v1/apps/" + id
		e.ownApp(owner, app, kakeibo(t))
		e.do(owner, http.MethodPost, app+"/entities/expense/records", `{"data":{"amount":100}}`, nil)
		tok, _ := e.do(owner, http.MethodPost, app+"/share", nil, nil).body["token"].(string)
		tok2, _ := e.do(owner, http.MethodPost, app+"/share", nil, nil).body["token"].(string)
		if r := e.do(owner, http.MethodPost, app+"/publish", `{}`, nil); r.code != http.StatusOK {
			t.Fatalf("publish: %d %s", r.code, r.raw)
		}
		inst := e.do(finder, http.MethodPost, "/v1/public-apps/"+id+"/install", nil, nil)
		copyID, _ := inst.body["id"].(string)

		// someone else's app is not in the caller's tree
		if r := e.do(other, http.MethodDelete, app, nil, nil); r.code != http.StatusNotFound {
			t.Fatalf("delete someone else's app: want 404, got %d %s", r.code, r.raw)
		}
		if r := e.do(owner, http.MethodDelete, app, nil, nil); r.code != http.StatusNoContent {
			t.Fatalf("delete: %d %s", r.code, r.raw)
		}
		if r := e.do(owner, http.MethodGet, app, nil, nil); r.code != http.StatusNotFound {
			t.Fatalf("app after delete: want 404, got %d %s", r.code, r.raw)
		}
		if r := e.do(owner, http.MethodGet, "/v1/apps", nil, nil); len(r.body["items"].([]any)) != 0 {
			t.Fatalf("list after delete: %s", r.raw)
		}
		for _, tk := range []string{tok, tok2} {
			if r := e.do(finder, http.MethodGet, "/v1/shares/"+tk, nil, nil); r.code != http.StatusNotFound {
				t.Fatalf("share after delete: want 404, got %d", r.code)
			}
		}
		if listed(e, id) {
			t.Fatal("listing must be deleted")
		}
		if r := e.do(finder, http.MethodGet, "/v1/apps/"+copyID, nil, nil); r.code != http.StatusOK {
			t.Fatalf("installed copy must stay: %d %s", r.code, r.raw)
		}
		if r := e.do(owner, http.MethodDelete, app, nil, nil); r.code != http.StatusNotFound {
			t.Fatalf("delete again: want 404, got %d %s", r.code, r.raw)
		}
		// opening the appId again seeds a blank app: no old versions or records survive
		e.do(owner, http.MethodGet, app+"/dsl", nil, nil)
		if r := e.do(owner, http.MethodGet, app+"/versions", nil, nil); len(r.body["items"].([]any)) != 1 {
			t.Fatalf("versions must not survive: %s", r.raw)
		}
		if r := e.do(owner, http.MethodGet, app+"/entities/expense/records", nil, nil); r.code == http.StatusOK && len(r.body["items"].([]any)) != 0 {
			t.Fatalf("records must not survive: %s", r.raw)
		}
	})
}

// A member who joined the owner's appId only deletes their own copy; the
// owner's app, share, listing and shared records stay. When the owner deletes,
// the member's copy stays.
func TestDeleteJoinedApp(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		owner, friend := "owner"+sfx, "friend"+sfx
		id := "DEL-BOARD" + sfx
		app := "/v1/apps/" + id
		e.ownApp(owner, app, sharedKakeibo(t))
		e.do(owner, http.MethodPost, app+"/entities/expense/records", `{"data":{"amount":100,"memo":"みんなへ"}}`, nil)
		e.do(owner, http.MethodPost, app+"/publish", `{}`, nil)
		tok, _ := e.do(owner, http.MethodPost, app+"/share", nil, nil).body["token"].(string)
		if r := e.do(friend, http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil); r.body["joined"] != true {
			t.Fatalf("join: %s", r.raw)
		}
		if r := e.do(friend, http.MethodDelete, app, nil, nil); r.code != http.StatusNoContent {
			t.Fatalf("member delete: %d %s", r.code, r.raw)
		}
		if !listed(e, id) {
			t.Fatal("owner's listing must stay")
		}
		if r := e.do(friend, http.MethodGet, "/v1/shares/"+tok, nil, nil); r.code != http.StatusOK {
			t.Fatalf("owner's share must stay: %d", r.code)
		}
		if r := e.do(owner, http.MethodGet, app+"/entities/expense/records", nil, nil); len(r.body["items"].([]any)) != 1 {
			t.Fatalf("owner's shared records must stay: %s", r.raw)
		}

		e.do(friend, http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil)
		if r := e.do(owner, http.MethodDelete, app, nil, nil); r.code != http.StatusNoContent {
			t.Fatalf("owner delete: %d %s", r.code, r.raw)
		}
		if r := e.do(friend, http.MethodGet, app, nil, nil); r.code != http.StatusOK {
			t.Fatalf("member's copy must stay: %d %s", r.code, r.raw)
		}
	})
}
