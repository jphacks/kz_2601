package appconfig

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// CodeAccountDeleted is the 410 error code for uids that deleted their account.
const CodeAccountDeleted = "account_deleted"

// deleteMe is DELETE /v1/me (#119, App Store 5.1.1(v)). Logical deletion:
// store.Accounts.DeactivateUser marks the uid deleted, archives its apps,
// revokes its share links and takes down its catalog listings. The app deletes
// the Firebase Auth user itself; cmd/purge erases the data (and the Auth user,
// as a backstop) store.PurgeAfter later (#458). Idempotent: always 204.
func (h *Handler) deleteMe(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeactivateUser(r.Context(), uidOf(r)); err != nil {
		fail(w, fmt.Errorf("deactivate user: %w", err))
		return
	}
	slog.Info("appconfig: account deleted (logical)")
	w.WriteHeader(http.StatusNoContent)
}

// recheckAfter bounds how long an instance trusts "not deleted" for a uid
// (other Cloud Run instances learn about a deletion within this window).
const recheckAfter = 30 * time.Second

// RejectDeleted answers 410 account_deleted to every authenticated request of
// a deleted uid, except DELETE /v1/me itself (so it stays idempotent). Mount it
// inside auth.Middleware.
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
			if gone, err = s.UserDeleted(r.Context(), uid); err != nil {
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
			httpx.Error(w, http.StatusGone, CodeAccountDeleted, "this account has been deleted")
			return
		}
		next.ServeHTTP(w, r)
	})
}
