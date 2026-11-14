package appconfig

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/runner/schedule"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Schedules (#342): saving a DSL that has (or had) time-triggered rules
// queues rules_changed in the same transaction as the version (Commit.Events),
// so the runner rebuilds the app's schedules from exactly that version.
// Off unless RUNNER_OUTBOX=1, like the record triggers.

// TimeZoneHeader carries the device's IANA zone (iOS TimeZone.current).
const TimeZoneHeader = "X-Time-Zone"

// requestTimeZone is the caller's IANA zone, or schedule.DefaultZone when
// the header is missing or names no zone.
func requestTimeZone(r *http.Request) string {
	if tz := headerTimeZone(r); tz != "" {
		return tz
	}
	return schedule.DefaultZone
}

// headerTimeZone is the zone X-Time-Zone names, "" when it names none.
func headerTimeZone(r *http.Request) string {
	name := strings.TrimSpace(r.Header.Get(TimeZoneHeader))
	if name == "" || name == "Local" || len(name) > 64 {
		return ""
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return ""
	}
	return loc.String()
}

func hasScheduledRules(a *dsl.App) bool {
	if a == nil {
		return false
	}
	for i := range a.ServerRules {
		if a.ServerRules[i].IsScheduled() {
			return true
		}
	}
	return false
}

// rulesChanged is the Commit.Events of saving next over prev (the stored
// DSL before the save; nil for a new app): one rules_changed when either
// has time-triggered rules, else none.
func (h *Handler) rulesChanged(r *http.Request, appID string, prev json.RawMessage, next *dsl.App) []store.OutboxEvent {
	if !h.outbox.on {
		return nil
	}
	if !hasScheduledRules(next) {
		var old dsl.App
		if len(prev) == 0 || json.Unmarshal(prev, &old) != nil || !hasScheduledRules(&old) {
			return nil
		}
	}
	uid := uidOf(r)
	for i := range next.Entities {
		if next.Entities[i].IsShared() {
			// only the creator's DSL counts for shared data (#337): a
			// participant's copy queues nothing ("" = not claimed yet: the
			// creator's first save)
			if owner, err := h.appOwner(r.Context(), appID); err == nil && owner != "" && owner != uid {
				return nil
			}
			break
		}
	}
	return []store.OutboxEvent{{
		Env: h.outbox.env, Kind: store.OutboxRulesChanged, AppID: appID, UID: uid,
		ActorUID: uid, ActorAnonymous: auth.Anonymous(r.Context()), TimeZone: requestTimeZone(r),
	}}
}
