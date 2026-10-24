package auth

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

const (
	ModeDev      = "dev"
	ModeFirebase = "firebase"

	DebugUIDHeader  = "X-Debug-Uid"
	emulatorHostEnv = "FIREBASE_AUTH_EMULATOR_HOST"
	bearerPrefix    = "Bearer "
)

type ctxKey struct{}

type TokenVerifier interface {
	VerifyIDToken(ctx context.Context, token string) (string, error)
}

var ErrNotConfigured = errors.New("token verifier is not configured")

type NotConfigured struct{}

func (NotConfigured) VerifyIDToken(context.Context, string) (string, error) {
	return "", ErrNotConfigured
}

func Middleware(verifier TokenVerifier, mode string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if debugUIDAllowed(mode) {
				if uid := r.Header.Get(DebugUIDHeader); uid != "" {
					next.ServeHTTP(w, withUID(r, uid))
					return
				}
			}
			raw, ok := cutBearerPrefix(r.Header.Get("Authorization"))
			if !ok || strings.TrimSpace(raw) == "" {
				httpx.Error(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "missing bearer token")
				return
			}
			if verifier == nil {
				httpx.Error(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "invalid token")
				return
			}
			uid, err := verifier.VerifyIDToken(r.Context(), raw)
			if err != nil || uid == "" {
				httpx.Error(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "invalid token")
				return
			}
			next.ServeHTTP(w, withUID(r, uid))
		})
	}
}

func cutBearerPrefix(header string) (string, bool) {
	if len(header) < len(bearerPrefix) || !strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
		return "", false
	}
	return header[len(bearerPrefix):], true
}

func debugUIDAllowed(mode string) bool {
	return mode == ModeDev && os.Getenv(emulatorHostEnv) != ""
}

func withUID(r *http.Request, uid string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxKey{}, uid))
}

func UID(ctx context.Context) string {
	s, _ := ctx.Value(ctxKey{}).(string)
	return s
}
