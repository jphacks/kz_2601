package appconfig

import (
	"net/http"
	"strings"
	"testing"
)

// membersKakeibo is the kakeibo example with expense public and edit:members (#252).
func membersKakeibo(t *testing.T) []byte {
	t.Helper()
	const anchor = `"id": "expense",`
	raw := string(kakeibo(t))
	if !strings.Contains(raw, anchor) {
		t.Fatal("kakeibo example changed")
	}
	return []byte(strings.Replace(raw, anchor, anchor+` "scope": "public", "edit": "members",`, 1))
}

func TestSharedRecordsMembersEdit(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/MEMBERS-APP-1"
	recs := app + "/entities/expense/records"

	e.do("u1", http.MethodGet, app+"/dsl", nil, nil)
	if r := e.do("u1", http.MethodPut, app+"/dsl", putBody(membersKakeibo(t)), map[string]string{"If-Match": `"1"`}); r.code != http.StatusCreated {
		t.Fatalf("put: %d %s", r.code, r.raw)
	}
	r := e.do("u1", http.MethodPost, recs, `{"data":{"amount":100,"memo":"お風呂掃除"}}`, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("u1 create: %d %s", r.code, r.raw)
	}
	id := r.body["id"].(string)

	// another participant may update, and the author stays u1
	e.do("u2", http.MethodGet, app+"/dsl", nil, nil)
	r = e.do("u2", http.MethodPatch, recs+"/"+id, `{"data":{"amount":120}}`, nil)
	if r.code != http.StatusOK || r.body["author"] != "u1" || r.body["data"].(map[string]any)["amount"] != float64(120) {
		t.Fatalf("u2 patch on members entity: %d %s", r.code, r.raw)
	}
	if memo := r.body["data"].(map[string]any)["memo"]; memo != "お風呂掃除" {
		t.Fatalf("patch must merge, memo=%v", memo)
	}
	// deleting is still the author's alone
	if r = e.do("u2", http.MethodDelete, recs+"/"+id, nil, nil); r.code != http.StatusForbidden || r.errCode() != "forbidden" {
		t.Fatalf("u2 delete on members entity: %d %s", r.code, r.raw)
	}
	if r = e.do("u2", http.MethodDelete, recs+"?confirm=true", nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("u2 delete all: %d %s", r.code, r.raw)
	}
	if r = e.do("u1", http.MethodGet, recs+"/"+id, nil, nil); r.code != http.StatusOK {
		t.Fatalf("delete-all by u2 must keep u1's record: %d %s", r.code, r.raw)
	}
	if r = e.do("u1", http.MethodDelete, recs+"/"+id, nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("u1 delete own: %d %s", r.code, r.raw)
	}
}

func TestMembersEditRejectsUnknownValue(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/MEMBERS-APP-2"
	bad := strings.Replace(string(membersKakeibo(t)), `"edit": "members"`, `"edit": "everyone"`, 1)
	e.do("u1", http.MethodGet, app+"/dsl", nil, nil)
	if r := e.do("u1", http.MethodPut, app+"/dsl", putBody([]byte(bad)), map[string]string{"If-Match": `"1"`}); r.code != http.StatusUnprocessableEntity {
		t.Fatalf("edit=everyone must be rejected: %d %s", r.code, r.raw)
	}
}
