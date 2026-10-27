package auth

import (
	"context"
	"strings"
)

// Identity is the verified caller. Anonymous = Firebase anonymous sign-in
// (sign_in_provider "anonymous"); such accounts are cheap to re-create, so
// the ai rate limiter treats them more strictly (#280).
type Identity struct {
	UID       string
	Anonymous bool
}

// IdentityVerifier is a TokenVerifier that also reports the sign-in provider.
type IdentityVerifier interface {
	VerifyIdentity(ctx context.Context, token string) (Identity, error)
}

// VerifyIdentity implements IdentityVerifier.
func (f *FirebaseVerifier) VerifyIdentity(ctx context.Context, token string) (Identity, error) {
	tok, err := f.client.VerifyIDToken(ctx, token)
	if err != nil {
		return Identity{}, err
	}
	return Identity{UID: tok.UID, Anonymous: tok.Firebase.SignInProvider == "anonymous"}, nil
}

// VerifyIdentity implements IdentityVerifier. Dev uids starting with "anon"
// are reported as anonymous so the stricter tier can be exercised locally.
func (d DevVerifier) VerifyIdentity(ctx context.Context, token string) (Identity, error) {
	uid, err := d.VerifyIDToken(ctx, token)
	if err != nil {
		return Identity{}, err
	}
	return Identity{UID: uid, Anonymous: strings.HasPrefix(uid, "anon")}, nil
}

// VerifyIdentity implements IdentityVerifier.
func (NotConfigured) VerifyIdentity(context.Context, string) (Identity, error) {
	return Identity{}, ErrNotConfigured
}

// BearerToken returns the raw token of an `Authorization: Bearer <t>` header.
func BearerToken(header string) (string, bool) {
	raw, ok := cutBearerPrefix(header)
	raw = strings.TrimSpace(raw)
	return raw, ok && raw != ""
}
