package appconfig

import (
	"net/http"
	"testing"
)

// #1641: filterField=author on a private entity. iOS NewItems sends
// author neq <me> to every entity (build 2026110601 included, GET and POST);
// a private entity holds only the caller's records, so it answers an empty
// list instead of 400.
func TestRecordsAuthorFilterOnPrivateEntity(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		me, other := "pa-me"+sfx, "pa-other"+sfx
		app := "/v1/apps/PRIV-AUTHOR" + sfx
		recs := app + "/entities/expense/records"
		query := recs + "/query"
		e.ownApp(me, app, kakeibo(t))
		for range 2 {
			if r := e.do(me, http.MethodPost, recs, `{"data":{"amount":100}}`, nil); r.code != http.StatusCreated {
				t.Fatalf("create: %d %s", r.code, r.raw)
			}
		}
		count := func(r resp) int {
			t.Helper()
			if r.code != http.StatusOK {
				t.Fatalf("%d %s", r.code, r.raw)
			}
			items, _ := r.body["items"].([]any)
			return len(items)
		}

		// others' records (NewItems): none, by POST and by the deprecated GET
		if n := count(e.do(me, http.MethodPost, query, `{"filterField":"author","filterOp":"neq","filterValue":"`+me+`","pageSize":50}`, nil)); n != 0 {
			t.Errorf("POST neq me: %d items", n)
		}
		if n := count(e.do(me, http.MethodGet, recs+"?pageSize=50&filterField=author&filterOp=neq&filterValue="+me, nil, nil)); n != 0 {
			t.Errorf("GET neq me: %d items", n)
		}
		// my records: all of them
		if n := count(e.do(me, http.MethodPost, query, `{"filterField":"author","filterOp":"eq","filterValue":"`+me+`"}`, nil)); n != 2 {
			t.Errorf("eq me: %d items", n)
		}
		if n := count(e.do(me, http.MethodPost, query, `{"filterField":"author","filterOp":"eq","filterValue":"`+other+`"}`, nil)); n != 0 {
			t.Errorf("eq other: %d items", n)
		}
		if n := count(e.do(me, http.MethodPost, query, `{"filterField":"author","filterOp":"neq","filterValue":"`+other+`"}`, nil)); n != 2 {
			t.Errorf("neq other: %d items", n)
		}

		// privacy: another user naming me as author never reaches my records
		if r := e.do(other, http.MethodPost, query, `{"filterField":"author","filterOp":"eq","filterValue":"`+me+`"}`, nil); r.code == http.StatusOK {
			if n := count(r); n != 0 {
				t.Errorf("other sees my private records: %s", r.raw)
			}
		}

		// still rejected: other ops on author, unknown fields
		for _, body := range []string{
			`{"filterField":"author","filterOp":"gt","filterValue":"x"}`,
			`{"filterField":"author","filterOp":"in","filterValue":"myFollows"}`,
			`{"filterField":"nope","filterOp":"eq","filterValue":"1"}`,
		} {
			if r := e.do(me, http.MethodPost, query, body, nil); r.code != http.StatusBadRequest {
				t.Errorf("%s: %d %s, want 400", body, r.code, r.raw)
			}
		}
	})
}
