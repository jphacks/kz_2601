package appcheck

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	fbappcheck "firebase.google.com/go/v4/appcheck"
)

// FirebaseVerifier verifies tokens with the Admin SDK (JWKS from
// firebaseappcheck.googleapis.com, refreshed every 6 h; no ADC needed).
type FirebaseVerifier struct{ client *fbappcheck.Client }

// NewFirebaseVerifier fetches the JWKS once; ctx also stops the refresh.
func NewFirebaseVerifier(ctx context.Context, projectID string) (*FirebaseVerifier, error) {
	if projectID == "" {
		return nil, fmt.Errorf("GOOGLE_CLOUD_PROJECT is required for App Check")
	}
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	client, err := app.AppCheck(ctx)
	if err != nil {
		return nil, err
	}
	return &FirebaseVerifier{client: client}, nil
}

// Verify implements Verifier. The SDK type-asserts claims without checks, so
// a malformed (but signed) token is turned into an error instead of a panic.
func (f *FirebaseVerifier) Verify(_ context.Context, token string) (appID string, err error) {
	defer func() {
		if v := recover(); v != nil {
			appID, err = "", fmt.Errorf("malformed app check token: %v", v)
		}
	}()
	tok, err := f.client.VerifyToken(token)
	if err != nil {
		return "", err
	}
	return tok.AppID, nil
}
