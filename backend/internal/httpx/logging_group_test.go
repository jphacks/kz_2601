package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// requestCtx is a context with the request meta WithRequestID sets.
func requestCtx(t *testing.T) context.Context {
	t.Helper()
	var ctx context.Context
	h := WithRequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { ctx = r.Context() }))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Cloud-Trace-Context", "105445aa7843bc8bf206b12000100000/1;o=1")
	req.Header.Set(RequestIDHeader, "req-g")
	h.ServeHTTP(httptest.NewRecorder(), req)
	NoteUID(ctx, testUID)
	return ctx
}

func TestCloudHandlerWithGroupKeepsReportingKeysTopLevel(t *testing.T) {
	var buf bytes.Buffer
	ctx := requestCtx(t)
	cloudLogger(&buf).With("svc", "ai").WithGroup("req").With("path", "/x").WithGroup("inner").
		ErrorContext(ctx, "boom", "code", 503)

	m := lines(t, &buf)[0]
	for k, want := range map[string]any{
		"severity":                             "ERROR",
		"message":                              "boom",
		"svc":                                  "ai",
		"@type":                                ReportedErrorEventType,
		"logging.googleapis.com/trace":         "projects/proj/traces/105445aa7843bc8bf206b12000100000",
		"logging.googleapis.com/spanId":        "0000000000000001",
		"logging.googleapis.com/trace_sampled": true,
		"requestId":                            "req-g",
		"uid_hash":                             UIDHash(testUID),
	} {
		if m[k] != want {
			t.Errorf("top-level %s = %v, want %v\n%s", k, m[k], want, buf.String())
		}
	}
	if sc, _ := m["serviceContext"].(map[string]any); sc["service"] != "ai-stg" || sc["version"] != "ai-stg-00001" {
		t.Errorf("top-level serviceContext = %v", m["serviceContext"])
	}
	want := map[string]any{"path": "/x", "inner": map[string]any{"code": float64(503)}}
	if !reflect.DeepEqual(m["req"], want) {
		t.Errorf("req = %v, want %v (the caller's attrs stay in the group)", m["req"], want)
	}
	for _, k := range []string{"@type", "serviceContext", "requestId", "uid_hash", "logging.googleapis.com/trace"} {
		if strings.Count(buf.String(), `"`+k+`"`) != 1 {
			t.Errorf("%s appears %d times: %s", k, strings.Count(buf.String(), `"`+k+`"`), buf.String())
		}
	}
}

// The caller's attrs come out as slog.JSONHandler writes them, groups and all.
func TestCloudHandlerGroupsMatchJSONHandler(t *testing.T) {
	cases := map[string]func(*slog.Logger) *slog.Logger{
		"no group":           func(l *slog.Logger) *slog.Logger { return l.With("a", 1) },
		"group":              func(l *slog.Logger) *slog.Logger { return l.WithGroup("g") },
		"attrs then group":   func(l *slog.Logger) *slog.Logger { return l.With("a", 1).WithGroup("g").With("b", 2) },
		"nested":             func(l *slog.Logger) *slog.Logger { return l.WithGroup("g").With("b", 2).WithGroup("h").With("c", 3) },
		"empty group name":   func(l *slog.Logger) *slog.Logger { return l.WithGroup("").With("a", 1) },
		"empty inner groups": func(l *slog.Logger) *slog.Logger { return l.WithGroup("g").WithGroup("h") },
	}
	for name, build := range cases {
		for _, withAttrs := range []bool{false, true} {
			var cloud, plain bytes.Buffer
			cl := build(slog.New(NewCloudHandler(&cloud, slog.LevelDebug, CloudConfig{})))
			pl := build(slog.New(slog.NewJSONHandler(&plain, nil)))
			var args []any
			if withAttrs {
				args = []any{"x", "y", slog.Group("grp", "z", true)}
			}
			cl.Info("hi", args...)
			pl.Info("hi", args...)

			got := lines(t, &cloud)[0]
			delete(got, "severity")
			delete(got, "message")
			delete(got, "time")
			var wantM map[string]any
			if err := json.Unmarshal(plain.Bytes(), &wantM); err != nil {
				t.Fatal(err)
			}
			delete(wantM, "level")
			delete(wantM, "msg")
			delete(wantM, "time")
			if !reflect.DeepEqual(got, wantM) {
				t.Errorf("%s (attrs %v): got %v, want %v", name, withAttrs, got, wantM)
			}
		}
	}
}
