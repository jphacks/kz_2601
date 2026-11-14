package appconfig

import (
	"net/http"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// Operator's forced stop (#346). `moderate stop-app -force` / `stop-all
// -force` stop the runner like any stop and also refuse here the record
// writes that would trigger the app's server rules (outboxPlanFor), so a
// stopped app's clients learn it at once instead of writing data that no
// rule will act on. Writes no rule matches, reads and deletes go on. The
// stop is read on each such write: it takes effect without a deploy, as
// soon as the operator writes it. The reason stays with the operator.

// checkAppStop returns 403 app_stopped while a forced global or app stop
// is active in env.
func (h *Handler) checkAppStop(r *http.Request, env, appID string) error {
	ids := []string{store.GlobalStopID, store.AppStopID(appID)}
	got, err := h.store.ActiveStops(r.Context(), env, ids, time.Now())
	if err != nil {
		return err
	}
	for _, id := range ids {
		s, ok := got[id]
		if !ok || !s.Force {
			continue
		}
		e := apiErr(http.StatusForbidden, httpx.CodeAppStopped, "the operator stopped this app's server features").With("level", s.Level)
		if !s.Until.IsZero() {
			e = e.With("until", s.Until.UTC().Format(time.RFC3339))
		}
		return e
	}
	return nil
}
