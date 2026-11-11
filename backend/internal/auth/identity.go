package auth

import (
	"context"
	"slices"
	"strings"
)

// Identity is the verified caller. Anonymous = Firebase anonymous sign-in
// (sign_in_provider "anonymous"); such accounts are cheap to re-create, so
// the ai rate limiter treats them more strictly (#280).
type Identity struct {
	UID       string
	Anonymous bool
	// Subjects are the person behind the account at its sign-in providers
	// ("apple.com:<sub>", sorted), from the token's firebase.identities. A
	// new Firebase account of the same Apple ID has the same subject.
	Subjects []string
}

// subjectProviders are the sign-in providers whose subject stands for one
// person (email / phone are left out).
var subjectProviders = []string{"apple.com", "google.com"}

// subjectsOf reads firebase.identities ({"apple.com": ["<sub>"], ...}).
func subjectsOf(identities map[string]interface{}) []string {
	var out []string
	for _, p := range subjectProviders {
		ids, _ := identities[p].([]interface{})
		for _, id := range ids {
			if sub, ok := id.(string); ok && sub != "" {
				out = append(out, p+":"+sub)
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// devSubjects is the subject of a dev / debug uid: the uid itself, none
// when anonymous.
func devSubjects(uid string, anonymous bool) []string {
	if anonymous || uid == "" {
		return nil
	}
	return []string{"dev:" + uid}
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
	return Identity{UID: tok.UID, Anonymous: tok.Firebase.SignInProvider == "anonymous", Subjects: subjectsOf(tok.Firebase.Identities)}, nil
}

// VerifyIdentity implements IdentityVerifier. Dev uids starting with "anon"
// are reported as anonymous so the stricter tier can be exercised locally.
func (d DevVerifier) VerifyIdentity(ctx context.Context, token string) (Identity, error) {
	uid, err := d.VerifyIDToken(ctx, token)
	if err != nil {
		return Identity{}, err
	}
	return debugIdentity(uid), nil
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
