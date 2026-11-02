package ai

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// A 5xx on the ai request path is logged at ERROR with the request's
// requestId and trace, like the access log (#295).
func TestErrorResponseLog_CarriesRequestIDAndTrace(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "sse"}[stream], func(t *testing.T) {
			var buf bytes.Buffer
			svc, _ := newSvc(&fakeCaller{}) // no replies: Claude fails
			svc.Logger = slog.New(httpx.NewCloudHandler(&buf, slog.LevelDebug, httpx.CloudConfig{ProjectID: "proj", Service: "ai-stg"}))
			h := httpx.WithRequestID(NewHandler(svc))

			url := "/v1/generate"
			if stream {
				url += "?stream=1"
			}
			body, _ := json.Marshal(Request{AppID: "app-RAW-1", Transcript: secretPrompt})
			req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
			req.Header.Set(httpx.RequestIDHeader, "req-295")
			req.Header.Set("traceparent", "00-105445aa7843bc8bf206b12000100000-0000000000000001-01")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			var line map[string]any
			for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
				var m map[string]any
				if err := json.Unmarshal([]byte(l), &m); err != nil {
					t.Fatalf("not JSON: %q", l)
				}
				if m["message"] == "ai: error response" {
					line = m
				}
			}
			if line == nil {
				t.Fatalf("no error response line (status %d): %s", rec.Code, buf.String())
			}
			want := map[string]any{
				"severity":                     "ERROR",
				"requestId":                    "req-295",
				"logging.googleapis.com/trace": "projects/proj/traces/105445aa7843bc8bf206b12000100000",
			}
			for k, v := range want {
				if line[k] != v {
					t.Errorf("%s = %v, want %v", k, line[k], v)
				}
			}
			if s, _ := line["status"].(float64); s < 500 {
				t.Errorf("status = %v, want 5xx", line["status"])
			}
			if strings.Contains(buf.String(), secretPrompt) || strings.Contains(buf.String(), "app-RAW-1") {
				t.Errorf("prompt or appId leaked: %s", buf.String())
			}
		})
	}
}
