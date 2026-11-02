package auth

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// The verified uid reaches the request's log lines as uid_hash only (#295).
func TestContextWithUIDNotesLogHash(t *testing.T) {
	const uid = "raw-uid-for-log"
	var buf bytes.Buffer
	logger := slog.New(httpx.NewCloudHandler(&buf, slog.LevelInfo, httpx.CloudConfig{}))
	h := httpx.Chain(Optional(DevVerifier{}, ModeDev)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})), httpx.WithRequestID, httpx.Log(logger))
	req := httptest.NewRequest(http.MethodGet, "/v1/chat", nil)
	req.Header.Set("Authorization", "Bearer "+uid)
	h.ServeHTTP(httptest.NewRecorder(), req)

	if !strings.Contains(buf.String(), `"uid_hash":"`+httpx.UIDHash(uid)+`"`) {
		t.Fatalf("access log has no uid_hash: %s", buf.String())
	}
	if strings.Contains(buf.String(), uid) {
		t.Fatalf("raw uid leaked: %s", buf.String())
	}
}
