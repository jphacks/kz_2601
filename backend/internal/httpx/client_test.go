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

// #2132: X-Client-Platform is folded into a fixed vocabulary.
func TestNormalizePlatform(t *testing.T) {
	for in, want := range map[string]string{
		"ios":                           "ios",
		"android":                       "android",
		"macos":                         "macos",
		"windows":                       "windows",
		"linux":                         "linux",
		"iOS":                           "ios",
		" Windows ":                     "windows",
		"LINUX":                         "linux",
		"":                              "other",
		"   ":                           "other",
		"other":                         "other",
		"darwin":                        "other",
		"ios; build=12":                 "other",
		strings.Repeat("a", 33):         "other",
		"ios" + strings.Repeat(" ", 40): "other",
	} {
		if got := NormalizePlatform(in); got != want {
			t.Errorf("NormalizePlatform(%q) = %q, want %q", in, got, want)
		}
	}
	for _, p := range Platforms {
		if NormalizePlatform(p) != p {
			t.Errorf("%q is not a fixed point", p)
		}
	}
}

// #2132: the request log carries the normalised platform, "other" when absent.
func TestLogClientPlatformNormalised(t *testing.T) {
	for header, want := range map[string]string{"MacOS": "macos", "beos": "other", "": "other"} {
		buf := bytes.NewBuffer(nil)
		handler := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), Log(slog.New(slog.NewJSONHandler(buf, nil))))
		req := httptest.NewRequest(http.MethodGet, "/v1/apps", nil)
		if header != "" {
			req.Header.Set(ClientPlatformHeader, header)
		}
		handler.ServeHTTP(httptest.NewRecorder(), req)
		var entry map[string]any
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatal(err)
		}
		if entry["clientPlatform"] != want {
			t.Errorf("header %q: clientPlatform = %v, want %q", header, entry["clientPlatform"], want)
		}
	}
}
