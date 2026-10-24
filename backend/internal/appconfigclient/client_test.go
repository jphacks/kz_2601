package appconfigclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestMemoryVersioningAndConflict(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	env, err := m.Get(ctx, "a")
	if err != nil || env.Version != 1 {
		t.Fatalf("seed: %v %+v", err, env)
	}
	app := dsl.Initial()
	app.App.Name = "x"
	raw, _ := json.Marshal(app)
	if _, err := m.Put(ctx, "a", 2, raw, Source{}); err == nil {
		t.Fatal("stale base must conflict")
	}
	next, err := m.Put(ctx, "a", 1, raw, Source{})
	if err != nil || next.Version != 2 {
		t.Fatalf("put: %v", err)
	}
	ops := []dsl.PatchOp{{Op: "replace", Path: "/app/name", Value: json.RawMessage(`"y"`)}}
	p, err := m.Patch(ctx, "a", 2, ops, Source{})
	if err != nil || p.Version != 3 {
		t.Fatalf("patch: %v", err)
	}
	var ae *Error
	if _, err := m.Patch(ctx, "a", 3, []dsl.PatchOp{{Op: "remove", Path: "/screens/0"}}, Source{}); !errors.As(err, &ae) || ae.Status != 422 {
		t.Fatalf("invalid patch must be 422: %v", err)
	}
}

func TestHTTPForwardsBearerAndIfMatch(t *testing.T) {
	var gotAuth, gotIfMatch, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotIfMatch, gotPath = r.Header.Get("Authorization"), r.Header.Get("If-Match"), r.Method+" "+r.URL.Path
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"appId":"a","version":8,"dsl":{}}`))
	}))
	defer srv.Close()
	c := NewHTTP(srv.URL)
	ctx := WithBearer(context.Background(), "Bearer tok")
	env, err := c.Patch(ctx, "a", 7, []dsl.PatchOp{{Op: "remove", Path: "/x"}}, Source{Kind: "ai"})
	if err != nil || env.Version != 8 {
		t.Fatalf("patch: %v %+v", err, env)
	}
	if gotAuth != "Bearer tok" || gotIfMatch != `"7"` || gotPath != "PATCH /v1/apps/a/dsl" {
		t.Fatalf("headers: %q %q %q", gotAuth, gotIfMatch, gotPath)
	}
}

func TestHTTPErrorMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(409)
		_, _ = w.Write([]byte(`{"error":{"code":"version_conflict","message":"stale","details":{"currentVersion":9}}}`))
	}))
	defer srv.Close()
	_, err := NewHTTP(srv.URL).Get(context.Background(), "a")
	var ae *Error
	if !errors.As(err, &ae) || ae.Status != 409 || ae.Code != "version_conflict" || ae.CurrentVersion != 9 {
		t.Fatalf("got %v", err)
	}
}
