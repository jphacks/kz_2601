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

// #302: the iOS version headers end up in the request log.
func TestLogClientVersion(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	logger := slog.New(slog.NewJSONHandler(buf, nil))
	handler := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), Log(logger))

	req := httptest.NewRequest(http.MethodGet, "/v1/apps", nil)
	req.Header.Set(ClientPlatformHeader, "ios")
	req.Header.Set(ClientBuildHeader, "12")
	req.Header.Set(ClientVersionHeader, strings.Repeat("9", 64))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["clientPlatform"] != "ios" || entry["clientBuild"] != "12" || len(entry["clientVersion"].(string)) != 32 {
		t.Fatalf("entry = %v", entry)
	}
	if _, ok := entry["clientDslVersion"]; ok {
		t.Fatal("absent header must not be logged")
	}
}
