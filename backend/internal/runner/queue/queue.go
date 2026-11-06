// Package queue takes runner work from the outbox (#341): claim with a lease,
// finish with an outcome, retry with a growing delay, give up after
// MaxAttempts. The storage is store.OutboxQueue (Memory / Firestore); the
// policy lives here.
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// Defaults.
const (
	DefaultLease       = 2 * time.Minute
	DefaultMaxAttempts = 3
	DefaultBaseDelay   = 30 * time.Second
	DefaultBatch       = 20
)

// OutcomeGaveUp is the outcome of an event that failed more than
// MaxAttempts times.
const OutcomeGaveUp = "gave_up"

// Queue is the outbox of one environment.
type Queue struct {
	st  store.OutboxQueue
	env string

	// Lease is how long a claimed event stays with this runner. A runner
	// that dies mid-run lets it come back after the lease (as a new attempt).
	Lease time.Duration
	// MaxAttempts: an event claimed for the (MaxAttempts+1)th time is given up.
	MaxAttempts int
	// BaseDelay is the first retry delay; each next one is 4 times longer
	// (30s, 2m, 8m).
	BaseDelay time.Duration
	Now       func() time.Time
}

// New returns the queue runner/{env}/outbox.
func New(st store.OutboxQueue, env string) *Queue {
	return &Queue{st: st, env: env, Lease: DefaultLease, MaxAttempts: DefaultMaxAttempts, BaseDelay: DefaultBaseDelay, Now: time.Now}
}

// Env is the environment the queue serves.
func (q *Queue) Env() string { return q.env }

// Claim leases up to limit due events.
func (q *Queue) Claim(ctx context.Context, limit int) ([]store.OutboxEvent, error) {
	return q.st.ClaimOutbox(ctx, q.env, q.Now(), q.Lease, limit)
}

// Exhausted reports whether ev (as claimed) is past MaxAttempts.
func (q *Queue) Exhausted(ev store.OutboxEvent) bool { return ev.Attempts > q.MaxAttempts }

// Done finishes ev with outcome. ErrLeaseLost (another runner took it) is
// not an error here: that runner records its own result.
func (q *Queue) Done(ctx context.Context, ev store.OutboxEvent, outcome string, rules map[string]string) error {
	return ignoreLost(q.st.FinishOutbox(ctx, q.env, ev.EventID, ev.Attempts, store.OutboxResult{
		Status: store.OutboxDone, Outcome: outcome, RuleOutcomes: rules, FinishedAt: q.Now(),
	}))
}

// GiveUp finishes ev as gave_up.
func (q *Queue) GiveUp(ctx context.Context, ev store.OutboxEvent, lastError string) error {
	return ignoreLost(q.st.FinishOutbox(ctx, q.env, ev.EventID, ev.Attempts, store.OutboxResult{
		Status: store.OutboxGaveUp, Outcome: OutcomeGaveUp, LastError: orDefault(lastError, ev.LastError), FinishedAt: q.Now(),
	}))
}

// Retry puts ev back after Delay(ev.Attempts). code is a short error class
// (never content); charge (nil = keep the stored one) is what this attempt
// counted, refunded if the event gives up.
func (q *Queue) Retry(ctx context.Context, ev store.OutboxEvent, code string, charge json.RawMessage) error {
	return ignoreLost(q.st.RetryOutbox(ctx, q.env, ev.EventID, ev.Attempts, q.Now().Add(q.Delay(ev.Attempts)), code, charge))
}

// Delay is the wait after the attempt-th failure.
func (q *Queue) Delay(attempt int) time.Duration {
	d := q.BaseDelay
	for i := 1; i < attempt; i++ {
		d *= 4
	}
	return d
}

func ignoreLost(err error) error {
	if errors.Is(err, store.ErrLeaseLost) {
		return nil
	}
	return err
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
