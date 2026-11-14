package appconfig

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// routes (account deletion #119)
func init() {
	registerRoutes(func(h *Handler, mux *http.ServeMux) {
		mux.HandleFunc("DELETE /v1/me", h.deleteMe)
	})
}

// deleteMe is DELETE /v1/me (#119, App Store 5.1.1(v)). Logical deletion:
// store.Accounts.DeactivateUser marks the uid deleted, archives its apps,
// revokes its share links and takes down its catalog listings. The app deletes
// the Firebase Auth user itself; cmd/purge erases the data (and the Auth user,
// as a backstop) store.PurgeAfter later (#458). Idempotent: always 204.
// The runner then revokes the external API connections at the providers
// (#1716); best effort, as cmd/purge erases the tokens anyway.
func (h *Handler) deleteMe(w http.ResponseWriter, r *http.Request) {
	uid := uidOf(r)
	if err := h.store.DeactivateUser(r.Context(), uid); err != nil {
		fail(w, fmt.Errorf("deactivate user: %w", err))
		return
	}
	slog.Info("appconfig: account deleted (logical)")
	h.revokeExt(r.Context(), uid)
	w.WriteHeader(http.StatusNoContent)
}

// Account deletion waits at most revokeWait for the runner; the revocation
// goes on in the background (bounded by revokeTimeout) when it takes longer,
// so the person is not kept waiting and the runner's call is not cut off.
var (
	revokeWait    = 3 * time.Second
	revokeTimeout = 30 * time.Second
)

// revokeExt asks the runner to revoke uid's connections. A failure is only
// logged: it never fails the deletion.
func (h *Handler) revokeExt(ctx context.Context, uid string) {
	if h.ext == nil {
		return
	}
	done := make(chan struct{})
	bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), revokeTimeout)
	go func() {
		defer close(done)
		defer cancel()
		n, err := h.ext.RevokeAll(bg, uid)
		if err != nil {
			slog.WarnContext(bg, "appconfig: revoking external connections on delete failed", slog.Any("error", err))
			return
		}
		if n > 0 {
			slog.InfoContext(bg, "appconfig: external connections revoked on delete", slog.Int("revoked", n))
		}
	}()
	t := time.NewTimer(revokeWait)
	defer t.Stop()
	select {
	case <-done:
	case <-t.C:
		slog.InfoContext(ctx, "appconfig: revoking external connections continues in the background")
	case <-ctx.Done():
	}
}

// recheckAfter bounds how long an instance trusts "not deleted" for a uid
// (other Cloud Run instances learn about a deletion within this window).
const recheckAfter = 30 * time.Second

// RejectDeleted answers 410 account_deleted to every authenticated request of
// a deleted uid, except DELETE /v1/me itself (so it stays idempotent). Mount it
// inside auth.Middleware. Its lookup (store.Accounts.SeeUser) also records
// users/{uid}.firstSeenAt for the runner's 24h rule (#339): the same read per
// uid per recheckAfter, plus one write the first time a uid is seen.
func RejectDeleted(s store.Accounts, next http.Handler) http.Handler {
	var (
		mu      sync.Mutex
		deleted = map[string]bool{}      // permanent
		active  = map[string]time.Time{} // uid -> checked at
	)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := auth.UID(r.Context())
		if r.Method == http.MethodDelete && r.URL.Path == "/v1/me" {
			next.ServeHTTP(w, r)
			mu.Lock()
			delete(active, uid) // re-read on the next request
			mu.Unlock()
			return
		}
		mu.Lock()
		gone, checked := deleted[uid], active[uid]
		mu.Unlock()
		if !gone && time.Since(checked) > recheckAfter {
			var err error
			if gone, err = s.SeeUser(r.Context(), uid); err != nil {
				fail(w, fmt.Errorf("account status: %w", err))
				return
			}
			mu.Lock()
			if gone {
				deleted[uid] = true
			} else {
				if len(active) > 10000 {
					active = map[string]time.Time{}
				}
				active[uid] = time.Now()
			}
			mu.Unlock()
		}
		if gone {
			httpx.Error(w, http.StatusGone, httpx.CodeAccountDeleted, "this account has been deleted")
			return
		}
		next.ServeHTTP(w, r)
	})
}
