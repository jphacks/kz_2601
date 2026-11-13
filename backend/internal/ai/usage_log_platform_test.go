package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

func TestUsageLogHasClientPlatform(t *testing.T) {
	for _, tc := range []struct {
		name   string
		header string // "-" = no header
		want   string
	}{
		{"ios", "iOS", "ios"},
		{"windows", " windows ", "windows"},
		{"no header", "-", "other"},
		{"unknown value", "playstation", "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := fakeAnthropic(t, [2]string{"200", okBody("claude-sonnet-5-5")})
			c, buf := testClient(srv, "claude-sonnet-5-5")
			h := ClientPlatformMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, err := c.Complete(r.Context(), userCompletion()); err != nil {
					t.Error(err)
				}
			}))
			req := httptest.NewRequest(http.MethodPost, "/generate", nil)
			if tc.header != "-" {
				req.Header.Set(httpx.ClientPlatformHeader, tc.header)
			}
			h.ServeHTTP(httptest.NewRecorder(), req)
			lines := usageLines(t, buf)
			if len(lines) != 1 {
				t.Fatalf("want 1 ai.usage line, got %d: %s", len(lines), buf)
			}
			if got := lines[0]["clientPlatform"]; got != tc.want {
				t.Errorf("clientPlatform = %#v, want %q", got, tc.want)
			}
			if lines[0]["operation"] != OpUnknown || lines[0]["outcome"] != "ok" {
				t.Errorf("other keys changed: %v", lines[0])
			}
		})
	}
}

func TestUsageLogNoClientPlatformOutsideRequest(t *testing.T) {
	srv, _ := fakeAnthropic(t, [2]string{"200", okBody("claude-sonnet-5-5")})
	c, buf := testClient(srv, "claude-sonnet-5-5")
	if _, err := c.Complete(context.Background(), userCompletion()); err != nil {
		t.Fatal(err)
	}
	lines := usageLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("want 1 ai.usage line, got %d: %s", len(lines), buf)
	}
	if v, ok := lines[0]["clientPlatform"]; ok {
		t.Errorf("batch/ai-eval call has clientPlatform %#v", v)
	}
}
