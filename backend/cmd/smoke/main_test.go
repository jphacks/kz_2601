package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
)

// -cleanup must DELETE /v1/apps/{id} (with the smoke bearer) for every app the
// run tracked, including the auth-probe app that GET /dsl auto-created.
func TestCleanupArchivesTrackedApps(t *testing.T) {
	var (
		mu   sync.Mutex
		dels []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/dsl"):
			_, _ = w.Write([]byte(`{"appId":"x","version":1,"dsl":{}}`))
		case req.Method == http.MethodDelete && strings.HasPrefix(req.URL.Path, "/v1/apps/"):
			mu.Lock()
			dels = append(dels, strings.TrimPrefix(req.URL.Path, "/v1/apps/"))
			mu.Unlock()
			if strings.HasSuffix(req.URL.Path, "-gone") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	r := &runner{appconfig: srv.URL, token: "Bearer tok", tokenKind: "flag", http: srv.Client(), store: appconfigclient.NewHTTP(srv.URL)}
	ctx := context.Background()
	if fallback := r.authProbe(ctx, "smoke-1"); fallback {
		t.Fatalf("probe must pass: %+v", r.steps)
	}
	r.track("smoke-1")
	r.track("smoke-1-gone")
	r.cleanup(ctx)

	want := []string{"smoke-1-probe", "smoke-1", "smoke-1-gone"}
	if strings.Join(dels, ",") != strings.Join(want, ",") {
		t.Fatalf("DELETE calls = %v, want %v", dels, want)
	}
	var pass, warn, fail int
	for _, s := range r.steps {
		switch {
		case !s.OK:
			fail++
		case s.Warn:
			warn++
		default:
			pass++
		}
	}
	// probe PASS + 2 archived PASS + 1 WARN (404)
	if pass != 3 || warn != 1 || fail != 0 {
		t.Fatalf("steps pass/warn/fail = %d/%d/%d, want 3/1/0: %+v", pass, warn, fail, r.steps)
	}
}

func TestCleanupNoAppconfigIsNoop(t *testing.T) {
	r := &runner{appconfig: "", http: http.DefaultClient}
	r.track("smoke-1")
	r.cleanup(context.Background())
	if len(r.steps) != 0 {
		t.Fatalf("expected no steps without -appconfig, got %+v", r.steps)
	}
}

func TestCleanupFailureIsReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"internal"}}`))
	}))
	defer srv.Close()
	r := &runner{appconfig: srv.URL, token: "Bearer tok", http: srv.Client()}
	r.track("smoke-1")
	r.cleanup(context.Background())
	if len(r.steps) != 1 || r.steps[0].OK {
		t.Fatalf("500 must be a FAIL step: %+v", r.steps)
	}
	if r.print(false) != 1 {
		t.Fatal("print must count the cleanup FAIL")
	}
}
