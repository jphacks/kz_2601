package auth

import (
	"context"
	"slices"
	"strings"
)

type (
	anonKey     struct{}
	subjectsKey struct{}
)

// ContextWithIdentity is ContextWithUID that also keeps whether the caller
// signed in anonymously (Anonymous) and its Subjects.
func ContextWithIdentity(ctx context.Context, id Identity) context.Context {
	ctx = context.WithValue(ContextWithUID(ctx, id.UID), anonKey{}, id.Anonymous)
	return context.WithValue(ctx, subjectsKey{}, slices.Clone(id.Subjects))
}

// Anonymous reports whether the verified caller is a Firebase anonymous
// account (false when unknown).
func Anonymous(ctx context.Context) bool {
	a, _ := ctx.Value(anonKey{}).(bool)
	return a
}

// Subjects are the verified caller's Identity.Subjects (nil when unknown).
func Subjects(ctx context.Context) []string {
	s, _ := ctx.Value(subjectsKey{}).([]string)
	return s
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

// debugIdentity is the X-Debug-Uid / DevVerifier caller (dev only): uids
// starting with "anon" are anonymous; the others are their own subject.
func debugIdentity(uid string) Identity {
	anon := strings.HasPrefix(uid, "anon")
	return Identity{UID: uid, Anonymous: anon, Subjects: devSubjects(uid, anon)}
}
