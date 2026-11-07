package appconfig

import (
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
)

func timelineDSL(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../../docs/dsl/examples/timeline.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func authorsOf(r resp) []string {
	var out []string
	items, _ := r.body["items"].([]any)
	for _, it := range items {
		a, _ := it.(map[string]any)["author"].(string)
		out = append(out, a)
	}
	slices.Sort(out)
	return out
}

// #270: follow people in a public app; the list filter keeps their posts only.
func TestFollowsTimeline(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		me, alice, bob := "fme"+sfx, "falice"+sfx, "fbob"+sfx
		app := "/v1/apps/FOLLOW-TL" + sfx
		recs := app + "/entities/posts/records"
		e.ownApp(me, app, timelineDSL(t))
		for _, u := range []string{me, alice, bob} {
			e.do(u, http.MethodGet, app+"/dsl", nil, nil)
			if r := e.do(u, http.MethodPost, recs, `{"data":{"body":"hi from `+u+`"}}`, nil); r.code != http.StatusCreated {
				t.Fatalf("post by %s: %d %s", u, r.code, r.raw)
			}
		}
		following := recs + "?filterField=author&filterOp=in&filterValue=myFollows"
		if r := e.do(me, http.MethodGet, following, nil, nil); r.code != http.StatusOK || len(authorsOf(r)) != 0 {
			t.Fatalf("nobody followed yet: %d %s", r.code, r.raw)
		}

		steps := []struct {
			method, target string
			following      bool
			count          float64
		}{
			{http.MethodPut, alice, true, 1},
			{http.MethodPut, alice, true, 1}, // idempotent
			{http.MethodPut, bob, true, 2},
			{http.MethodDelete, bob, false, 1},
			{http.MethodDelete, bob, false, 1},
		}
		for i, s := range steps {
			r := e.do(me, s.method, app+"/follows/"+s.target, nil, nil)
			if r.code != http.StatusOK || r.body["following"] != s.following || r.body["count"] != s.count {
				t.Fatalf("step %d: %d %s", i, r.code, r.raw)
			}
		}
		if r := e.do(me, http.MethodGet, app+"/follows", nil, nil); r.code != http.StatusOK || len(r.body["uids"].([]any)) != 1 || r.body["uids"].([]any)[0] != alice {
			t.Fatalf("list follows: %d %s", r.code, r.raw)
		}
		if got := authorsOf(e.do(me, http.MethodGet, following, nil, nil)); !slices.Equal(got, []string{alice}) {
			t.Errorf("in: %v, want alice only", got)
		}
		notFollowing := strings.Replace(following, "Op=in", "Op=nin", 1)
		if got := authorsOf(e.do(me, http.MethodGet, notFollowing, nil, nil)); !slices.Equal(got, []string{bob, me}) {
			t.Errorf("nin: %v, want bob and me", got)
		}
		// follows are per person: alice follows nobody
		if got := authorsOf(e.do(alice, http.MethodGet, following, nil, nil)); len(got) != 0 {
			t.Errorf("alice's timeline: %v", got)
		}

		for _, c := range []struct {
			name, method, path string
			code               int
			reason             string
		}{
			{"self", http.MethodPut, app + "/follows/" + me, http.StatusBadRequest, reasonFollowSelf},
			{"bad uid", http.MethodPut, app + "/follows/a%20b", http.StatusBadRequest, ""},
			{"bad value", http.MethodGet, recs + "?filterField=author&filterOp=in&filterValue=" + alice, http.StatusBadRequest, ""},
		} {
			if r := e.do(me, c.method, c.path, nil, nil); r.code != c.code || (c.reason != "" && r.reason() != c.reason) {
				t.Errorf("%s: %d %s", c.name, r.code, r.raw)
			}
		}
	})
}

// #270: in a participants-only app, only members follow, and only members.
func TestFollowsSharedApp(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		owner, mom, stranger := "fo-owner"+sfx, "fo-mom"+sfx, "fo-stranger"+sfx
		app := "/v1/apps/FOLLOW-SH" + sfx
		e.ownApp(owner, app, []byte(strings.Replace(string(timelineDSL(t)), `"scope": "public"`, `"scope": "shared"`, 1)))
		tok := e.share(owner, app)
		if r := e.do(mom, http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil); r.code != http.StatusCreated {
			t.Fatalf("mom joins: %d %s", r.code, r.raw)
		}
		e.do(stranger, http.MethodGet, app+"/dsl", nil, nil)

		wantNotMember(t, e.do(stranger, http.MethodPut, app+"/follows/"+owner, nil, nil), "stranger follows")
		wantNotMember(t, e.do(stranger, http.MethodGet, app+"/follows", nil, nil), "stranger lists")
		if r := e.do(mom, http.MethodPut, app+"/follows/"+owner, nil, nil); r.code != http.StatusOK {
			t.Fatalf("mom follows the owner: %d %s", r.code, r.raw)
		}
		if r := e.do(mom, http.MethodPut, app+"/follows/"+stranger, nil, nil); r.code != http.StatusNotFound || r.reason() != reasonNotInApp {
			t.Fatalf("following a non-member: %d %s", r.code, r.raw)
		}
	})
}

// #270: an app without shared data has nothing to follow in.
func TestFollowsNeedSharedData(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		u := "fo-solo" + sfx
		app := "/v1/apps/FOLLOW-PRIV" + sfx
		e.ownApp(u, app, walk(t)) // private only
		if r := e.do(u, http.MethodPut, app+"/follows/x", nil, nil); r.code != http.StatusNotFound || r.reason() != reasonNoSharedData {
			t.Fatalf("private app: %d %s", r.code, r.raw)
		}
	})
}
