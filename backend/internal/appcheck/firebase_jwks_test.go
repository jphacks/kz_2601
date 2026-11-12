package appcheck

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	fbappcheck "firebase.google.com/go/v4/appcheck"
)

const (
	testProject = "demo-appcheck"
	testKID     = "test-kid"
	testIssuer  = "https://firebaseappcheck.googleapis.com/123456"
)

// fakeJWKS serves a generated RSA key as the App Check JWKS and points the
// Admin SDK at it for this test (no network beyond the loopback server).
func fakeJWKS(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	b64 := base64.RawURLEncoding.EncodeToString
	jwks := map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "kid": testKID, "use": "sig", "alg": "RS256",
		"n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes()),
	}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	t.Cleanup(srv.Close)
	old := fbappcheck.JWKSUrl
	fbappcheck.JWKSUrl = srv.URL
	t.Cleanup(func() { fbappcheck.JWKSUrl = old })
	return key
}

// signRS256 builds a JWT signed by key with the given header overrides.
func signRS256(t *testing.T, key *rsa.PrivateKey, header, claims map[string]any) string {
	t.Helper()
	h := map[string]any{"alg": "RS256", "typ": "JWT", "kid": testKID}
	for k, v := range header {
		h[k] = v
	}
	enc := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	signing := enc(h) + "." + enc(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func appCheckClaims(over map[string]any) map[string]any {
	now := time.Now().Unix()
	c := map[string]any{
		"iss": testIssuer, "sub": "1:123:ios:abc",
		"aud": []any{"projects/123456", "projects/" + testProject},
		"iat": now - 10, "exp": now + 3600,
	}
	for k, v := range over {
		if v == nil {
			delete(c, k)
			continue
		}
		c[k] = v
	}
	return c
}

func TestFirebaseVerifierVerify(t *testing.T) {
	key := fakeJWKS(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel) // stops the JWKS refresh goroutine
	v, err := NewFirebaseVerifier(ctx, testProject)
	if err != nil {
		t.Fatalf("NewFirebaseVerifier: %v", err)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()

	tests := []struct {
		name   string
		token  string
		wantID string // "" = must be rejected
	}{
		{name: "valid", token: signRS256(t, key, nil, appCheckClaims(nil)), wantID: "1:123:ios:abc"},
		{name: "other project", token: signRS256(t, key, nil, appCheckClaims(map[string]any{"aud": []any{"projects/someone-else"}}))},
		{name: "other issuer", token: signRS256(t, key, nil, appCheckClaims(map[string]any{"iss": "https://evil.example/"}))},
		{name: "expired", token: signRS256(t, key, nil, appCheckClaims(map[string]any{"iat": now - 7200, "exp": now - 3600}))},
		{name: "empty subject", token: signRS256(t, key, nil, appCheckClaims(map[string]any{"sub": ""}))},
		{name: "signed by another key", token: signRS256(t, other, nil, appCheckClaims(nil))},
		{name: "unknown kid", token: signRS256(t, key, map[string]any{"kid": "nope"}, appCheckClaims(nil))},
		{name: "wrong typ", token: signRS256(t, key, map[string]any{"typ": "at+jwt"}, appCheckClaims(nil))},
		{name: "alg none", token: strings.Join(strings.Split(signRS256(t, key, map[string]any{"alg": "none"}, appCheckClaims(nil)), ".")[:2], ".") + "."},
		{name: "garbage", token: "not.a.jwt"},
		// signed but malformed claims: the SDK's unchecked type assertions
		// panic; Verify turns that into a rejection
		{name: "aud as string", token: signRS256(t, key, nil, appCheckClaims(map[string]any{"aud": "projects/" + testProject}))},
		{name: "no iss", token: signRS256(t, key, nil, appCheckClaims(map[string]any{"iss": nil}))},
		{name: "no exp", token: signRS256(t, key, nil, appCheckClaims(map[string]any{"exp": nil}))},
		{name: "aud with a number", token: signRS256(t, key, nil, appCheckClaims(map[string]any{"aud": []any{1, "projects/" + testProject}}))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := v.Verify(context.Background(), tt.token)
			if tt.wantID == "" {
				if err == nil || id != "" {
					t.Fatalf("accepted: id=%q err=%v", id, err)
				}
				return
			}
			if err != nil || id != tt.wantID {
				t.Fatalf("id=%q err=%v, want %q", id, err, tt.wantID)
			}
		})
	}
}

func TestNewFirebaseVerifierJWKSUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	old := fbappcheck.JWKSUrl
	fbappcheck.JWKSUrl = srv.URL
	t.Cleanup(func() { fbappcheck.JWKSUrl = old })
	if v, err := NewFirebaseVerifier(context.Background(), testProject); err == nil {
		t.Fatalf("verifier without keys = %+v", v)
	}
}

func TestFromEnvVerifierStates(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	long := strings.Repeat("x", 300)
	tests := []struct {
		name       string
		env        map[string]string
		verifier   func() (Verifier, error)
		token      string
		wantCode   int
		wantLog    string
		notWantLog string
		wantBody   string
	}{
		{
			// (nil, nil) is a broken constructor, not a pass: log records it
			name:     "log mode, nil verifier",
			env:      map[string]string{"APP_CHECK_MODE": "log"},
			verifier: func() (Verifier, error) { return nil, nil },
			token:    "good-ios", wantCode: http.StatusNoContent, wantLog: `"appCheck":"unconfigured"`,
		},
		{
			name:     "enforce, nil verifier fails closed",
			env:      map[string]string{"APP_CHECK_MODE": " Enforce "},
			verifier: func() (Verifier, error) { return nil, nil },
			token:    "good-ios", wantCode: http.StatusUnauthorized, wantLog: "no verifier",
			wantBody: `"reason":"unconfigured"`,
		},
		{
			name:     "enforce, app id allow list",
			env:      map[string]string{"APP_CHECK_MODE": "enforce", "APP_CHECK_APP_IDS": " ios , ,android"},
			verifier: func() (Verifier, error) { return &fakeVerifier{}, nil },
			token:    "good-web", wantCode: http.StatusUnauthorized, wantLog: `"appCheck":"wrong_app"`,
		},
		{
			name:     "enforce, allowed app",
			env:      map[string]string{"APP_CHECK_MODE": "enforce", "APP_CHECK_APP_IDS": "ios,android"},
			verifier: func() (Verifier, error) { return &fakeVerifier{}, nil },
			token:    "good-android", wantCode: http.StatusNoContent,
		},
		{
			// a verifier error is logged at most 200 bytes
			name:     "log mode, long error truncated",
			env:      map[string]string{"APP_CHECK_MODE": "log"},
			verifier: func() (Verifier, error) { return errVerifier{errors.New(long)}, nil },
			token:    "anything", wantCode: http.StatusNoContent,
			wantLog: `"error":"` + long[:200] + `"`, notWantLog: long[:201],
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			mw, _, err := FromEnv(env(tt.env), logger, tt.verifier)
			if err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/generate", nil)
			req.Header.Set(Header, tt.token)
			mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})).ServeHTTP(rec, req)
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body lacks %q: %s", tt.wantBody, rec.Body)
			}
			if tt.wantLog != "" && !strings.Contains(logs.String(), tt.wantLog) {
				t.Errorf("logs lack %q:\n%s", tt.wantLog, logs.String())
			}
			if tt.notWantLog != "" && strings.Contains(logs.String(), tt.notWantLog) {
				t.Errorf("logs contain %q", tt.notWantLog)
			}
		})
	}
}

type errVerifier struct{ err error }

func (e errVerifier) Verify(context.Context, string) (string, error) { return "", e.err }

func TestTruncate(t *testing.T) {
	for _, tt := range []struct {
		s    string
		n    int
		want string
	}{
		{"abc", 5, "abc"},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"},
		{"", 0, ""},
	} {
		if got := truncate(tt.s, tt.n); got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
		}
	}
}
