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

func TestMemoryVersionsAndRestore(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	app := dsl.Initial()
	app.App.Name = "x"
	raw, _ := json.Marshal(app)
	if _, err := m.Put(ctx, "a", 1, raw, Source{Kind: "ai", Mode: "full"}); err != nil {
		t.Fatal(err)
	}
	ops := []dsl.PatchOp{{Op: "replace", Path: "/app/name", Value: json.RawMessage(`"y"`)}}
	if _, err := m.Patch(ctx, "a", 2, ops, Source{Kind: "ai", Mode: "patch"}); err != nil {
		t.Fatal(err)
	}
	vs, err := m.Versions(ctx, "a")
	if err != nil || len(vs) != 3 || vs[0].Version != 3 || vs[2].Version != 1 || vs[0].Source.Mode != "patch" {
		t.Fatalf("versions newest first: %v %+v", err, vs)
	}
	env, err := m.Restore(ctx, "a", 2)
	if err != nil || env.Version != 4 {
		t.Fatalf("restore: %v %+v", err, env)
	}
	got, _ := dsl.Parse(env.DSL)
	if got.App.Name != "x" {
		t.Fatalf("restored DSL must be version 2's: %q", got.App.Name)
	}
	vs, _ = m.Versions(ctx, "a")
	if vs[0].Source.Kind != "restore" || vs[0].Source.RestoredFrom != 2 || vs[0].ParentVersion != 3 {
		t.Fatalf("restore source: %+v", vs[0].Source)
	}
	var ae *Error
	if _, err := m.Restore(ctx, "a", 99); !errors.As(err, &ae) || ae.Status != 404 {
		t.Fatalf("unknown version must be 404: %v", err)
	}
}

func TestHTTPRestoreUsesResponseDSL(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/apps/a/versions":
			_, _ = w.Write([]byte(`{"items":[{"appId":"a","version":3,"parentVersion":2,"source":{"kind":"restore","restoredFrom":1}},{"appId":"a","version":2}]}`))
		case r.Method == http.MethodPost:
			w.Header().Set("ETag", `"4"`)
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"appId":"a","version":4,"parentVersion":3,"source":{"kind":"restore","mode":"full","restoredFrom":2},"dsl":{"k":1}}`))
		default:
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	c := NewHTTP(srv.URL)
	ctx := WithBearer(context.Background(), "Bearer tok")
	vs, err := c.Versions(ctx, "a")
	if err != nil || len(vs) != 2 || vs[0].Source.RestoredFrom != 1 {
		t.Fatalf("versions: %v %+v", err, vs)
	}
	env, err := c.Restore(ctx, "a", 2)
	if err != nil || env.Version != 4 || string(env.DSL) != `{"k":1}` {
		t.Fatalf("restore: %v %+v", err, env)
	}
	want := []string{"GET /v1/apps/a/versions?pageSize=50", "POST /v1/apps/a/versions/2/restore"}
	if len(calls) != 2 || calls[0] != want[0] || calls[1] != want[1] {
		t.Fatalf("calls: %v", calls)
	}
}

// Older appconfig (bare VersionSummary, no dsl): fall back to GET /dsl.
func TestHTTPRestoreFallsBackToGetDSL(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		if r.Method == http.MethodPost {
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"appId":"a","version":4,"source":{"kind":"restore","mode":"full","restoredFrom":2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"appId":"a","version":4,"dsl":{"k":1}}`))
	}))
	defer srv.Close()
	c := NewHTTP(srv.URL)
	env, err := c.Restore(WithBearer(context.Background(), "Bearer tok"), "a", 2)
	if err != nil || env.Version != 4 || string(env.DSL) != `{"k":1}` {
		t.Fatalf("restore: %v %+v", err, env)
	}
	if len(calls) != 2 || calls[1] != "GET /v1/apps/a/dsl" {
		t.Fatalf("calls: %v", calls)
	}
}
