package auth

import (
	"context"
	"errors"
	"regexp"

	firebase "firebase.google.com/go/v4"
	fbauth "firebase.google.com/go/v4/auth"
)

// FirebaseVerifier verifies Firebase ID tokens with the Admin SDK
// (AUTH_MODE=firebase). Credentials come from ADC (Cloud Run SA locally
// `gcloud auth application-default login`).
type FirebaseVerifier struct{ client *fbauth.Client }

// NewFirebaseVerifier initialises the Admin SDK for projectID.
func NewFirebaseVerifier(ctx context.Context, projectID string) (*FirebaseVerifier, error) {
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	client, err := app.Auth(ctx)
	if err != nil {
		return nil, err
	}
	return &FirebaseVerifier{client: client}, nil
}

// VerifyIDToken implements TokenVerifier.
func (f *FirebaseVerifier) VerifyIDToken(ctx context.Context, token string) (string, error) {
	tok, err := f.client.VerifyIDToken(ctx, token)
	if err != nil {
		return "", err
	}
	return tok.UID, nil
}

var devUID = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

// DevVerifier (AUTH_MODE=dev only) treats `Authorization: Bearer <uid>` as
// the uid itself. The ai service forwards the caller's Authorization header
// untouched, so this lets `curl -H 'Authorization: Bearer dev-smoke' ai/v1/generate`
// reach appconfig end-to-end without a Firebase token. Never use in firebase mode.
type DevVerifier struct{}

// VerifyIDToken implements TokenVerifier.
func (DevVerifier) VerifyIDToken(_ context.Context, token string) (string, error) {
	if !devUID.MatchString(token) {
		return "", errors.New("dev bearer must match " + devUID.String())
	}
	return token, nil
}
