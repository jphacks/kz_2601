package auth

import (
	"context"
	"errors"
	"testing"

	firebaseauth "firebase.google.com/go/v4/auth"
)

type stubIDTokenVerifier struct {
	token *firebaseauth.Token
	err   error
}

func (s stubIDTokenVerifier) VerifyIDToken(context.Context, string) (*firebaseauth.Token, error) {
	return s.token, s.err
}

func TestNewFirebaseVerifier(t *testing.T) {
	tests := []struct {
		name         string
		projectID    string
		emulatorHost string
		wantErr      error
	}{
		{
			name:    "missing project id is rejected",
			wantErr: ErrProjectIDMissing,
		},
		{
			name:         "emulator host needs no credentials",
			projectID:    "demo-project",
			emulatorHost: "localhost:9099",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(projectIDEnv, tt.projectID)
			t.Setenv(emulatorHostEnv, tt.emulatorHost)

			verifier, err := NewFirebaseVerifier(context.Background())
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				if verifier != nil {
					t.Fatalf("verifier = %v, want nil", verifier)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewFirebaseVerifier: %v", err)
			}
			if verifier == nil || verifier.client == nil {
				t.Fatalf("verifier = %v, want an initialized client", verifier)
			}
		})
	}
}

func TestFirebaseVerifierVerifyIDToken(t *testing.T) {
	decodeFailed := errors.New("decode failed")

	tests := []struct {
		name     string
		verifier *FirebaseVerifier
		wantUID  string
		wantErr  error
	}{
		{
			name:     "nil verifier is not configured",
			verifier: nil,
			wantErr:  ErrNotConfigured,
		},
		{
			name:     "zero verifier is not configured",
			verifier: &FirebaseVerifier{},
			wantErr:  ErrNotConfigured,
		},
		{
			name:     "sdk failure is propagated",
			verifier: &FirebaseVerifier{client: stubIDTokenVerifier{err: decodeFailed}},
			wantErr:  decodeFailed,
		},
		{
			name:     "uid comes from the decoded token",
			verifier: &FirebaseVerifier{client: stubIDTokenVerifier{token: &firebaseauth.Token{UID: "u1"}}},
			wantUID:  "u1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uid, err := tt.verifier.VerifyIDToken(context.Background(), "token")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if uid != tt.wantUID {
				t.Fatalf("uid = %q, want %q", uid, tt.wantUID)
			}
		})
	}
}
