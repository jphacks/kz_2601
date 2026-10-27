package auth

import (
	"context"
	"log"
	"net/http"
	"strings"
)

// Optional puts the caller's uid in the context when the request carries a
// valid ID token (or, in dev mode with the emulator, X-Debug-Uid) and passes
// every request on, verified or not. Handlers that need a caller check
// UID(ctx) themselves (ai: DELETE /v1/chat/{appId}, #286). A uid already in
// the context (set by an outer middleware) is kept, so the token is verified
// only once.
func Optional(verifier TokenVerifier, mode string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if UID(r.Context()) != "" {
				next.ServeHTTP(w, r)
				return
			}
			if debugUIDAllowed(mode) {
				if uid := r.Header.Get(DebugUIDHeader); uid != "" {
					next.ServeHTTP(w, withUID(r, uid))
					return
				}
			}
			raw, ok := cutBearerPrefix(r.Header.Get("Authorization"))
			raw = strings.TrimSpace(raw)
			if ok && raw != "" && verifier != nil {
				if uid, err := verifier.VerifyIDToken(r.Context(), raw); err == nil && uid != "" {
					r = withUID(r, uid)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ContextWithUID returns ctx carrying uid as the verified caller, for a
// middleware that verified the token itself (e.g. the ai rate limiter).
func ContextWithUID(ctx context.Context, uid string) context.Context {
	return context.WithValue(ctx, ctxKey{}, uid)
}

// Verifier is what FirebaseVerifier / DevVerifier / NotConfigured all are:
// a uid-only TokenVerifier (Middleware, Optional) and an IdentityVerifier.
type Verifier interface {
	TokenVerifier
	IdentityVerifier
}

// VerifierFromEnv builds the verifier for mode: DevVerifier for AUTH_MODE=dev,
// otherwise the Firebase Admin SDK for projectID. Without a project, or when
// the SDK cannot start, it logs why and returns NotConfigured, which rejects
// every token (fail closed).
func VerifierFromEnv(ctx context.Context, mode, projectID string) Verifier {
	if mode == ModeDev {
		log.Printf("warning: AUTH_MODE=dev: bearer tokens are trusted as uids; never use in production")
		return DevVerifier{}
	}
	if projectID == "" {
		log.Printf("auth: GOOGLE_CLOUD_PROJECT is unset; tokens will be rejected (AUTH_MODE=dev for local runs)")
		return NotConfigured{}
	}
	v, err := NewFirebaseVerifier(ctx, projectID)
	if err != nil {
		log.Printf("auth: firebase verifier init failed: %v (tokens will be rejected)", err)
		return NotConfigured{}
	}
	return v
}
