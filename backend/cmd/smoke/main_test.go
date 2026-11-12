package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
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

func comp(t dsl.ComponentType, area string, kids ...dsl.Component) dsl.Component {
	return dsl.Component{Type: t, Area: area, Children: kids}
}

func appOf(screens [][]dsl.Component) *dsl.App {
	app := &dsl.App{}
	for _, cs := range screens {
		app.Screens = append(app.Screens, dsl.Screen{Components: cs})
	}
	return app
}

func TestOverlaps(t *testing.T) {
	text := func(area string) dsl.Component { return comp(dsl.CompText, area) }
	cases := map[string]struct {
		screens [][]dsl.Component
		want    int
	}{
		"empty":            {nil, 0},
		"no area":          {[][]dsl.Component{{text(""), text("")}}, 0},
		"side by side":     {[][]dsl.Component{{text("A1:C2"), text("D1:F2")}}, 0},
		"touching rows":    {[][]dsl.Component{{text("A1:F1"), text("A2:F2")}}, 0},
		"one overlap":      {[][]dsl.Component{{text("A1:C2"), text("C2:D3")}}, 1},
		"three on a cell":  {[][]dsl.Component{{text("A1:A1"), text("A1:A1"), text("A1:B1")}}, 3},
		"malformed counts": {[][]dsl.Component{{text("Z9:Z1"), text("A1:B2")}}, 1},
		"per screen":       {[][]dsl.Component{{text("A1:B2")}, {text("A1:B2")}}, 0},
		"children ignored": {[][]dsl.Component{{comp(dsl.CompCard, "A1:F3", text("A1:F3"))}}, 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := overlaps(appOf(tc.screens)); got != tc.want {
				t.Fatalf("overlaps = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestPhotoSource(t *testing.T) {
	photo := func(src string) dsl.Component { return dsl.Component{Type: dsl.CompPhotoField, Source: src} }
	text := comp(dsl.CompText, "")
	cases := map[string]struct {
		screens [][]dsl.Component
		want    string
	}{
		"no screens":     {nil, ""},
		"none":           {[][]dsl.Component{{text}}, ""},
		"default source": {[][]dsl.Component{{photo("")}}, "default"},
		"explicit":       {[][]dsl.Component{{photo("camera")}}, "camera"},
		"nested":         {[][]dsl.Component{{comp(dsl.CompCard, "", comp(dsl.CompVStack, "", photo("library")))}}, "library"},
		"first wins":     {[][]dsl.Component{{photo("camera"), photo("library")}}, "camera"},
		"later screen":   {[][]dsl.Component{{text}, {photo("library")}}, "library"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := photoSource(appOf(tc.screens)); got != tc.want {
				t.Fatalf("photoSource = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCanon(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"key order":   {`{"b":1, "a":[1, 2]}`, `{"a":[1,2],"b":1}`},
		"whitespace":  {" [ 1 ,\n2 ] ", `[1,2]`},
		"nested keys": {`{"a":{"c":1,"b":2}}`, `{"a":{"b":2,"c":1}}`},
		"invalid raw": {`{not json`, `{not json`},
		"scalar":      {`"x"`, `"x"`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := string(canon(json.RawMessage(tc.in))); got != tc.want {
				t.Fatalf("canon(%s) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}
