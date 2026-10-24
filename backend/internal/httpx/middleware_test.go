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

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
}

func TestWithRequestID(t *testing.T) {
	tests := []struct {
		name      string
		incoming  string
		wantEcho  bool
		wantEqual string
	}{
		{name: "generated when absent", wantEcho: false},
		{name: "client value adopted", incoming: "abc-123", wantEcho: true, wantEqual: "abc-123"},
		{name: "illegal characters rejected", incoming: "bad value\nx", wantEcho: false},
		{name: "overlong rejected", incoming: strings.Repeat("a", 65), wantEcho: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen string
			handler := WithRequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				seen = RequestID(r.Context())
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.incoming != "" {
				req.Header.Set(RequestIDHeader, tt.incoming)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			header := rec.Header().Get(RequestIDHeader)
			if header == "" || header != seen {
				t.Fatalf("header = %q, context = %q", header, seen)
			}
			if tt.wantEcho && header != tt.wantEqual {
				t.Fatalf("header = %q, want %q", header, tt.wantEqual)
			}
			if !tt.wantEcho && tt.incoming != "" && header == tt.incoming {
				t.Fatalf("header = %q, want a generated id", header)
			}
		})
	}
}

func TestRecover(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantStatus int
		wantCode   string
	}{
		{
			name:       "panic becomes 500",
			handler:    func(http.ResponseWriter, *http.Request) { panic("secret token leaked in panic") },
			wantStatus: http.StatusInternalServerError,
			wantCode:   CodeInternal,
		},
		{
			name: "no panic passes through",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				WriteJSON(w, http.StatusOK, map[string]string{"ok": "yes"})
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			Recover(discardLogger())(tt.handler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantCode == "" {
				return
			}
			var got ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.Error.Code != tt.wantCode {
				t.Fatalf("code = %q, want %q", got.Error.Code, tt.wantCode)
			}
			if strings.Contains(rec.Body.String(), "secret token") {
				t.Fatalf("panic value leaked into response: %s", rec.Body.String())
			}
		})
	}
}

func TestLog(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	handler := Chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}), WithRequestID, Log(logger))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/apps", nil))

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal %q: %v", buf.String(), err)
	}
	if entry["method"] != http.MethodGet || entry["path"] != "/v1/apps" || entry["status"] != float64(http.StatusTeapot) {
		t.Fatalf("entry = %v", entry)
	}
	if entry["requestId"] == "" || entry["duration"] == nil {
		t.Fatalf("entry = %v", entry)
	}
}

func TestLogLevel(t *testing.T) {
	tests := []struct {
		raw  string
		want slog.Level
	}{
		{raw: "", want: slog.LevelInfo},
		{raw: "debug", want: slog.LevelDebug},
		{raw: " WARN ", want: slog.LevelWarn},
		{raw: "error", want: slog.LevelError},
		{raw: "nonsense", want: slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := LogLevel(tt.raw); got != tt.want {
				t.Fatalf("LogLevel(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestLogRecordsPanickingRequests(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantStatus float64
	}{
		{
			name:       "panic before writing is logged as 500",
			handler:    func(http.ResponseWriter, *http.Request) { panic("boom") },
			wantStatus: float64(http.StatusInternalServerError),
		},
		{
			name: "panic after writing keeps the written status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusAccepted)
				panic("boom")
			},
			wantStatus: float64(http.StatusAccepted),
		},
		{
			name: "normal request is logged",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			},
			wantStatus: float64(http.StatusTeapot),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := bytes.NewBuffer(nil)
			logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
			handler := Chain(tt.handler, WithRequestID, Log(logger), Recover(logger))

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/apps", nil))

			entry, ok := findLogEntry(t, buf.String(), "request")
			if !ok {
				t.Fatalf("no access log line for a panicking request: %s", buf.String())
			}
			if entry["status"] != tt.wantStatus {
				t.Fatalf("logged status = %v, want %v", entry["status"], tt.wantStatus)
			}
			if entry["method"] != http.MethodGet || entry["path"] != "/v1/apps" {
				t.Fatalf("entry = %v", entry)
			}
			if entry["requestId"] == "" || entry["duration"] == nil {
				t.Fatalf("entry = %v", entry)
			}
			if rec.Code != int(tt.wantStatus) {
				t.Fatalf("response status = %d, want %v", rec.Code, tt.wantStatus)
			}
		})
	}
}

func findLogEntry(t *testing.T, output, msg string) (map[string]any, bool) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("unmarshal %q: %v", line, err)
		}
		if entry["msg"] == msg {
			return entry, true
		}
	}
	return nil, false
}

func TestErrorEnvelope(t *testing.T) {
	tests := []struct {
		name            string
		handler         http.HandlerFunc
		wantStatus      int
		wantCode        string
		wantBodyPreload string
	}{
		{
			name:       "plain text 404 becomes an envelope",
			handler:    func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) },
			wantStatus: http.StatusNotFound,
			wantCode:   CodeNotFound,
		},
		{
			name: "plain text 405 becomes an envelope",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Allow", "GET")
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			},
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   CodeBadRequest,
		},
		{
			name: "handler written json 404 is left alone",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				Error(w, http.StatusNotFound, CodeNotFound, "app not found")
			},
			wantStatus:      http.StatusNotFound,
			wantCode:        CodeNotFound,
			wantBodyPreload: "app not found",
		},
		{
			name: "success is left alone",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				WriteJSON(w, http.StatusOK, map[string]string{"ok": "yes"})
			},
			wantStatus:      http.StatusOK,
			wantBodyPreload: `"ok"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ErrorEnvelope(tt.handler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/nope", nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("content-type = %q, want application/json", ct)
			}
			if tt.wantBodyPreload != "" && !strings.Contains(rec.Body.String(), tt.wantBodyPreload) {
				t.Fatalf("body = %q, want it to contain %q", rec.Body.String(), tt.wantBodyPreload)
			}
			if tt.wantCode == "" {
				return
			}
			var got ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("unmarshal %q: %v", rec.Body.String(), err)
			}
			if got.Error.Code != tt.wantCode {
				t.Fatalf("code = %q, want %q", got.Error.Code, tt.wantCode)
			}
		})
	}
}
