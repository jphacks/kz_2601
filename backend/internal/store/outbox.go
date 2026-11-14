package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// Outbox (#341): appconfig queues "a record was created / updated" for the
// runner in the same transaction as the record write, so a write and its
// trigger are stored together or not at all.
//
//	runner/{env}/outbox/{eventId}
//
// {env} is APP_ENV ("local" when unset): stg and prod sharing one Firestore
// keep apart queues.

// Outbox event kinds and states.
const (
	OutboxRecordCreated = "record_created"
	OutboxRecordUpdated = "record_updated"
	// OutboxRulesChanged: a DSL with (or that had) time-triggered rules was
	// saved; the runner rebuilds the app's schedules (#342, schedule.go).
	OutboxRulesChanged = "rules_changed"
	// OutboxSchedule: one due run of a schedule (#342). Its id is the
	// schedule key + the due time, so it exists once per run.
	OutboxSchedule = "schedule"

	OutboxPending = "pending"

	// OutboxTTL is how long an event is kept (expireAt, Firestore TTL policy).
	OutboxTTL = 30 * 24 * time.Hour
)

// OutboxEvent is one queued trigger.
type OutboxEvent struct {
	EventID string `json:"eventId"` // "" = assigned by the store
	Env     string `json:"env"`     // path segment runner/{env}/outbox

	Kind     string `json:"kind"` // record_created | record_updated | rules_changed | schedule
	AppID    string `json:"appId"`
	Scope    string `json:"scope"`         // private | shared | public
	UID      string `json:"uid,omitempty"` // private: whose records
	Entity   string `json:"entity"`
	RecordID string `json:"recordId"`
	Author   string `json:"author,omitempty"` // shared/public: the record's poster

	// Version of the DSL the rules come from at trigger time: the creator's
	// published version (shared/public) or the user's own (private).
	Version       int            `json:"version"`
	RuleIDs       []string       `json:"ruleIds"`
	ChangedFields []string       `json:"changedFields,omitempty"`
	Data          map[string]any `json:"data,omitempty"` // only the fields the rules read
	Prev          map[string]any `json:"prev,omitempty"` // same fields before the update

	ActorUID       string `json:"actorUid"`
	ActorAnonymous bool   `json:"actorAnonymous"`

	// TimeZone is the creator's IANA zone ("" = the runner's default):
	// what "today" / "now" and the schedules are read in (#342).
	TimeZone string `json:"timeZone,omitempty"`
	// DueAt: schedule events only, the run time the schedule was due.
	DueAt time.Time `json:"dueAt,omitzero"`
	// Charge is what an attempt counted (quota.Charge as JSON), kept so a
	// later attempt that gives up can refund it (#342). Never content.
	Charge json.RawMessage `json:"charge,omitempty"`

	// AvailableAt: when the event may be claimed; zero once it is finished
	// (the runner removes the field, outbox_queue.go).
	AvailableAt time.Time `json:"availableAt"`
	Attempts    int       `json:"attempts"`
	// Deferrals: claims put back without counting as a failure (DeferOutbox);
	// Attempts - Deferrals is what counts toward giving up.
	Deferrals int       `json:"deferrals,omitempty"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	ExpireAt  time.Time `json:"expireAt"`

	// Set by the runner (outbox_queue.go).
	Outcome      string            `json:"outcome,omitempty"`
	RuleOutcomes map[string]string `json:"ruleOutcomes,omitempty"`
	LastError    string            `json:"lastError,omitempty"`
	FinishedAt   time.Time         `json:"finishedAt,omitzero"`
}

// EmitFunc builds the events of one record write inside its transaction:
// prev is the record before (nil on create), next the record as written.
// It must have no side effects (a Firestore transaction may run it again).
type EmitFunc func(prev *Record, next Record) []OutboxEvent

// Outbox writes records together with their outbox events. The plain
// CreateRecord / UpdateRecord are unchanged and queue nothing.
type Outbox interface {
	// CreateRecordEmit is CreateRecord that also stores emit's events when
	// the record is created (an existing id returns it and emits nothing).
	CreateRecordEmit(ctx context.Context, rs RecordSet, rec Record, emit EmitFunc) (out *Record, created bool, err error)
	// UpdateRecordEmit is UpdateRecord that reads the record before the
	// update in the same transaction and stores emit's events with it.
	UpdateRecordEmit(ctx context.Context, rs RecordSet, id string, data map[string]any, emit EmitFunc) (*Record, error)
}

// ErrOutboxEvent is a malformed event; the write it came with is not stored.
var ErrOutboxEvent = errors.New("invalid outbox event")

var outboxEnvRe = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// prepareOutbox fills the store-owned fields of events and validates them.
func prepareOutbox(events []OutboxEvent, now time.Time) error {
	for i := range events {
		ev := &events[i]
		if ev.EventID == "" {
			ev.EventID = NewID("evt_")
		}
		if !outboxEnvRe.MatchString(ev.Env) {
			return fmt.Errorf("%w: env %q", ErrOutboxEvent, ev.Env)
		}
		if ev.AppID == "" {
			return fmt.Errorf("%w: appId is required", ErrOutboxEvent)
		}
		switch ev.Kind {
		case OutboxRecordCreated, OutboxRecordUpdated:
			if ev.Entity == "" || ev.RecordID == "" || len(ev.RuleIDs) == 0 {
				return fmt.Errorf("%w: entity / recordId / ruleIds are required", ErrOutboxEvent)
			}
		case OutboxRulesChanged:
			if ev.UID == "" || ev.Version <= 0 {
				return fmt.Errorf("%w: rules_changed needs uid and version", ErrOutboxEvent)
			}
		case OutboxSchedule:
			if len(ev.RuleIDs) != 1 || ev.DueAt.IsZero() {
				return fmt.Errorf("%w: schedule needs one rule and dueAt", ErrOutboxEvent)
			}
		default:
			return fmt.Errorf("%w: kind %q", ErrOutboxEvent, ev.Kind)
		}
		if ev.Status == "" {
			ev.Status = OutboxPending
		}
		if ev.CreatedAt.IsZero() {
			ev.CreatedAt = now
		}
		if ev.AvailableAt.IsZero() {
			ev.AvailableAt = ev.CreatedAt
		}
		if ev.ExpireAt.IsZero() {
			ev.ExpireAt = ev.CreatedAt.Add(OutboxTTL)
		}
	}
	return nil
}
