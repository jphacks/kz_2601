package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusCreated, map[string]string{"name": "家計簿 <b>"})

	if got := rec.Code; got != http.StatusCreated {
		t.Fatalf("status = %d, want %d", got, http.StatusCreated)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content-type = %q", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"家計簿 <b>"`) {
		t.Fatalf("body = %q, want unescaped japanese and html", body)
	}
}

func TestError(t *testing.T) {
	tests := []struct {
		name      string
		requestID string
		status    int
		code      string
		message   string
	}{
		{name: "unauthenticated", status: http.StatusUnauthorized, code: CodeUnauthenticated, message: "missing bearer token"},
		{name: "with request id", requestID: "req-1", status: http.StatusNotFound, code: CodeNotFound, message: "not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			if tt.requestID != "" {
				rec.Header().Set(RequestIDHeader, tt.requestID)
			}
			Error(rec, tt.status, tt.code, tt.message)

			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}
			var envelope map[string]map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			body, ok := envelope["error"]
			if !ok || len(envelope) != 1 {
				t.Fatalf("envelope = %v, want a single \"error\" key", envelope)
			}
			if body["code"] != tt.code || body["message"] != tt.message {
				t.Fatalf("error body = %v", body)
			}
			if _, ok := body["details"]; ok {
				t.Fatalf("details must be omitted when empty: %v", body)
			}
			if tt.requestID == "" {
				if _, ok := body["requestId"]; ok {
					t.Fatalf("requestId must be omitted when empty: %v", body)
				}
			} else if body["requestId"] != tt.requestID {
				t.Fatalf("requestId = %v, want %q", body["requestId"], tt.requestID)
			}
		})
	}
}

func TestErrorWithDetails(t *testing.T) {
	rec := httptest.NewRecorder()
	ErrorWithDetails(rec, http.StatusConflict, CodeVersionConflict, "current version is 7", map[string]any{"currentVersion": 7})

	var got ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Error.Details["currentVersion"] != float64(7) {
		t.Fatalf("details = %v", got.Error.Details)
	}
}

type decodeTarget struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestDecodeBody(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantOK     bool
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{name: "valid", body: `{"name":"a","count":1}`, wantOK: true},
		{name: "empty body", body: "", wantStatus: http.StatusBadRequest, wantCode: CodeBadRequest, wantMsg: "request body is empty"},
		{name: "malformed json", body: `{"name":`, wantStatus: http.StatusBadRequest, wantCode: CodeBadRequest, wantMsg: "request body contains malformed JSON"},
		{name: "unknown field", body: `{"nope":1}`, wantStatus: http.StatusBadRequest, wantCode: CodeBadRequest, wantMsg: `request body contains unknown field "nope"`},
		{name: "type mismatch", body: `{"count":"x"}`, wantStatus: http.StatusBadRequest, wantCode: CodeBadRequest, wantMsg: `field "count" must be of type int`},
		{name: "not an object", body: `[1,2]`, wantStatus: http.StatusBadRequest, wantCode: CodeBadRequest, wantMsg: "request body must be a JSON object"},
		{name: "too large", body: `{"name":"` + strings.Repeat("a", MaxRequestBytes) + `"}`, wantStatus: http.StatusRequestEntityTooLarge, wantCode: CodePayloadTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			var target decodeTarget

			if got := DecodeBody(rec, req, &target); got != tt.wantOK {
				t.Fatalf("DecodeBody = %v, want %v (body %q)", got, tt.wantOK, rec.Body.String())
			}
			if tt.wantOK {
				return
			}
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			var got ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.Error.Code != tt.wantCode {
				t.Fatalf("code = %q, want %q", got.Error.Code, tt.wantCode)
			}
			if tt.wantMsg != "" && got.Error.Message != tt.wantMsg {
				t.Fatalf("message = %q, want %q", got.Error.Message, tt.wantMsg)
			}
		})
	}
}

func TestDecodeBodyHidesInternalTypeNames(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "struct field", body: `{"name":{"a":1}}`},
		{name: "nested struct", body: `{"count":{}}`},
		{name: "whole document", body: `"plain string"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			var target decodeTarget
			DecodeBody(rec, req, &target)

			body := rec.Body.String()
			for _, leak := range []string{"httpx.", "decodeTarget", "struct {"} {
				if strings.Contains(body, leak) {
					t.Fatalf("response leaks internal type name %q: %s", leak, body)
				}
			}
		})
	}
}
