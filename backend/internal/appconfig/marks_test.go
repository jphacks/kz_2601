package appconfig

import (
	"net/http"
	"os"
	"testing"
)

func recipes(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../../docs/dsl/examples/recipes.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// markOf reads marks.<id> of a record body: count, mine, and whether it is there.
func markOf(rec map[string]any, id string) (int, bool, bool) {
	marks, _ := rec["marks"].(map[string]any)
	m, ok := marks[id].(map[string]any)
	if !ok {
		return 0, false, false
	}
	c, _ := m["count"].(float64)
	mine, _ := m["mine"].(bool)
	return int(c), mine, true
}

func wantMark(t *testing.T, what string, rec map[string]any, count int, mine bool) {
	t.Helper()
	c, m, ok := markOf(rec, "save")
	if !ok || c != count || m != mine {
		t.Fatalf("%s: save = {count:%d mine:%v present:%v}, want {count:%d mine:%v}; record %v", what, c, m, ok, count, mine, rec)
	}
}

// #255: two accounts mark the same post once each; the count stays 0..2.
func TestMarksOncePerPerson(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		u1, u2 := "mk1"+sfx, "mk2"+sfx
		app := "/v1/apps/MARKS-APP" + sfx
		recs := app + "/entities/recipes/records"
		e.ownApp(u1, app, recipes(t))
		r := e.do(u1, http.MethodPost, recs, `{"data":{"title":"肉じゃが","minutes":30}}`, nil)
		if r.code != http.StatusCreated {
			t.Fatalf("create: %d %s", r.code, r.raw)
		}
		id := r.body["id"].(string)
		mark := recs + "/" + id + "/marks/save"
		e.do(u2, http.MethodGet, app+"/dsl", nil, nil)

		// nobody yet: every declared mark is listed with count 0
		r = e.do(u2, http.MethodGet, recs, nil, nil)
		wantMark(t, "fresh list", r.body["items"].([]any)[0].(map[string]any), 0, false)

		steps := []struct {
			uid, method string
			count       int
			mine        bool
		}{
			{u1, http.MethodPut, 1, true},
			{u1, http.MethodPut, 1, true}, // idempotent: a second tap does not add
			{u2, http.MethodPut, 2, true},
			{u1, http.MethodDelete, 1, false},
			{u1, http.MethodDelete, 1, false},
			{u2, http.MethodDelete, 0, false},
			{u2, http.MethodDelete, 0, false}, // never below 0
			{u2, http.MethodPut, 1, true},
		}
		for i, s := range steps {
			r := e.do(s.uid, s.method, mark, nil, nil)
			if r.code != http.StatusOK {
				t.Fatalf("step %d %s %s: %d %s", i, s.uid, s.method, r.code, r.raw)
			}
			if c, _ := r.body["count"].(float64); int(c) != s.count || r.body["mine"] != s.mine {
				t.Fatalf("step %d %s %s: %s, want count %d mine %v", i, s.uid, s.method, r.raw, s.count, s.mine)
			}
		}
		// each account sees its own "mine" on list and get
		for _, c := range []struct {
			uid  string
			mine bool
		}{{u1, false}, {u2, true}} {
			r = e.do(c.uid, http.MethodGet, recs, nil, nil)
			wantMark(t, c.uid+" list", r.body["items"].([]any)[0].(map[string]any), 1, c.mine)
			r = e.do(c.uid, http.MethodGet, recs+"/"+id, nil, nil)
			wantMark(t, c.uid+" get", r.body, 1, c.mine)
		}
		// updating the post keeps its marks (patch response carries them too)
		r = e.do(u1, http.MethodPatch, recs+"/"+id, `{"data":{"minutes":25}}`, nil)
		if r.code != http.StatusOK {
			t.Fatalf("patch: %d %s", r.code, r.raw)
		}
		wantMark(t, "patch", r.body, 1, false)

		// deleting the post removes its marks with it
		if r = e.do(u1, http.MethodDelete, recs+"/"+id, nil, nil); r.code != http.StatusNoContent {
			t.Fatalf("delete: %d %s", r.code, r.raw)
		}
		if r = e.do(u2, http.MethodPut, mark, nil, nil); r.code != http.StatusNotFound {
			t.Fatalf("mark on a deleted record: %d %s", r.code, r.raw)
		}
	})
}

func TestMarkErrorsAndPrivate(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/MARKS-APP-ERR"
	recs := app + "/entities/recipes/records"
	e.ownApp("u1", app, recipes(t))
	id := e.do("u1", http.MethodPost, recs, `{"data":{"title":"カレー"}}`, nil).body["id"].(string)
	for _, c := range []struct {
		path string
		code int
	}{
		{recs + "/" + id + "/marks/like", http.StatusNotFound},       // not declared
		{recs + "/rec_MISSING/marks/save", http.StatusNotFound},      // no such record
		{recs + "/" + id + "/marks/Bad%20Id", http.StatusBadRequest}, // invalid id
	} {
		if r := e.do("u1", http.MethodPut, c.path, nil, nil); r.code != c.code {
			t.Fatalf("PUT %s: %d %s, want %d", c.path, r.code, r.raw, c.code)
		}
	}

	// entities without marks do not grow a marks key
	const priv = "/v1/apps/MARKS-APP-PRIV"
	e.ownApp("u1", priv, kakeibo(t))
	e.do("u1", http.MethodPost, priv+"/entities/expense/records", `{"data":{"amount":100}}`, nil)
	r := e.do("u1", http.MethodGet, priv+"/entities/expense/records", nil, nil)
	if _, ok := r.body["items"].([]any)[0].(map[string]any)["marks"]; ok {
		t.Fatalf("private record must not carry marks: %s", r.raw)
	}
	if r := e.do("u1", http.MethodPut, priv+"/entities/expense/records/x/marks/save", nil, nil); r.code != http.StatusNotFound {
		t.Fatalf("mark on private entity: %d %s", r.code, r.raw)
	}
}
