package appconfig

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

// POST .../records/query: the filter / sort / paging in the body give the
// same list as the (deprecated) GET query.

func TestRecordsQueryPost(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		me, alice, bob := "qme"+sfx, "qalice"+sfx, "qbob"+sfx
		app := "/v1/apps/QUERY-TL" + sfx
		recs := app + "/entities/posts/records"
		query := recs + "/query"
		e.ownApp(me, app, timelineDSL(t))
		for _, u := range []string{me, alice, bob} {
			e.do(u, http.MethodGet, app+"/dsl", nil, nil)
			if r := e.do(u, http.MethodPost, recs, `{"data":{"body":"hi from `+u+`"}}`, nil); r.code != http.StatusCreated {
				t.Fatalf("post by %s: %d %s", u, r.code, r.raw)
			}
		}

		// author neq (iOS NewItems): same as GET
		post := e.do(me, http.MethodPost, query, `{"filterField":"author","filterOp":"neq","filterValue":"`+me+`","pageSize":50}`, nil)
		get := e.do(me, http.MethodGet, recs+"?pageSize=50&filterField=author&filterOp=neq&filterValue="+me, nil, nil)
		if post.code != http.StatusOK || get.code != http.StatusOK || !slices.Equal(authorsOf(post), []string{alice, bob}) || !slices.Equal(authorsOf(get), authorsOf(post)) {
			t.Fatalf("neq: POST %d %s / GET %d %s", post.code, post.raw, get.code, get.raw)
		}
		if got := authorsOf(e.do(me, http.MethodPost, query, `{"filterField":"author","filterOp":"eq","filterValue":"`+alice+`"}`, nil)); !slices.Equal(got, []string{alice}) {
			t.Errorf("eq: %v", got)
		}
		// no filter: everything
		if got := authorsOf(e.do(me, http.MethodPost, query, `{}`, nil)); len(got) != 3 {
			t.Errorf("empty body: %v", got)
		}
		// follows filter (#270)
		if r := e.do(me, http.MethodPut, app+"/follows/"+alice, nil, nil); r.code != http.StatusOK {
			t.Fatalf("follow: %d %s", r.code, r.raw)
		}
		if got := authorsOf(e.do(me, http.MethodPost, query, `{"filterField":"author","filterOp":"in","filterValue":"myFollows"}`, nil)); !slices.Equal(got, []string{alice}) {
			t.Errorf("in myFollows: %v", got)
		}

		// paging: the token from the body form continues in the body form
		p1 := e.do(me, http.MethodPost, query, `{"pageSize":2}`, nil)
		tok, _ := p1.body["nextPageToken"].(string)
		if p1.code != http.StatusOK || len(authorsOf(p1)) != 2 || tok == "" {
			t.Fatalf("page 1: %d %s", p1.code, p1.raw)
		}
		p2 := e.do(me, http.MethodPost, query, `{"pageSize":2,"pageToken":"`+tok+`"}`, nil)
		if _, more := p2.body["nextPageToken"]; p2.code != http.StatusOK || len(authorsOf(p2)) != 1 || more {
			t.Fatalf("page 2: %d %s", p2.code, p2.raw)
		}
	})
}

func TestRecordsQueryPostRejects(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		me := "qrme" + sfx
		app := "/v1/apps/QUERY-RJ" + sfx
		query := app + "/entities/posts/records/query"
		e.ownApp(me, app, timelineDSL(t))
		e.do(me, http.MethodGet, app+"/dsl", nil, nil)
		for _, c := range []struct {
			path, body string
			code       int
		}{
			{query, ``, http.StatusBadRequest},
			{query, `[]`, http.StatusBadRequest},
			{query, `{"filterField":"author","filterOp":"eq","filterValue":"u","url":"x"}`, http.StatusBadRequest},
			{query, `{"filterField":"author","filterOp":"gt","filterValue":"u"}`, http.StatusBadRequest},
			{query, `{"filterField":"nope","filterOp":"eq","filterValue":"1"}`, http.StatusBadRequest},
			{query, `{"filterField":"author","filterOp":"in","filterValue":"someone"}`, http.StatusBadRequest},
			{query, `{"filterValue":1}`, http.StatusBadRequest},
			{query, `{"pageSize":0}`, http.StatusBadRequest},
			{query, `{"pageSize":"2"}`, http.StatusBadRequest},
			{query, `{"pageToken":"!!"}`, http.StatusBadRequest},
			{query, `{"sortDir":"up"}`, http.StatusBadRequest},
			{query + "?filterValue=u", `{"filterValue":"u"}`, http.StatusBadRequest},
			{query, `{"filterValue":"` + strings.Repeat("a", lookupBodyMaxBytes) + `"}`, http.StatusRequestEntityTooLarge},
			{app + "/entities/nope/records/query", `{}`, http.StatusNotFound},
		} {
			if r := e.do(me, http.MethodPost, c.path, c.body, nil); r.code != c.code {
				t.Errorf("%s %.60s: %d %s, want %d", c.path, c.body, r.code, r.raw, c.code)
			}
		}
		if r := e.do("", http.MethodPost, query, `{}`, nil); r.code != http.StatusUnauthorized {
			t.Errorf("anonymous: %d", r.code)
		}
	})
}

// Who may list is decided as for GET: a non-member of a participants-only app
// gets the same answer from both.
func TestRecordsQueryPostAuthzSameAsGet(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		owner, stranger := "qa-owner"+sfx, "qa-stranger"+sfx
		app := "/v1/apps/QUERY-SH" + sfx
		recs := app + "/entities/posts/records"
		e.ownApp(owner, app, []byte(strings.Replace(string(timelineDSL(t)), `"scope": "public"`, `"scope": "shared"`, 1)))
		e.do(owner, http.MethodGet, app+"/dsl", nil, nil)
		if r := e.do(owner, http.MethodPost, recs, `{"data":{"body":"owner only"}}`, nil); r.code != http.StatusCreated {
			t.Fatalf("post: %d %s", r.code, r.raw)
		}
		e.do(stranger, http.MethodGet, app+"/dsl", nil, nil)
		get := e.do(stranger, http.MethodGet, recs+"?filterField=author&filterOp=eq&filterValue="+owner, nil, nil)
		post := e.do(stranger, http.MethodPost, recs+"/query", `{"filterField":"author","filterOp":"eq","filterValue":"`+owner+`"}`, nil)
		if post.code != get.code || post.errCode() != get.errCode() || len(authorsOf(post)) != len(authorsOf(get)) {
			t.Fatalf("stranger: POST %d %s / GET %d %s", post.code, post.raw, get.code, get.raw)
		}
		if post.code == http.StatusOK && len(authorsOf(post)) != 0 {
			t.Fatalf("stranger sees records: %s", post.raw)
		}
		if got := authorsOf(e.do(owner, http.MethodPost, recs+"/query", `{"filterField":"author","filterOp":"eq","filterValue":"`+owner+`"}`, nil)); !slices.Equal(got, []string{owner}) {
			t.Errorf("owner: %v", got)
		}
	})
}
