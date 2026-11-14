package appconfig

import (
	"log/slog"
	"net/http"
	"sync"

	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// App activity (#342): the runner stops the schedules of an app nobody
// opened for 30 days (#338) and resumes them once it is opened again.
// GET /dsl (opening the app) writes runner/{env}/apps/{key}.lastOpenedAt,
// at most once a day per app and instance. Off unless RUNNER_OUTBOX=1.

// maxActivityKeys bounds the in-memory "written today" set.
const maxActivityKeys = 10000

type activity struct {
	mu   sync.Mutex
	day  string
	seen map[string]bool
}

// first reports whether key was not written yet on day (and marks it).
func (a *activity) first(day, key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.day != day || len(a.seen) >= maxActivityKeys {
		a.day, a.seen = day, map[string]bool{}
	}
	if a.seen[key] {
		return false
	}
	a.seen[key] = true
	return true
}

func (a *activity) forget(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.seen, key)
}

// activityKey is the runner's key of the app as uid opens it: the appId for
// an app with shared / public data (anyone opening it counts), appId + uid
// for a private one (each user's copy has its own schedules).
func activityKey(appID, uid string, a *dsl.App) string {
	for i := range a.Entities {
		if a.Entities[i].IsShared() {
			return store.TenantKey(appID, "")
		}
	}
	return store.TenantKey(appID, uid)
}

// touchOpened records that uid opened app. Failures are logged: opening
// the app must not fail because of it.
func (h *Handler) touchOpened(r *http.Request, app *store.App) {
	if !h.outbox.on {
		return
	}
	parsed, err := h.parsedDSL(uidOf(r), app)
	if err != nil {
		return
	}
	key := activityKey(app.ID, uidOf(r), parsed)
	now := store.Now()
	if !h.activity.first(now.Format("2006-01-02"), key) {
		return
	}
	if err := h.store.TouchAppOpened(r.Context(), h.outbox.env, key, now); err != nil {
		h.activity.forget(key)
		slog.Warn("appconfig: recording app activity failed", slog.String("appId", app.ID), slog.Any("error", err))
	}
}
