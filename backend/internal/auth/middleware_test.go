package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

type fakeVerifier struct {
	uid string
	err error
}

func (f fakeVerifier) VerifyIDToken(_ context.Context, token string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if token != "good-token" {
		return "", errors.New("invalid token")
	}
	return f.uid, nil
}

type recordingVerifier struct {
	seen *string
	uid  string
}

func (v recordingVerifier) VerifyIDToken(_ context.Context, token string) (string, error) {
	*v.seen = token
	if token != "good-token" {
		return "", errors.New("invalid token")
	}
	return v.uid, nil
}

func TestMiddleware(t *testing.T) {
	tests := []struct {
		name         string
		mode         string
		emulatorHost string
		authHeader   string
		debugUID     string
		verifier     TokenVerifier
		wantStatus   int
		wantUID      string
	}{
		{
			name:       "missing token is unauthenticated",
			mode:       ModeFirebase,
			verifier:   fakeVerifier{uid: "u1"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "non bearer scheme is unauthenticated",
			mode:       ModeFirebase,
			authHeader: "Basic abc",
			verifier:   fakeVerifier{uid: "u1"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "empty bearer value is unauthenticated",
			mode:       ModeFirebase,
			authHeader: "Bearer   ",
			verifier:   fakeVerifier{uid: "u1"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid token is unauthenticated",
			mode:       ModeFirebase,
			authHeader: "Bearer bad-token",
			verifier:   fakeVerifier{uid: "u1"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "verifier failure is unauthenticated",
			mode:       ModeFirebase,
			authHeader: "Bearer good-token",
			verifier:   fakeVerifier{err: errors.New("network down")},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "unconfigured verifier is unauthenticated",
			mode:       ModeFirebase,
			authHeader: "Bearer good-token",
			verifier:   NotConfigured{},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "valid token puts uid in context",
			mode:       ModeFirebase,
			authHeader: "Bearer good-token",
			verifier:   fakeVerifier{uid: "u1"},
			wantStatus: http.StatusOK,
			wantUID:    "u1",
		},
		{
			name:         "dev mode with emulator accepts debug uid",
			mode:         ModeDev,
			emulatorHost: "localhost:9099",
			debugUID:     "u-dev",
			verifier:     fakeVerifier{uid: "u1"},
			wantStatus:   http.StatusOK,
			wantUID:      "u-dev",
		},
		{
			name:         "firebase mode ignores debug uid",
			mode:         ModeFirebase,
			emulatorHost: "localhost:9099",
			debugUID:     "u-dev",
			verifier:     fakeVerifier{uid: "u1"},
			wantStatus:   http.StatusUnauthorized,
		},
		{
			name:       "dev mode without emulator ignores debug uid",
			mode:       ModeDev,
			debugUID:   "u-dev",
			verifier:   fakeVerifier{uid: "u1"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:         "dev mode still verifies real tokens",
			mode:         ModeDev,
			emulatorHost: "localhost:9099",
			authHeader:   "Bearer good-token",
			verifier:     fakeVerifier{uid: "u1"},
			wantStatus:   http.StatusOK,
			wantUID:      "u1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(emulatorHostEnv, tt.emulatorHost)

			var seen string
			handler := Middleware(tt.verifier, tt.mode)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = UID(r.Context())
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/v1/apps", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if tt.debugUID != "" {
				req.Header.Set(DebugUIDHeader, tt.debugUID)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if seen != tt.wantUID {
				t.Fatalf("uid = %q, want %q", seen, tt.wantUID)
			}
			if tt.wantStatus != http.StatusUnauthorized {
				return
			}
			var got httpx.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.Error.Code != httpx.CodeUnauthenticated {
				t.Fatalf("code = %q, want %q", got.Error.Code, httpx.CodeUnauthenticated)
			}
		})
	}
}

func TestBearerSchemeIsCaseInsensitive(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		wantStatus int
		wantUID    string
		wantToken  string
	}{
		{name: "canonical case", authHeader: "Bearer good-token", wantStatus: http.StatusOK, wantUID: "u1", wantToken: "good-token"},
		{name: "lower case scheme", authHeader: "bearer good-token", wantStatus: http.StatusOK, wantUID: "u1", wantToken: "good-token"},
		{name: "upper case scheme", authHeader: "BEARER good-token", wantStatus: http.StatusOK, wantUID: "u1", wantToken: "good-token"},
		{name: "mixed case scheme", authHeader: "BeArEr good-token", wantStatus: http.StatusOK, wantUID: "u1", wantToken: "good-token"},
		{name: "token case is preserved", authHeader: "bearer GoOd-ToKeN", wantStatus: http.StatusUnauthorized, wantToken: "GoOd-ToKeN"},
		{name: "other scheme is still rejected", authHeader: "bearerish good-token", wantStatus: http.StatusUnauthorized},
		{name: "scheme without separator is rejected", authHeader: "bearergood-token", wantStatus: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(emulatorHostEnv, "")

			var seenToken string
			verifier := recordingVerifier{seen: &seenToken, uid: "u1"}

			var seenUID string
			handler := Middleware(verifier, ModeFirebase)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seenUID = UID(r.Context())
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/v1/apps", nil)
			req.Header.Set("Authorization", tt.authHeader)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if seenUID != tt.wantUID {
				t.Fatalf("uid = %q, want %q", seenUID, tt.wantUID)
			}
			if seenToken != tt.wantToken {
				t.Fatalf("token passed to verifier = %q, want %q", seenToken, tt.wantToken)
			}
		})
	}
}

func TestUIDWithoutMiddleware(t *testing.T) {
	if got := UID(context.Background()); got != "" {
		t.Fatalf("UID = %q, want empty", got)
	}
}
