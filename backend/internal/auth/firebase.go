package auth

import (
	"context"
	"errors"
	"fmt"
	"os"

	firebase "firebase.google.com/go/v4"
	firebaseauth "firebase.google.com/go/v4/auth"
)

const projectIDEnv = "GOOGLE_CLOUD_PROJECT"

var (
	ErrProjectIDMissing = errors.New(projectIDEnv + " is required for firebase auth")
	ErrEmulatorHostSet  = errors.New(emulatorHostEnv + " disables ID token signature verification and must not be set in " + ModeFirebase + " auth mode")
)

func CheckEmulatorHost(mode string) error {
	if mode == ModeFirebase && os.Getenv(emulatorHostEnv) != "" {
		return ErrEmulatorHostSet
	}
	return nil
}

type idTokenVerifier interface {
	VerifyIDToken(ctx context.Context, idToken string) (*firebaseauth.Token, error)
}

var _ TokenVerifier = (*FirebaseVerifier)(nil)

type FirebaseVerifier struct {
	client idTokenVerifier
}

func NewFirebaseVerifier(ctx context.Context) (*FirebaseVerifier, error) {
	projectID := os.Getenv(projectIDEnv)
	if projectID == "" {
		return nil, ErrProjectIDMissing
	}
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		return nil, fmt.Errorf("init firebase app: %w", err)
	}
	client, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("init firebase auth client: %w", err)
	}
	return &FirebaseVerifier{client: client}, nil
}

func (v *FirebaseVerifier) VerifyIDToken(ctx context.Context, token string) (string, error) {
	if v == nil || v.client == nil {
		return "", ErrNotConfigured
	}
	decoded, err := v.client.VerifyIDToken(ctx, token)
	if err != nil {
		return "", err
	}
	return decoded.UID, nil
}
