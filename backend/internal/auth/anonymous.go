package auth

import (
	"context"
	"strings"
)

type anonKey struct{}

// ContextWithIdentity is ContextWithUID that also keeps whether the caller
// signed in anonymously (Anonymous).
func ContextWithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ContextWithUID(ctx, id.UID), anonKey{}, id.Anonymous)
}

// Anonymous reports whether the verified caller is a Firebase anonymous
// account (false when unknown).
func Anonymous(ctx context.Context) bool {
	a, _ := ctx.Value(anonKey{}).(bool)
	return a
}

// verifyIdentity verifies token, reporting the sign-in provider when the
// verifier can (IdentityVerifier); a uid-only verifier reports not anonymous.
func verifyIdentity(ctx context.Context, verifier TokenVerifier, token string) (Identity, error) {
	if iv, ok := verifier.(IdentityVerifier); ok {
		return iv.VerifyIdentity(ctx, token)
	}
	uid, err := verifier.VerifyIDToken(ctx, token)
	return Identity{UID: uid}, err
}

// debugIdentity is the X-Debug-Uid caller (dev + emulator only): uids
// starting with "anon" are anonymous, as with DevVerifier.
func debugIdentity(uid string) Identity {
	return Identity{UID: uid, Anonymous: strings.HasPrefix(uid, "anon")}
}
