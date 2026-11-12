package appconfig

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// faultStore2577 fails the block reads / writes on demand.
type faultStore2577 struct {
	store.Store
	listBlocks, putBlock, blockedEither error
}

func (s *faultStore2577) ListBlocks(ctx context.Context, uid string) ([]store.Block, error) {
	if s.listBlocks != nil {
		return nil, s.listBlocks
	}
	return s.Store.ListBlocks(ctx, uid)
}

func (s *faultStore2577) PutBlock(ctx context.Context, uid string, b store.Block) error {
	if s.putBlock != nil {
		return s.putBlock
	}
	return s.Store.PutBlock(ctx, uid, b)
}

func (s *faultStore2577) BlockedEither(ctx context.Context, uid string, others []string) (map[string]bool, error) {
	if s.blockedEither != nil {
		return nil, s.blockedEither
	}
	return s.Store.BlockedEither(ctx, uid, others)
}

func newFaultEnv2577(t *testing.T) (*env, *faultStore2577) {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	st := &faultStore2577{Store: store.NewMemory()}
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}, st
}

func TestCreateBlockFailures2577(t *testing.T) {
	e, st := newFaultEnv2577(t)
	block := func(uid, body string) resp {
		t.Helper()
		return e.do(uid, http.MethodPost, "/v1/me/blocks", body, nil)
	}
	const pub = "PUB-2577"
	e.ownApp("pub2577", "/v1/apps/"+pub, kakeibo(t))
	if r := e.do("pub2577", http.MethodPost, "/v1/apps/"+pub+"/publish", `{"authorName":"こうかい"}`, nil); r.code != http.StatusOK {
		t.Fatalf("publish: %d %s", r.code, r.raw)
	}

	for name, c := range map[string]struct {
		uid, body string
		code      int
	}{
		"not json":           {"me2577", `{`, http.StatusBadRequest},
		"no fields":          {"me2577", `{}`, http.StatusBadRequest},
		"invalid uid":        {"me2577", `{"uid":"a b"}`, http.StatusBadRequest},
		"uid and listing":    {"me2577", `{"uid":"x","listingId":"` + pub + `"}`, http.StatusBadRequest},
		"invalid listingId":  {"me2577", `{"listingId":"../x"}`, http.StatusBadRequest},
		"unknown listing":    {"me2577", `{"listingId":"MISSING-2577"}`, http.StatusNotFound},
		"block yourself":     {"me2577", `{"uid":"me2577"}`, http.StatusBadRequest},
		"own listing":        {"pub2577", `{"listingId":"` + pub + `"}`, http.StatusBadRequest},
		"name only, no uid":  {"me2577", `{"name":"だれか"}`, http.StatusBadRequest},
		"empty uid and name": {"me2577", `{"uid":"","name":""}`, http.StatusBadRequest},
	} {
		if r := block(c.uid, c.body); r.code != c.code {
			t.Fatalf("%s: want %d, got %d %s", name, c.code, r.code, r.raw)
		}
	}
	if r := e.do("me2577", http.MethodGet, "/v1/me/blocks", nil, nil); len(r.body["items"].([]any)) != 0 {
		t.Fatalf("rejected blocks were stored: %s", r.raw)
	}

	// The same person twice: both 201, one block, the later name wins; a
	// blank name falls back to the default and a long one is cut.
	long := strings.Repeat("な", maxAuthorNameLen+10)
	for _, body := range []string{`{"uid":"x2577","name":"  "}`, `{"uid":"x2577","name":"` + long + `"}`} {
		if r := block("me2577", body); r.code != http.StatusCreated || r.body["uid"] != "x2577" {
			t.Fatalf("block %s: %d %s", body, r.code, r.raw)
		}
	}
	r := e.do("me2577", http.MethodGet, "/v1/me/blocks", nil, nil)
	items := r.body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["name"] != strings.Repeat("な", maxAuthorNameLen) {
		t.Fatalf("after blocking twice: %s", r.raw)
	}
	if r := block("me2577", `{"uid":"y2577"}`); r.code != http.StatusCreated || r.body["name"] != defaultAuthorName {
		t.Fatalf("no name: %d %s", r.code, r.raw)
	}

	// At the cap: a new person is refused, an already blocked one is still accepted.
	for i := range store.MaxBlocks - 2 {
		if err := st.Store.PutBlock(context.Background(), "me2577", store.Block{UID: fmt.Sprintf("fill%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	if r := block("me2577", `{"uid":"z2577"}`); r.code != http.StatusBadRequest {
		t.Fatalf("over the cap: %d %s", r.code, r.raw)
	}
	if r := block("me2577", `{"uid":"x2577","name":"rename"}`); r.code != http.StatusCreated {
		t.Fatalf("re-block at the cap: %d %s", r.code, r.raw)
	}

	st.listBlocks = errors.New("injected")
	if r := block("other2577", `{"uid":"x2577"}`); r.code != http.StatusInternalServerError || r.errCode() != "internal" {
		t.Fatalf("ListBlocks fails: %d %s", r.code, r.raw)
	}
	if r := e.do("other2577", http.MethodGet, "/v1/me/blocks", nil, nil); r.code != http.StatusInternalServerError {
		t.Fatalf("list when ListBlocks fails: %d %s", r.code, r.raw)
	}
	st.listBlocks, st.putBlock = nil, errors.New("injected")
	if r := block("other2577", `{"uid":"x2577"}`); r.code != http.StatusInternalServerError || r.errCode() != "internal" {
		t.Fatalf("PutBlock fails: %d %s", r.code, r.raw)
	}
}

func TestReportTargetFailures2577(t *testing.T) {
	e, st := newFaultEnv2577(t)
	report := func(uid, body string) resp {
		t.Helper()
		return e.do(uid, http.MethodPost, "/v1/reports", body, nil)
	}
	const app = "SHARED-2577"
	e.ownApp("u1", "/v1/apps/"+app, sharedKakeibo(t))
	r := e.do("u1", http.MethodPost, "/v1/apps/"+app+"/entities/expense/records", `{"data":{"amount":100}}`, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("post: %d %s", r.code, r.raw)
	}
	recID := r.body["id"].(string)
	tok, _ := e.do("u1", http.MethodPost, "/v1/apps/"+app+"/share", nil, nil).body["token"].(string)
	if tok == "" {
		t.Fatal("no share token")
	}
	const private = "PRIVATE-2577"
	e.ownApp("u1", "/v1/apps/"+private, kakeibo(t))

	rec := func(appID, eid, id string) string {
		return `{"kind":"record","appId":"` + appID + `","entityId":"` + eid + `","recordId":"` + id + `","reason":"spam"}`
	}
	for name, c := range map[string]struct {
		body    string
		code    int
		errCode string
	}{
		"no kind":              {`{"id":"x","reason":"spam"}`, http.StatusBadRequest, "bad_request"},
		"unknown kind":         {`{"kind":"user","id":"x","reason":"spam"}`, http.StatusBadRequest, "bad_request"},
		"no reason":            {`{"kind":"share","id":"` + tok + `"}`, http.StatusBadRequest, "bad_request"},
		"listing without id":   {`{"kind":"listing","reason":"spam"}`, http.StatusBadRequest, "bad_request"},
		"share without id":     {`{"kind":"share","reason":"spam"}`, http.StatusBadRequest, "bad_request"},
		"share invalid id":     {`{"kind":"share","id":"../x","reason":"spam"}`, http.StatusBadRequest, "bad_request"},
		"unknown share":        {`{"kind":"share","id":"MISSING","reason":"spam"}`, http.StatusNotFound, "not_found"},
		"app without appId":    {`{"kind":"app","reason":"spam"}`, http.StatusBadRequest, "bad_request"},
		"app reserved appId":   {`{"kind":"app","appId":"null","reason":"spam"}`, http.StatusBadRequest, "bad_request"},
		"app not shared":       {`{"kind":"app","appId":"` + private + `","reason":"spam"}`, http.StatusNotFound, "not_found"},
		"unknown app":          {`{"kind":"app","appId":"MISSING-2577","reason":"spam"}`, http.StatusNotFound, "not_found"},
		"record without ids":   {`{"kind":"record","reason":"spam"}`, http.StatusBadRequest, "bad_request"},
		"record no entityId":   {rec(app, "", recID), http.StatusBadRequest, "bad_request"},
		"record no recordId":   {rec(app, "expense", ""), http.StatusBadRequest, "bad_request"},
		"record bad entityId":  {rec(app, "a b", recID), http.StatusBadRequest, "bad_request"},
		"unknown record":       {rec(app, "expense", "MISSING"), http.StatusNotFound, "not_found"},
		"record in other ent.": {rec(app, "nope", recID), http.StatusNotFound, "not_found"},
	} {
		if r := report("u2", c.body); r.code != c.code || r.errCode() != c.errCode {
			t.Fatalf("%s: want %d %s, got %d %s", name, c.code, c.errCode, r.code, r.raw)
		}
	}

	// The owner cannot report their own share; someone else can.
	if r := report("u1", `{"kind":"share","id":"`+tok+`","reason":"spam"}`); r.code != http.StatusBadRequest {
		t.Fatalf("own share: %d %s", r.code, r.raw)
	}
	if r := report("u2", `{"kind":"share","id":"`+tok+`","reason":"spam"}`); r.code != http.StatusCreated || r.body["created"] != true {
		t.Fatalf("share report: %d %s", r.code, r.raw)
	}

	st.blockedEither = errors.New("injected")
	if r := report("u2", rec(app, "expense", recID)); r.code != http.StatusInternalServerError || r.errCode() != "internal" {
		t.Fatalf("block lookup fails: %d %s", r.code, r.raw)
	}
}
