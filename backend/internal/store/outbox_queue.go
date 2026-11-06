package store

import (
	"context"
	"errors"
	"time"
)

// The runner's side of the outbox (#341). An event is in the queue while it
// has availableAt; claiming it moves availableAt forward by a lease (so
// another runner does not take it) and counts the attempt; finishing it
// removes availableAt and records the result. The document itself stays
// until its expireAt (TTL). Claiming is a range on the single field
// availableAt, so no composite index is needed.

// Outbox states after the runner saw an event (OutboxPending is the first).
const (
	OutboxRetry  = "retry"   // failed; availableAt is the next attempt
	OutboxDone   = "done"    // handled (outcome says how)
	OutboxGaveUp = "gave_up" // failed more times than allowed
)

// ErrLeaseLost: the event was claimed again (its lease ran out) or already
// finished, so this attempt's result is not stored.
var ErrLeaseLost = errors.New("outbox lease lost")

// OutboxResult is how an event ended.
type OutboxResult struct {
	Status  string // OutboxDone | OutboxGaveUp
	Outcome string // ok / skipped_* / error ... (runner outcomes)
	// RuleOutcomes is the outcome of each rule (rule id -> outcome).
	RuleOutcomes map[string]string
	LastError    string // short code, never content
	FinishedAt   time.Time
}

// OutboxQueue is what the runner's queue needs from the store.
type OutboxQueue interface {
	// ClaimOutbox takes up to limit events of env with availableAt <= now,
	// each in its own transaction: availableAt = now + lease, attempts++.
	// An event another runner claimed in between is skipped.
	ClaimOutbox(ctx context.Context, env string, now time.Time, lease time.Duration, limit int) ([]OutboxEvent, error)
	// FinishOutbox removes availableAt and stores res, when the event is
	// still at attempt (else ErrLeaseLost).
	FinishOutbox(ctx context.Context, env, eventID string, attempt int, res OutboxResult) error
	// RetryOutbox sets availableAt = at, status retry and lastError, when
	// the event is still at attempt (else ErrLeaseLost).
	RetryOutbox(ctx context.Context, env, eventID string, attempt int, at time.Time, lastError string) error
}
