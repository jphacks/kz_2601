package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type countingVerifier struct {
	fakeVerifier
	calls *int
}

func (c countingVerifier) VerifyIDToken(ctx context.Context, token string) (string, error) {
	*c.calls++
	return c.fakeVerifier.VerifyIDToken(ctx, token)
}

func TestOptional(t *testing.T) {
	tests := []struct {
		name         string
		mode         string
		emulatorHost string
		authHeader   string
		debugUID     string
		preset       string // uid already in the context
		verifier     TokenVerifier
		wantUID      string
		wantCalls    int
	}{
		{name: "no token passes without uid", mode: ModeFirebase, verifier: fakeVerifier{uid: "u1"}},
		{name: "invalid token passes without uid", mode: ModeFirebase, authHeader: "Bearer bad-token", verifier: fakeVerifier{uid: "u1"}, wantCalls: 1},
		{name: "valid token sets uid", mode: ModeFirebase, authHeader: "Bearer good-token", verifier: fakeVerifier{uid: "u1"}, wantUID: "u1", wantCalls: 1},
		{name: "nil verifier passes without uid", mode: ModeFirebase, authHeader: "Bearer good-token"},
		{name: "unconfigured passes without uid", mode: ModeFirebase, authHeader: "Bearer good-token", verifier: NotConfigured{}},
		{name: "uid from an outer middleware is kept", mode: ModeFirebase, authHeader: "Bearer good-token", preset: "u-outer", verifier: fakeVerifier{uid: "u1"}, wantUID: "u-outer"},
		{name: "dev mode with emulator accepts debug uid", mode: ModeDev, emulatorHost: "localhost:9099", debugUID: "u-dev", verifier: fakeVerifier{uid: "u1"}, wantUID: "u-dev"},
		{name: "firebase mode ignores debug uid", mode: ModeFirebase, emulatorHost: "localhost:9099", debugUID: "u-dev", verifier: fakeVerifier{uid: "u1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(emulatorHostEnv, tt.emulatorHost)
			calls := 0
			v := tt.verifier
			if f, ok := v.(fakeVerifier); ok {
				v = countingVerifier{fakeVerifier: f, calls: &calls}
			}
			reached, seen := false, ""
			h := Optional(v, tt.mode)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reached, seen = true, UID(r.Context())
			}))
			req := httptest.NewRequest(http.MethodPost, "/v1/chat", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if tt.debugUID != "" {
				req.Header.Set(DebugUIDHeader, tt.debugUID)
			}
			if tt.preset != "" {
				req = req.WithContext(ContextWithUID(req.Context(), tt.preset))
			}
			h.ServeHTTP(httptest.NewRecorder(), req)
			if !reached || seen != tt.wantUID || calls != tt.wantCalls {
				t.Fatalf("reached=%v uid=%q calls=%d, want uid=%q calls=%d", reached, seen, calls, tt.wantUID, tt.wantCalls)
			}
		})
	}
}

func TestVerifierFromEnv(t *testing.T) {
	ctx := context.Background()
	if _, ok := VerifierFromEnv(ctx, ModeDev, "").(DevVerifier); !ok {
		t.Fatal("dev mode: DevVerifier")
	}
	if _, ok := VerifierFromEnv(ctx, ModeFirebase, "").(NotConfigured); !ok {
		t.Fatal("firebase without a project: NotConfigured (fail closed)")
	}
}
