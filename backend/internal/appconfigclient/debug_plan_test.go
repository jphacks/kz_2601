package appconfigclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/debugoverride"
)

// #2085: ai forwards the stg test plan its own middleware carried; nothing
// is sent when the override is off on ai.
func TestHTTPForwardsDebugPlan(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get(debugoverride.Header))
		_, _ = w.Write([]byte(`{"appId":"a","version":1,"dsl":{}}`))
	}))
	defer srv.Close()
	c := NewHTTP(srv.URL)

	cfg, err := debugoverride.FromEnv(func(k string) string {
		return map[string]string{"APP_ENV": "stg", debugoverride.EnabledEnv: "1", debugoverride.UIDsEnv: "u1"}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	ctxOf := func(cfg *debugoverride.Config) context.Context {
		var ctx context.Context
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(debugoverride.Header, "free")
		debugoverride.Middleware(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { ctx = r.Context() })).
			ServeHTTP(httptest.NewRecorder(), req)
		return ctx
	}
	for _, ctx := range []context.Context{ctxOf(cfg), ctxOf(nil), context.Background()} {
		if _, err := c.Get(ctx, "a"); err != nil {
			t.Fatal(err)
		}
	}
	if len(got) != 3 || got[0] != "free" || got[1] != "" || got[2] != "" {
		t.Fatalf("forwarded %q", got)
	}
}
