package appconfig

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
	"github.com/jphacks/kz_2601/backend/internal/runner/status"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// GET /v1/apps/{appId}/automations (#347): the creator's view of their app's
// server rules: what each does, when it last ran and how, whether the
// operator stopped it, and the app's executions today. It reads the
// runner's small per-rule documents (store.RuleStatus), never the outbox.
// Only the creator may read it; nothing in it is a record value, a
// notification text, someone else's uid or a stop's reason.

func init() {
	registerRoutes(func(h *Handler, mux *http.ServeMux) {
		h.runnerEnv, _ = runnerEnv(os.Getenv)
		mux.HandleFunc("GET /v1/apps/{appId}/automations", h.automations)
	})
}

type automationsResponse struct {
	AppID string `json:"appId"`
	// Stopped: the operator stopped every rule of the app.
	Stopped *automationStop `json:"stopped,omitempty"`
	// NotifyPaused: the rules run, their notifications are not sent.
	NotifyPaused *automationStop  `json:"notifyPaused,omitempty"`
	Quota        *automationQuota `json:"quota,omitempty"`
	Rules        []automationRule `json:"rules"`
}

type automationStop struct {
	Until *time.Time `json:"until,omitempty"` // nil = until the operator lifts it
}

type automationQuota struct {
	Used    int       `json:"used"`
	Limit   int       `json:"limit"`
	ResetAt time.Time `json:"resetAt"`
}

type automationRule struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Trigger     string `json:"trigger"` // record | schedule
	Result      string `json:"result"`  // status.Result*
	Enabled     bool   `json:"enabled"` // false: the creator turned it off (serverRules[].enabled)
	// ConsecutiveFailures: runs that gave up in a row (0 after an ok run);
	// iOS offers a fix in the chat when it keeps failing.
	ConsecutiveFailures int             `json:"consecutiveFailures,omitempty"`
	LastRunAt           *time.Time      `json:"lastRunAt,omitempty"`
	LastOKAt            *time.Time      `json:"lastOkAt,omitempty"`
	Stopped             *automationStop `json:"stopped,omitempty"` // the operator stopped this rule
}

func (h *Handler) automations(w http.ResponseWriter, r *http.Request) {
	ctx, uid := r.Context(), uidOf(r)
	appID, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	owner, err := h.appOwner(ctx, appID)
	if err != nil {
		fail(w, err)
		return
	}
	if owner == "" || owner != uid {
		fail(w, apiErr(http.StatusForbidden, httpx.CodeForbidden, "only the creator of this app can see its automations"))
		return
	}
	app, err := h.store.GetApp(ctx, uid, appID)
	if err != nil {
		fail(w, mapStoreErr(err, "app"))
		return
	}
	a, err := h.parsedDSL(uid, app)
	if err != nil {
		// an older stored DSL that today's rules reject still lists its rules
		a = &dsl.App{}
		if json.Unmarshal(app.DSL, a) != nil {
			fail(w, errors.New("appconfig: stored dsl does not decode"))
			return
		}
	}
	out := automationsResponse{AppID: appID, Rules: []automationRule{}}
	if len(a.ServerRules) == 0 {
		httpx.WriteJSON(w, http.StatusOK, out)
		return
	}

	// shared / public runs keep one status per rule; private runs one per user (the caller's)
	statusIDs := make([]string, 0, 2*len(a.ServerRules))
	stopIDs := []string{store.AppStopID(appID), store.NotifyStopID(appID), store.GlobalStopID}
	for i := range a.ServerRules {
		id := a.ServerRules[i].ID
		statusIDs = append(statusIDs, store.RuleStatusID(appID, "", id), store.RuleStatusID(appID, uid, id))
		stopIDs = append(stopIDs, store.RuleStopID(appID, id))
	}
	now := time.Now()
	statuses, err := h.store.GetRuleStatuses(ctx, h.runnerEnv, statusIDs)
	if err != nil {
		fail(w, err)
		return
	}
	stops, err := h.store.ActiveStops(ctx, h.runnerEnv, stopIDs, now)
	if err != nil {
		fail(w, err)
		return
	}
	out.Stopped = stopOf(stops, store.AppStopID(appID))
	if out.Stopped == nil { // #346: every app of the environment
		out.Stopped = stopOf(stops, store.GlobalStopID)
	}
	out.NotifyPaused = stopOf(stops, store.NotifyStopID(appID))

	desc := lang(r) // description language (Accept-Language)
	var latest *store.RuleStatus // the newest run that saw the day's counter
	for i := range a.ServerRules {
		rule := &a.ServerRules[i]
		item := automationRule{ID: rule.ID, Description: a.DescribeServerRuleIn(rule, desc), Trigger: "record", Result: status.ResultNone,
			Enabled: rule.IsEnabled(), Stopped: stopOf(stops, store.RuleStopID(appID, rule.ID))}
		if rule.IsScheduled() {
			item.Trigger = "schedule"
		}
		var st *store.RuleStatus
		var lastOK time.Time
		for _, id := range []string{store.RuleStatusID(appID, "", rule.ID), store.RuleStatusID(appID, uid, rule.ID)} {
			s, ok := statuses[id]
			if !ok {
				continue
			}
			if st == nil || s.At.After(st.At) {
				st = &s
			}
			if s.LastOKAt.After(lastOK) {
				lastOK = s.LastOKAt
			}
		}
		if !lastOK.IsZero() {
			item.LastOKAt = timePtr(lastOK)
		}
		if st != nil {
			item.Result = status.Result(st.Outcome, st.Reason, st.NotifyPaused)
			item.LastRunAt = timePtr(st.At)
			item.ConsecutiveFailures = st.Failures
			if st.QuotaDay != "" && st.QuotaLimit > 0 && (latest == nil || st.At.After(latest.At)) {
				latest = st
			}
		}
		out.Rules = append(out.Rules, item)
	}
	out.Quota = quotaOf(latest, now)
	httpx.WriteJSON(w, http.StatusOK, out)
}

func stopOf(stops map[string]store.StopState, id string) *automationStop {
	s, ok := stops[id]
	if !ok {
		return nil
	}
	out := &automationStop{}
	if !s.Until.IsZero() {
		out.Until = timePtr(s.Until)
	}
	return out
}

// quotaOf is the app's executions today (the runner's quota day, JST) as
// the latest run counted them; a count of an earlier day reads as 0 used.
func quotaOf(s *store.RuleStatus, now time.Time) *automationQuota {
	if s == nil {
		return nil
	}
	today := now.In(ratelimit.JST)
	y, m, d := today.Date()
	q := &automationQuota{Limit: s.QuotaLimit, ResetAt: time.Date(y, m, d+1, 0, 0, 0, 0, ratelimit.JST)}
	if s.QuotaDay == today.Format("2006-01-02") {
		q.Used = min(s.QuotaUsed, s.QuotaLimit)
	}
	return q
}

func timePtr(t time.Time) *time.Time { return &t }
