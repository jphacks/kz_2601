package store

import (
	"context"
	"strings"
	"time"
)

// Runner writes (#341). The runner writes records directly (never through
// the Emit methods, so its writes queue no trigger: no chains) and keeps one
// mark per action it carried out:
//
//	runner/{env}/done/{key}   {createdAt, expireAt}
//
// A mark is what makes an update / increment run once even when the event
// is retried: ApplyRecordOnce checks the mark, writes the record and sets the
// mark in one transaction.

// RunnerMarkTTL is how long a done mark is kept (expireAt, Firestore TTL
// policy). Longer than any retry of an event (the runner drops events older
// than an hour).
const RunnerMarkTTL = 7 * 24 * time.Hour

// RecordOrigin says which rule run wrote a record (#337 layer 5).
type RecordOrigin struct {
	RuleID   string `json:"ruleId"`
	Version  int    `json:"version"`
	EventID  string `json:"eventId"`
	ActorUID string `json:"actorUid,omitempty"` // whose write triggered it (account erasure, #458)
}

// RunnerWrites are the runner's idempotent writes.
type RunnerWrites interface {
	// RunnerMarked reports whether runner/{env}/done/{key} exists.
	RunnerMarked(ctx context.Context, env, key string) (bool, error)
	// MarkRunnerDone creates the mark (an existing mark is left as is).
	MarkRunnerDone(ctx context.Context, env, key string) error
	// ApplyRecordOnce, in one transaction: when the mark is absent, reads
	// the record, writes fn(its data) and creates the mark. applied=false
	// when the mark was already there (nothing written). ErrNotFound when
	// the record does not exist (no mark is set). fn must have no side
	// effects (a Firestore transaction may run it again).
	ApplyRecordOnce(ctx context.Context, rs RecordSet, id, env, key string, fn func(data map[string]any) map[string]any) (applied bool, err error)
}

// runnerMarkID keeps a key a legal document id.
func runnerMarkID(key string) string { return strings.ReplaceAll(key, "/", "_") }
