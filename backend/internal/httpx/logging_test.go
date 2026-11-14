package httpx

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testUID = "raw-uid-abc123"

func cloudLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(NewCloudHandler(buf, slog.LevelDebug, CloudConfig{ProjectID: "proj", Service: "ai-stg", Version: "ai-stg-00001"}))
}

func lines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("not JSON: %q", line)
		}
		out = append(out, m)
	}
	return out
}

func TestCloudHandlerSeverityAndMessage(t *testing.T) {
	tests := []struct {
		level slog.Level
		want  string
	}{
		{slog.LevelDebug, "DEBUG"},
		{slog.LevelInfo, "INFO"},
		{slog.LevelWarn, "WARNING"},
		{slog.LevelError, "ERROR"},
		{slog.LevelError + 4, "CRITICAL"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			var buf bytes.Buffer
			cloudLogger(&buf).Log(t.Context(), tt.level, "hello")
			m := lines(t, &buf)[0]
			if m["severity"] != tt.want || m["message"] != "hello" {
				t.Fatalf("entry = %v", m)
			}
			if _, ok := m["level"]; ok {
				t.Fatalf("level key left: %v", m)
			}
			_, typed := m["@type"]
			if typed != (tt.level >= slog.LevelError) {
				t.Fatalf("@type present = %v at %v: %v", typed, tt.level, m)
			}
		})
	}
}

func TestCloudHandlerErrorEvent(t *testing.T) {
	var buf bytes.Buffer
	cloudLogger(&buf).Error("boom", slog.String("@type", "custom"))
	m := lines(t, &buf)[0]
	if m["@type"] != "custom" {
		t.Fatalf("existing @type overwritten or duplicated: %s", buf.String())
	}
	if strings.Count(buf.String(), `"@type"`) != 1 {
		t.Fatalf("duplicate key: %s", buf.String())
	}
	sc, _ := m["serviceContext"].(map[string]any)
	if sc["service"] != "ai-stg" || sc["version"] != "ai-stg-00001" {
		t.Fatalf("serviceContext = %v", m["serviceContext"])
	}
}

func TestParseTrace(t *testing.T) {
	tests := []struct {
		name, traceparent, cloud string
		trace, span              string
		sampled                  bool
	}{
		{name: "traceparent", traceparent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			trace: "0af7651916cd43dd8448eb211c80319c", span: "b7ad6b7169203331", sampled: true},
		{name: "traceparent wins", traceparent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-00",
			cloud: "11111111111111111111111111111111/1;o=1",
			trace: "0af7651916cd43dd8448eb211c80319c", span: "b7ad6b7169203331"},
		{name: "cloud header", cloud: "105445AA7843BC8BF206B12000100000/1;o=1",
			trace: "105445aa7843bc8bf206b12000100000", span: "0000000000000001", sampled: true},
		{name: "cloud header no span", cloud: "105445aa7843bc8bf206b12000100000",
			trace: "105445aa7843bc8bf206b12000100000"},
		{name: "bad traceparent falls back", traceparent: "00-xyz-abc-01", cloud: "105445aa7843bc8bf206b12000100000/255;o=0",
			trace: "105445aa7843bc8bf206b12000100000", span: "00000000000000ff"},
		{name: "zero trace", cloud: "00000000000000000000000000000000/1;o=1"},
		{name: "garbage", cloud: "not-a-trace\n"},
		{name: "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trace, span, sampled := parseTrace(tt.traceparent, tt.cloud)
			if trace != tt.trace || span != tt.span || sampled != tt.sampled {
				t.Fatalf("got (%q, %q, %v), want (%q, %q, %v)", trace, span, sampled, tt.trace, tt.span, tt.sampled)
			}
		})
	}
}

// One request: trace, requestId and uid_hash reach both the handler's own
// line and the outer access log; the raw uid never does.
func TestRequestLogFields(t *testing.T) {
	var buf bytes.Buffer
	logger := cloudLogger(&buf)
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		NoteUID(r.Context(), testUID) // what auth.ContextWithUID does
		logger.InfoContext(r.Context(), "inside")
		w.WriteHeader(http.StatusNoContent)
	}), WithRequestID, Log(logger), Recover(logger))

	req := httptest.NewRequest(http.MethodGet, "/v1/apps?q=private", nil)
	req.Header.Set("X-Cloud-Trace-Context", "105445aa7843bc8bf206b12000100000/1;o=1")
	req.Header.Set(RequestIDHeader, "req-1")
	h.ServeHTTP(httptest.NewRecorder(), req)

	got := lines(t, &buf)
	if len(got) != 2 {
		t.Fatalf("want 2 lines: %s", buf.String())
	}
	for _, m := range got {
		if m["logging.googleapis.com/trace"] != "projects/proj/traces/105445aa7843bc8bf206b12000100000" ||
			m["logging.googleapis.com/spanId"] != "0000000000000001" ||
			m["logging.googleapis.com/trace_sampled"] != true ||
			m["requestId"] != "req-1" || m["uid_hash"] != UIDHash(testUID) {
			t.Fatalf("entry = %v", m)
		}
	}
	if got[1]["event"] != RequestEvent || got[1]["severity"] != "INFO" {
		t.Fatalf("access log = %v", got[1])
	}
	if strings.Contains(buf.String(), testUID) || strings.Contains(buf.String(), "private") {
		t.Fatalf("raw uid or query leaked: %s", buf.String())
	}
	if strings.Count(buf.String(), `"requestId"`) != 2 {
		t.Fatalf("requestId duplicated: %s", buf.String())
	}
}

func TestNoTraceWithoutProject(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewCloudHandler(&buf, slog.LevelInfo, CloudConfig{}))
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), WithRequestID, Log(logger))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Cloud-Trace-Context", "105445aa7843bc8bf206b12000100000/1;o=1")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if strings.Contains(buf.String(), "logging.googleapis.com/trace") {
		t.Fatalf("trace without a project id: %s", buf.String())
	}
}

func Test5xxAndPanicReachErrorReporting(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		msg     string
		stack   bool
	}{
		{name: "5xx", msg: "request", handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}},
		{name: "panic", msg: "panic recovered", stack: true, handler: func(http.ResponseWriter, *http.Request) {
			panic("boom")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := cloudLogger(&buf)
			h := Chain(tt.handler, WithRequestID, Log(logger), Recover(logger))
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/apps/x", nil))

			var entry map[string]any
			for _, m := range lines(t, &buf) {
				if m["message"] == tt.msg {
					entry = m
				}
			}
			if entry == nil {
				t.Fatalf("no %q line: %s", tt.msg, buf.String())
			}
			if entry["severity"] != "ERROR" || entry["@type"] != ReportedErrorEventType {
				t.Fatalf("entry = %v", entry)
			}
			hr, _ := entry["context"].(map[string]any)["httpRequest"].(map[string]any)
			if hr["method"] != http.MethodPost || hr["url"] != "/v1/apps/x" || hr["responseStatusCode"] == nil {
				t.Fatalf("context.httpRequest = %v", entry["context"])
			}
			st, _ := entry["stack_trace"].(string)
			if tt.stack && (!strings.HasPrefix(st, "panic: boom\n\ngoroutine ") || !strings.Contains(st, "[running]:")) {
				t.Fatalf("stack_trace = %q", st)
			}
		})
	}
}

func TestRecorderFlush(t *testing.T) {
	rec := httptest.NewRecorder()
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("wrapped writer is not a Flusher")
		}
		_, _ = w.Write([]byte("data: x\n\n"))
		f.Flush()
	}), WithRequestID, Log(discardLogger()), Recover(discardLogger()))
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if !rec.Flushed {
		t.Fatal("not flushed through the recorder")
	}
}

func TestDebugErrors(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	tests := []struct {
		name    string
		enabled bool
		target  string
		want    int
	}{
		{name: "disabled passes through", target: DebugErrorPath, want: http.StatusOK},
		{name: "5xx", enabled: true, target: DebugErrorPath, want: http.StatusInternalServerError},
		{name: "panic is recovered as 500", enabled: true, target: DebugErrorPath + "?kind=panic", want: http.StatusInternalServerError},
		{name: "other paths untouched", enabled: true, target: "/health", want: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := Chain(ok, WithRequestID, Recover(discardLogger()), DebugErrors(tt.enabled))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.target, nil))
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestCloudConfigFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want CloudConfig
	}{
		{"cloud run", map[string]string{"GOOGLE_CLOUD_PROJECT": "proj", "K_SERVICE": "ai-stg", "K_REVISION": "ai-stg-00002"},
			CloudConfig{ProjectID: "proj", Service: "ai-stg", Version: "ai-stg-00002"}},
		{"project only", map[string]string{"GOOGLE_CLOUD_PROJECT": "proj"}, CloudConfig{ProjectID: "proj"}},
		{"local", nil, CloudConfig{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CloudConfigFromEnv(envMap(tt.env)); got != tt.want {
				t.Fatalf("CloudConfigFromEnv = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// NewLogger writes Cloud Logging JSON both on Cloud Run and locally; only the
// serviceContext of ERROR lines depends on K_SERVICE.
func TestNewLogger(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		wantService map[string]any
	}{
		{"cloud run", map[string]string{"K_SERVICE": "ai-stg", "K_REVISION": "ai-stg-00002"},
			map[string]any{"service": "ai-stg", "version": "ai-stg-00002"}},
		{"cloud run without revision", map[string]string{"K_SERVICE": "ai-stg"}, map[string]any{"service": "ai-stg"}},
		{"local", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := newLogger(&buf, envMap(tt.env))
			log.Debug("hidden at the default level")
			log.Error("boom")
			ls := lines(t, &buf)
			if len(ls) != 1 {
				t.Fatalf("want only the ERROR line with LOG_LEVEL unset: %s", buf.String())
			}
			m := ls[0]
			if m["severity"] != "ERROR" || m["message"] != "boom" || m["@type"] != ReportedErrorEventType {
				t.Fatalf("entry = %v", m)
			}
			sc, has := m["serviceContext"].(map[string]any)
			if has != (tt.wantService != nil) {
				t.Fatalf("serviceContext = %v, want %v", m["serviceContext"], tt.wantService)
			}
			if has && (len(sc) != len(tt.wantService) || sc["service"] != tt.wantService["service"] || sc["version"] != tt.wantService["version"]) {
				t.Fatalf("serviceContext = %v, want %v", sc, tt.wantService)
			}
		})
	}
}

func TestNewLoggerLevel(t *testing.T) {
	var buf bytes.Buffer
	newLogger(&buf, envMap(map[string]string{"LOG_LEVEL": "debug"})).Debug("shown")
	if m := lines(t, &buf); len(m) != 1 || m[0]["severity"] != "DEBUG" {
		t.Fatalf("LOG_LEVEL=debug: %s", buf.String())
	}
}

func TestCloudHandlerWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	cloudLogger(&buf).With("component", "ai").With("region", "asia").Error("boom")
	m := lines(t, &buf)[0]
	if m["component"] != "ai" || m["region"] != "asia" {
		t.Fatalf("WithAttrs lost: %v", m)
	}
	if m["severity"] != "ERROR" || m["message"] != "boom" || m["@type"] != ReportedErrorEventType {
		t.Fatalf("cloud keys lost after With: %v", m)
	}
	sc, _ := m["serviceContext"].(map[string]any)
	if sc["service"] != "ai-stg" || sc["version"] != "ai-stg-00001" {
		t.Fatalf("cfg lost after With: %v", m)
	}
}

// Only the group and the top-level severity / message are checked: keys the
// handler adds per record (@type, serviceContext, trace) currently land inside
// the open group, which no caller relies on.
func TestCloudHandlerWithGroup(t *testing.T) {
	var buf bytes.Buffer
	cloudLogger(&buf).With("component", "ai").WithGroup("req").With("path", "/v1").Warn("slow", "ms", 900)
	m := lines(t, &buf)[0]
	if m["component"] != "ai" {
		t.Fatalf("attrs before the group lost: %v", m)
	}
	req, _ := m["req"].(map[string]any)
	if req["path"] != "/v1" || req["ms"] != float64(900) {
		t.Fatalf("WithGroup lost: %v", m)
	}
	if m["severity"] != "WARNING" || m["message"] != "slow" {
		t.Fatalf("cloud keys lost after WithGroup: %v", m)
	}
}
