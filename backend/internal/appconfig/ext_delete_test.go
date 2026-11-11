package appconfig

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/extapi"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// DELETE /v1/me revokes the leaver's connections through the runner (#1716).
func TestDeleteMeRevokesExt(t *testing.T) {
	e := newExtEnv(t, 100)
	r := e.do("solo", http.MethodPost, "/v1/me/connections/fakeOauth/start", nil, nil)
	u, err := url.Parse(r.body["authUrl"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if back := e.svc.Callback(t.Context(), extapi.FakeOAuth, u.Query().Get("state"), u.Query().Get("code")); !strings.HasSuffix(back, "status=ok") {
		t.Fatalf("callback: %s", back)
	}
	if l, _ := e.svc.Conns.List(t.Context(), "solo"); len(l) != 1 {
		t.Fatalf("connected: %+v", l)
	}

	if r := e.do("solo", http.MethodDelete, "/v1/me", nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", r.code, r.raw)
	}
	if l, _ := e.svc.Conns.List(t.Context(), "solo"); len(l) != 0 {
		t.Fatalf("connections left after delete: %+v", l)
	}
}

// A runner that fails (or is gone) never fails the deletion.
func TestDeleteMeRunnerFailure(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	var (
		mu   sync.Mutex
		hits []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b extapi.ConnBody
		_ = json.NewDecoder(r.Body).Decode(&b)
		mu.Lock()
		hits = append(hits, r.URL.Path+" "+b.UID)
		mu.Unlock()
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"reason": extapi.ReasonUnavailable})
	}))
	t.Cleanup(srv.Close)

	st := store.NewMemory()
	e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(RejectDeleted(st, New(st, WithExt(&extapi.Client{BaseURL: srv.URL, HTTP: srv.Client()}))))}
	if r := e.do("leaver", http.MethodDelete, "/v1/me", nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("delete with a failing runner: %d %s", r.code, r.raw)
	}
	mu.Lock()
	got := append([]string(nil), hits...)
	mu.Unlock()
	if len(got) != 1 || got[0] != "/internal/ext/connections/revokeAll leaver" {
		t.Fatalf("runner calls: %v", got)
	}
	if r := e.do("leaver", http.MethodGet, "/v1/apps", nil, nil); r.code != http.StatusGone {
		t.Fatalf("deleted anyway: %d %s", r.code, r.raw)
	}

	// an unreachable runner: still deleted
	srv.Close()
	if r := e.do("other", http.MethodDelete, "/v1/me", nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("delete with no runner: %d %s", r.code, r.raw)
	}
}
