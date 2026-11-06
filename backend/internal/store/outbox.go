package store

import (
	"context"
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

	OutboxPending = "pending"

	// OutboxTTL is how long an event is kept (expireAt, Firestore TTL policy).
	OutboxTTL = 30 * 24 * time.Hour
)

// OutboxEvent is one queued trigger.
type OutboxEvent struct {
	EventID string `json:"eventId"` // "" = assigned by the store
	Env     string `json:"env"`     // path segment runner/{env}/outbox

	Kind     string `json:"kind"` // record_created | record_updated
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

	AvailableAt time.Time `json:"availableAt"`
	Attempts    int       `json:"attempts"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	ExpireAt    time.Time `json:"expireAt"`
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
		switch {
		case !outboxEnvRe.MatchString(ev.Env):
			return fmt.Errorf("%w: env %q", ErrOutboxEvent, ev.Env)
		case ev.Kind != OutboxRecordCreated && ev.Kind != OutboxRecordUpdated:
			return fmt.Errorf("%w: kind %q", ErrOutboxEvent, ev.Kind)
		case ev.AppID == "" || ev.Entity == "" || ev.RecordID == "" || len(ev.RuleIDs) == 0:
			return fmt.Errorf("%w: appId / entity / recordId / ruleIds are required", ErrOutboxEvent)
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
