package store

import (
	"context"
	"strings"
	"time"
)

// Runner stops (#346). The operator's switches below the environment-wide
// RUNNER_PAUSED / RUNNER_NOTIFY_PAUSED:
//
//	runner/{env}/stops/app_{appId}            the app's server rules do not run
//	runner/{env}/stops/rule_{hash(appId,rule)} one rule does not run
//	runner/{env}/stops/notify_{appId}         the app's rules run, notifications are not sent
//
// They live apart from the app's documents, and appconfig has no way to
// write them (they are not part of Store): a creator cannot lift an
// operator's stop (#337). A lifted stop keeps its document (liftedAt) as the
// record of the operation and as the point after which reports count again
// for the automatic notify pause.

// Stop levels.
const (
	StopApp    = "app"
	StopRule   = "rule"
	StopNotify = "notify"
)

// Who set a stop.
const (
	StopByOperator = "operator"
	StopByAuto     = "auto" // the report threshold (#339)
)

// Stop is runner/{env}/stops/{id}.
type Stop struct {
	ID     string `json:"id"`
	Level  string `json:"level"` // StopApp | StopRule | StopNotify
	AppID  string `json:"appId"`
	RuleID string `json:"ruleId,omitempty"`
	By     string `json:"by"` // StopByOperator | StopByAuto
	// Reason is the operator's note (shown to the creator later, #347).
	Reason string `json:"reason,omitempty"`
	// Until: zero = until lifted.
	Until     time.Time `json:"until,omitzero"`
	CreatedAt time.Time `json:"createdAt"`
	// LiftedAt: zero = not lifted.
	LiftedAt time.Time `json:"liftedAt,omitzero"`
}

// Active reports whether s stops anything at now.
func (s Stop) Active(now time.Time) bool {
	return s.ID != "" && s.LiftedAt.IsZero() && (s.Until.IsZero() || now.Before(s.Until))
}

// Ended is when s stopped being active (zero while active or never set).
func (s Stop) Ended(now time.Time) time.Time {
	switch {
	case s.ID == "" || s.Active(now):
		return time.Time{}
	case !s.LiftedAt.IsZero() && (s.Until.IsZero() || s.LiftedAt.Before(s.Until)):
		return s.LiftedAt
	}
	return s.Until
}

// AppStopID names an app's stop. An appId holds no "/" (appconfig's idRe),
// so it is a valid document id after the prefix.
func AppStopID(appID string) string { return "app_" + appID }

// NotifyStopID names an app's notify pause.
func NotifyStopID(appID string) string { return "notify_" + appID }

// RuleStopID names a rule's stop: a hash of both parts, so no two
// (appId, ruleId) pairs share a document.
func RuleStopID(appID, ruleID string) string { return "rule_" + keyHash(appID, ruleID) }

// ReportedAppID is the appId a report is about, "" when it is not about
// one shared app (a listing or a share link).
func ReportedAppID(r Report) string {
	switch r.Kind {
	case ReportApp:
		return r.TargetID
	case ReportRecord:
		app, _, _ := strings.Cut(r.TargetID, "/")
		return app
	}
	return ""
}

// RunnerStops is the operator's per-app switches and what the automatic
// notify pause reads. Only the runner and cmd/moderate use it.
type RunnerStops interface {
	// GetStops returns the stops among ids that exist (any state), by id.
	GetStops(ctx context.Context, env string, ids []string) (map[string]Stop, error)
	// PutStop creates or replaces s.ID (an operator's stop or its change).
	PutStop(ctx context.Context, env string, s Stop) error
	// PutStopIfInactive writes s unless s.ID is active at now (the automatic
	// pause never replaces an operator's stop or extends a running one).
	PutStopIfInactive(ctx context.Context, env string, s Stop, now time.Time) (put bool, err error)
	// LiftStop sets liftedAt = at on an active stop. lifted=false when
	// there is no active stop with that id.
	LiftStop(ctx context.Context, env, id string, at time.Time) (lifted bool, err error)
	// ListStops returns every stop document of env, by id.
	ListStops(ctx context.Context, env string) ([]Stop, error)
	// ReportsSince returns up to limit reports created at or after since, newest first.
	ReportsSince(ctx context.Context, since time.Time, limit int) ([]Report, error)
}
