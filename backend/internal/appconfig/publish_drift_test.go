package appconfig

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// failPublishStore fails every commit that publishes, like a transaction
// whose apps_public write fails.
type failPublishStore struct {
	store.Store
	fail bool
}

func (s *failPublishStore) CommitVersion(ctx context.Context, uid, appID string, c store.Commit) (*store.Version, error) {
	if s.fail && c.Publish != nil {
		return nil, errors.New("injected publish failure")
	}
	return s.Store.CommitVersion(ctx, uid, appID, c)
}

// The creator's commits and the published DSL move together: a commit that
// cannot publish fails as a whole instead of leaving an older mirror behind.
func TestCommitPublishNoDrift(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	mem := store.NewMemory()
	st := &failPublishStore{Store: mem}
	e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}
	const app = "/v1/apps/DRIFT-1"
	e.ownApp("u1", app, sharedKakeibo(t))
	mirror := func() *store.PublicApp {
		t.Helper()
		p, err := mem.GetPublicApp(t.Context(), "DRIFT-1")
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	if p := mirror(); p.Version != 2 {
		t.Fatalf("published version %d, want 2", p.Version)
	}

	st.fail = true
	for _, c := range []struct {
		name, method, path string
		body               any
	}{
		{"put", http.MethodPut, app + "/dsl", putBody(kakeibo(t))},
		{"patch", http.MethodPatch, app + "/dsl", `{"patch":[{"op":"replace","path":"/app/name","value":"x"}]}`},
		{"restore", http.MethodPost, app + "/versions/1/restore", nil},
		{"rename", http.MethodPatch, app, `{"name":"新しい名前"}`},
	} {
		r := e.do("u1", c.method, c.path, c.body, map[string]string{"If-Match": `"2"`})
		if r.code/100 == 2 {
			t.Fatalf("%s: succeeded although publishing failed: %d %s", c.name, r.code, r.raw)
		}
		if got := e.do("u1", http.MethodGet, app+"/dsl", nil, nil).version(); got != 2 {
			t.Fatalf("%s: creator's version %d, want 2", c.name, got)
		}
		if p := mirror(); p.Version != 2 {
			t.Fatalf("%s: mirror version %d, want 2", c.name, p.Version)
		}
	}

	// once publishing works again the next commit brings both forward
	st.fail = false
	if r := e.do("u1", http.MethodPut, app+"/dsl", putBody(sharedKakeibo(t)), map[string]string{"If-Match": `"2"`}); r.code != http.StatusCreated {
		t.Fatalf("retry: %d %s", r.code, r.raw)
	}
	if p := mirror(); p.Version != 3 {
		t.Fatalf("mirror version %d, want 3", p.Version)
	}
}

// When the creator takes every entity off sharing, the published DSL follows
// (it no longer carries the shared rules); participants stop seeing the
// shared records, which are kept and come back when sharing returns.
func TestWithdrawSharingUpdatesMirror(t *testing.T) {
	storeEnvs(t, func(t *testing.T, e *env, st store.Store, sfx string) {
		appID := "WITHDRAW" + sfx
		app := "/v1/apps/" + appID
		recs := app + "/entities/expense/records"
		owner, member := "wd-owner"+sfx, "wd-member"+sfx
		e.ownApp(owner, app, sharedKakeibo(t))
		if r := e.do(owner, http.MethodPost, recs, `{"data":{"amount":100}}`, nil); r.code != http.StatusCreated {
			t.Fatalf("owner post: %d %s", r.code, r.raw)
		}
		e.do(member, http.MethodGet, app+"/dsl", nil, nil) // join (#93)
		list := func() int {
			t.Helper()
			r := e.do(member, http.MethodGet, recs, nil, nil)
			if r.code != http.StatusOK {
				t.Fatalf("member list: %d %s", r.code, r.raw)
			}
			items, _ := r.body["items"].([]any)
			return len(items)
		}
		if n := list(); n != 1 {
			t.Fatalf("shared: member sees %d records, want 1", n)
		}

		// withdraw: the mirror is the creator's latest, with no shared entity
		if r := e.do(owner, http.MethodPut, app+"/dsl", putBody(kakeibo(t)), map[string]string{"If-Match": `"2"`}); r.code != http.StatusCreated {
			t.Fatalf("withdraw: %d %s", r.code, r.raw)
		}
		p, err := st.GetPublicApp(t.Context(), appID)
		if err != nil {
			t.Fatal(err)
		}
		pub := &dsl.App{}
		if err := json.Unmarshal(p.DSL, pub); err != nil {
			t.Fatal(err)
		}
		if p.Version != 3 || pub.HasSharedEntity() {
			t.Fatalf("mirror after withdraw: version %d shared %v", p.Version, pub.HasSharedEntity())
		}
		if n := list(); n != 0 {
			t.Fatalf("withdrawn: member still sees %d shared records", n)
		}

		// sharing again: the kept records are back for the member
		if r := e.do(owner, http.MethodPut, app+"/dsl", putBody(sharedKakeibo(t)), map[string]string{"If-Match": `"3"`}); r.code != http.StatusCreated {
			t.Fatalf("reshare: %d %s", r.code, r.raw)
		}
		if n := list(); n != 1 {
			t.Fatalf("reshared: member sees %d records, want 1", n)
		}
	})
}
