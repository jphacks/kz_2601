package store

import (
	"context"
	"time"
)

// Rule status (#347): the latest result of each server rule, for its creator.
// The runner overwrites one small document per rule (and, private, per user)
// at the end of every run, so the creator's screen never reads the outbox:
//
//	runner/{env}/rule_status/{RuleStatusID(appId, uid, ruleId)}   uid "" for shared / public
//
// It holds ids, outcome codes, times and the app's execution count of the
// day only: no record values, no notification text, no uid (decision 337
// 「実行の履歴」). expireAt is the Firestore TTL field.

// RuleStatusTTL: a rule that has not run for this long has no status.
const RuleStatusTTL = 30 * 24 * time.Hour

// RuleStatus is runner/{env}/rule_status/{id}.
type RuleStatus struct {
	ID     string `json:"id"`
	AppID  string `json:"appId"`
	RuleID string `json:"ruleId"`
	// At is when the rule last finished a run (whatever the outcome).
	At time.Time `json:"at"`
	// Outcome is the runner's outcome (quota.Outcome* / runner.Outcome* / gave_up).
	Outcome string `json:"outcome"`
	// Reason of a skipped_stopped outcome (runner.Reason*).
	Reason string `json:"reason,omitempty"`
	// NotifyPaused: the rule ran but its notifications were not sent.
	NotifyPaused bool `json:"notifyPaused,omitempty"`
	// LastOKAt: the last run that ended ok. A put with a zero LastOKAt keeps the stored one.
	LastOKAt time.Time `json:"lastOkAt,omitzero"`
	// QuotaDay (yyyy-mm-dd of the quota's day), QuotaUsed / QuotaLimit: the
	// app's execution counter as this run saw it. A put with an empty
	// QuotaDay keeps the stored values.
	QuotaDay   string    `json:"quotaDay,omitempty"`
	QuotaUsed  int       `json:"quotaUsed,omitempty"`
	QuotaLimit int       `json:"quotaLimit,omitempty"`
	ExpireAt   time.Time `json:"expireAt"`
	// Failures: the runs that gave up in a row (read). A put with Failed adds
	// one; a put with LastOKAt resets it to 0; any other put keeps it.
	Failures int  `json:"failures,omitempty"`
	Failed   bool `json:"-"`
	// FailureReason: why a gave_up run failed (status.Failure*, a fixed word,
	// never the error). Empty for any other outcome: each put overwrites it.
	FailureReason string `json:"failureReason,omitempty"`
}

// RuleStatusID names a rule's status: a hash of the parts (like ScheduleKey).
func RuleStatusID(appID, uid, ruleID string) string { return "r_" + keyHash(appID, uid, ruleID) }

// RuleStatuses is what the runner writes.
type RuleStatuses interface {
	// PutRuleStatus overwrites s.ID, keeping the stored LastOKAt / Quota*
	// when s leaves them empty.
	PutRuleStatus(ctx context.Context, env string, s RuleStatus) error
}

// StopState is all the creator may know of an operator's stop: that it is
// in effect, at which level and until when (zero = until lifted). Who set it,
// why and how many reports led to it stay with the operator (#337 / #339).
type StopState struct {
	Level string    `json:"level"`
	Until time.Time `json:"until,omitzero"`
	// Force: the rule-triggering writes are refused too (Stop.Force).
	Force bool `json:"force,omitempty"`
}

// RunnerActivity is the read-only view of the runner's state the creator's
// screen needs (appconfig GET /v1/apps/{appId}/automations). It can neither
// write nor lift a stop, nor read a stop's reason (#337).
type RunnerActivity interface {
	// ActiveStops returns the stops among ids that are active at now, by id.
	ActiveStops(ctx context.Context, env string, ids []string, now time.Time) (map[string]StopState, error)
	// GetRuleStatuses returns the statuses among ids that exist and have not
	// expired at the store's clock, by id.
	GetRuleStatuses(ctx context.Context, env string, ids []string) (map[string]RuleStatus, error)
}

// activeStops keeps the stops of got active at now, without their details.
func activeStops(got map[string]Stop, now time.Time) map[string]StopState {
	out := map[string]StopState{}
	for id, s := range got {
		if s.Active(now) {
			out[id] = StopState{Level: s.Level, Until: s.Until, Force: s.Force}
		}
	}
	return out
}
