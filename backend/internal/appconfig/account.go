package appconfig

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/auth"
)

// Option configures New.
type Option func(*Handler)

// WithUserDeleter makes DELETE /v1/me also delete the Firebase Auth user.
// Without it (AUTH_MODE=dev, tests) only the stored data is deleted.
func WithUserDeleter(d auth.UserDeleter) Option {
	return func(h *Handler) { h.users = d }
}

// deleteMe is DELETE /v1/me (#119, App Store 5.1.1(v)): physically deletes
// everything the caller owns, then the sign-in account. Idempotent: a retry
// after any failure deletes what is left and still answers 204.
func (h *Handler) deleteMe(w http.ResponseWriter, r *http.Request) {
	ctx, uid := r.Context(), uidOf(r)
	if err := h.store.DeleteUserData(ctx, uid); err != nil {
		fail(w, fmt.Errorf("delete user data: %w", err))
		return
	}
	h.forget(uid)
	// Last, so that a failure above leaves a token the client can retry with.
	if h.users != nil {
		if err := h.users.DeleteUser(ctx, uid); err != nil {
			fail(w, fmt.Errorf("delete auth user: %w", err))
			return
		}
	}
	slog.Info("appconfig: account deleted", slog.Bool("authUserDeleted", h.users != nil))
	w.WriteHeader(http.StatusNoContent)
}

// forget drops uid's parsed DSLs from the cache.
func (h *Handler) forget(uid string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for k := range h.cache {
		if strings.HasPrefix(k, uid+"/") {
			delete(h.cache, k)
		}
	}
}
