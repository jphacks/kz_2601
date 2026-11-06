package store

import (
	"context"
	"time"
)

// Schedules (#342). Time-triggered rules are not one Cloud Scheduler job
// each: the runner keeps one document per rule and, on every tick, takes
// the ones that are due (a range on the single field nextRunAt).
//
//	runner/{env}/schedules/{appId}_{ruleId}         shared / public apps
//	runner/{env}/schedules/{appId}_{uid}_{ruleId}   private apps (one per user)
//	runner/{env}/apps/{appId}[_{uid}]               lastOpenedAt, timeZone
//
// appconfig writes apps/*.lastOpenedAt when the app is opened (at most once
// a day); the runner writes the rest.

// Schedule states.
const (
	ScheduleActive = "active"
	// ScheduleInvalid: its timing cannot be computed; it is out of the range
	// query (no nextRunAt) until the next rules_changed rewrites it.
	ScheduleInvalid = "invalid"
)

// Schedule is one time-triggered rule of one app (and, private, one user).
type Schedule struct {
	ID     string `json:"id"` // ScheduleKey(AppID, UID, RuleID)
	AppID  string `json:"appId"`
	UID    string `json:"uid,omitempty"` // private apps only
	Scope  string `json:"scope"`         // private | shared | public
	RuleID string `json:"ruleId"`

	Every   string `json:"every"` // daily | weekly | monthly
	Time    string `json:"time"`  // HH:MM in TimeZone
	Weekday string `json:"weekday,omitempty"`
	Day     int    `json:"day,omitempty"`
	// TimeZone is the creator's IANA zone when the DSL was saved.
	TimeZone string `json:"timeZone"`

	// Version of the DSL the schedule was built from (and the rule runs with).
	Version int `json:"version"`
	// CreatorAnonymous: the creator was signed in anonymously when saving.
	CreatorAnonymous bool   `json:"creatorAnonymous,omitempty"`
	Status           string `json:"status"`

	NextRunAt time.Time `json:"nextRunAt"` // UTC, jitter included
	LastDueAt time.Time `json:"lastDueAt,omitzero"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// SameTiming reports whether s and o run at the same times.
func (s Schedule) SameTiming(o Schedule) bool {
	return s.Every == o.Every && s.Time == o.Time && s.Weekday == o.Weekday && s.Day == o.Day && s.TimeZone == o.TimeZone
}

// TenantKey names an app's runner state: the appId, plus the uid for a
// private app (each user has their own copy and their own schedules).
func TenantKey(appID, uid string) string {
	if uid == "" {
		return runnerMarkID(appID)
	}
	return runnerMarkID(appID + "_" + uid)
}

// ScheduleKey is the document id of a schedule.
func ScheduleKey(appID, uid, ruleID string) string {
	return TenantKey(appID, uid) + "_" + runnerMarkID(ruleID)
}

// RunnerApp is runner/{env}/apps/{TenantKey}.
type RunnerApp struct {
	// LastOpenedAt: the last GET /dsl that wrote it (zero = never).
	LastOpenedAt time.Time
	// TimeZone of the creator as of TimeZoneVersion ("" = unknown).
	TimeZone        string
	TimeZoneVersion int
}

// ScheduleAdvance is called on a due schedule inside the claim transaction.
// next is its new nextRunAt (zero = it cannot be scheduled: status invalid);
// ev is the run to queue (nil = none). It must have no side effects.
type ScheduleAdvance func(s Schedule) (next time.Time, ev *OutboxEvent)

// Schedules is the runner's schedule table and per-app state.
type Schedules interface {
	// PutSchedule creates or replaces s.ID in one transaction. A stored
	// schedule of a newer version is kept (put=false). When the stored one
	// has the same timing, its nextRunAt, lastDueAt and createdAt are kept.
	PutSchedule(ctx context.Context, env string, s Schedule) (put bool, err error)
	// DeleteSchedule deletes id when its version is at most maxVersion.
	DeleteSchedule(ctx context.Context, env, id string, maxVersion int) (deleted bool, err error)
	// ListSchedules returns the schedules of appID (every user's, for private apps).
	ListSchedules(ctx context.Context, env, appID string) ([]Schedule, error)
	// ClaimSchedules takes up to limit active schedules with nextRunAt <= now,
	// each in its own transaction: re-checks it is due, moves nextRunAt to
	// advance's next and creates advance's event unless an event with that
	// id exists. It returns the events it created.
	ClaimSchedules(ctx context.Context, env string, now time.Time, limit int, advance ScheduleAdvance) ([]OutboxEvent, error)

	// TouchAppOpened sets lastOpenedAt = at (appconfig, GET /dsl).
	TouchAppOpened(ctx context.Context, env, key string, at time.Time) error
	// SetAppTimeZone stores the creator's zone unless a newer version did.
	SetAppTimeZone(ctx context.Context, env, key, tz string, version int) error
	// RunnerApp reads apps/{key}; a missing document is the zero value.
	RunnerApp(ctx context.Context, env, key string) (RunnerApp, error)
}
