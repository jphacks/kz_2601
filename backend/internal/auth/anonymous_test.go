package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// #341: the middleware keeps whether the caller is an anonymous account.
func TestMiddlewareAnonymous(t *testing.T) {
	t.Setenv(emulatorHostEnv, "localhost:9099")
	for _, tc := range []struct {
		name, header, value string
		verifier            TokenVerifier
		want                bool
	}{
		{"dev bearer anon", "Authorization", "Bearer anon-1", DevVerifier{}, true},
		{"dev bearer user", "Authorization", "Bearer user-1", DevVerifier{}, false},
		{"debug uid anon", DebugUIDHeader, "anon-2", nil, true},
		{"debug uid user", DebugUIDHeader, "user-2", nil, false},
		{"uid-only verifier", "Authorization", "Bearer anon-3", uidOnly{}, false},
	} {
		var got, called bool
		h := Middleware(tc.verifier, ModeDev)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			called, got = true, Anonymous(r.Context())
		}))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(tc.header, tc.value)
		h.ServeHTTP(httptest.NewRecorder(), req)
		if !called || got != tc.want {
			t.Errorf("%s: called %v anonymous %v, want %v", tc.name, called, got, tc.want)
		}
	}
}

type uidOnly struct{}

func (uidOnly) VerifyIDToken(_ context.Context, token string) (string, error) { return token, nil }
