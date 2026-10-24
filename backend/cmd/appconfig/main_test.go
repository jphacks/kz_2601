package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

func testHandler() http.Handler {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	return newHandler(auth.Middleware(nil, auth.ModeDev), logger)
}

func TestRoutes(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		debugUID   string
		wantStatus int
	}{
		{name: "health is public", path: "/health", wantStatus: http.StatusOK},
		{name: "healthz is public", path: "/healthz", wantStatus: http.StatusOK},
		{name: "apps requires auth", path: "/v1/apps", wantStatus: http.StatusUnauthorized},
		{name: "apps with debug uid", path: "/v1/apps", debugUID: "u1", wantStatus: http.StatusOK},
		{name: "unknown path is 404", path: "/v1/nope", wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.debugUID != "" {
				req.Header.Set(auth.DebugUIDHeader, tt.debugUID)
			}
			rec := httptest.NewRecorder()
			testHandler().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if rec.Header().Get(httpx.RequestIDHeader) == "" {
				t.Fatalf("missing %s header", httpx.RequestIDHeader)
			}
		})
	}
}

var openAPIErrorCodes = []string{
	"bad_request", "unauthenticated", "not_found", "version_conflict",
	"invalid_dsl", "invalid_record", "payload_too_large", "rate_limited", "internal",
}

func TestUnmatchedRoutesReturnErrorEnvelope(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantCode   string
		wantAllow  bool
	}{
		{
			name:       "unknown path is a not_found envelope",
			method:     http.MethodGet,
			path:       "/v1/nope",
			wantStatus: http.StatusNotFound,
			wantCode:   httpx.CodeNotFound,
		},
		{
			name:       "unknown nested path is a not_found envelope",
			method:     http.MethodGet,
			path:       "/v1/apps/x/nope",
			wantStatus: http.StatusNotFound,
			wantCode:   httpx.CodeNotFound,
		},
		{
			name:       "wrong method is a bad_request envelope",
			method:     http.MethodPost,
			path:       "/v1/apps",
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   httpx.CodeBadRequest,
			wantAllow:  true,
		},
		{
			name:       "wrong method on health is a bad_request envelope",
			method:     http.MethodDelete,
			path:       "/health",
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   httpx.CodeBadRequest,
			wantAllow:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")

			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Header.Set(auth.DebugUIDHeader, "u1")
			rec := httptest.NewRecorder()
			testHandler().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("content-type = %q, want application/json", ct)
			}
			if tt.wantAllow && rec.Header().Get("Allow") == "" {
				t.Fatalf("missing Allow header on 405")
			}

			var got httpx.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("unmarshal %q: %v", rec.Body.String(), err)
			}
			if got.Error.Code != tt.wantCode {
				t.Fatalf("code = %q, want %q", got.Error.Code, tt.wantCode)
			}
			if !slices.Contains(openAPIErrorCodes, got.Error.Code) {
				t.Fatalf("code %q is not in the OpenAPI Error.code enum", got.Error.Code)
			}
			if got.Error.Message == "" {
				t.Fatalf("message is empty: %s", rec.Body.String())
			}
			if got.Error.RequestID == "" || got.Error.RequestID != rec.Header().Get(httpx.RequestIDHeader) {
				t.Fatalf("requestId = %q, header = %q", got.Error.RequestID, rec.Header().Get(httpx.RequestIDHeader))
			}
			if strings.Contains(rec.Body.String(), "page not found") {
				t.Fatalf("stdlib plain text leaked: %s", rec.Body.String())
			}
		})
	}
}

func TestListAppsReturnsEmptyItems(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")

	req := httptest.NewRequest(http.MethodGet, "/v1/apps", nil)
	req.Header.Set(auth.DebugUIDHeader, "u1")
	rec := httptest.NewRecorder()
	testHandler().ServeHTTP(rec, req)

	var got struct {
		Items []any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", rec.Body.String(), err)
	}
	if got.Items == nil || len(got.Items) != 0 {
		t.Fatalf("items = %v, want an empty array", got.Items)
	}
}

func TestEnvOr(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "unset falls back", value: "", want: "8080"},
		{name: "set wins", value: "9090", want: "9090"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PORT", tt.value)
			if got := envOr("PORT", "8080"); got != tt.want {
				t.Fatalf("envOr = %q, want %q", got, tt.want)
			}
		})
	}
}
