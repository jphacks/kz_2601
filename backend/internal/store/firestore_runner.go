package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

var (
	_ OutboxQueue  = (*Firestore)(nil)
	_ RunnerWrites = (*Firestore)(nil)
)

// originDoc is Record.Origin in Firestore.
type originDoc struct {
	RuleID   string `firestore:"ruleId"`
	Version  int64  `firestore:"version"`
	EventID  string `firestore:"eventId"`
	ActorUID string `firestore:"actorUid,omitempty"`
}

func toOriginDoc(o *RecordOrigin) *originDoc {
	if o == nil {
		return nil
	}
	return &originDoc{RuleID: o.RuleID, Version: int64(o.Version), EventID: o.EventID, ActorUID: o.ActorUID}
}

func (d *originDoc) toOrigin() *RecordOrigin {
	if d == nil {
		return nil
	}
	return &RecordOrigin{RuleID: d.RuleID, Version: int(d.Version), EventID: d.EventID, ActorUID: d.ActorUID}
}

// ClaimOutbox implements OutboxQueue: one single-field range query, then one
// transaction per event that re-checks availableAt.
func (f *Firestore) ClaimOutbox(ctx context.Context, env string, now time.Time, lease time.Duration, limit int) ([]OutboxEvent, error) {
	q := f.outboxCol(env).Where("availableAt", "<=", now).OrderBy("availableAt", firestore.Asc)
	if limit > 0 {
		q = q.Limit(limit)
	}
	it := q.Documents(ctx)
	var refs []*firestore.DocumentRef
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			it.Stop()
			return nil, err
		}
		refs = append(refs, snap.Ref)
	}
	it.Stop()
	until := now.Add(lease)
	var out []OutboxEvent
	for _, ref := range refs {
		var got *OutboxEvent
		err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
			got = nil
			snap, err := tx.Get(ref)
			if err != nil {
				return err
			}
			var d outboxDoc
			if err := snap.DataTo(&d); err != nil {
				return err
			}
			if _, err := snap.DataAt("availableAt"); err != nil || d.AvailableAt.After(now) {
				return nil // finished or claimed by someone else in between
			}
			d.AvailableAt, d.Attempts = until, d.Attempts+1
			if err := tx.Update(ref, []firestore.Update{
				{Path: "availableAt", Value: until},
				{Path: "attempts", Value: firestore.Increment(1)},
			}); err != nil {
				return err
			}
			ev := d.toEvent(env, ref.ID)
			got = &ev
			return nil
		})
		if isNotFound(err) {
			continue
		}
		if err != nil {
			return out, err
		}
		if got != nil {
			out = append(out, *got)
		}
	}
	return out, nil
}

// leased runs fn on the event when it is still at attempt.
func (f *Firestore) leased(ctx context.Context, env, eventID string, attempt int, updates func() []firestore.Update) error {
	ref := f.outboxCol(env).Doc(eventID)
	return f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if isNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		att, _ := snap.DataAt("attempts")
		n, _ := att.(int64)
		if _, err := snap.DataAt("availableAt"); err != nil || int(n) != attempt {
			return ErrLeaseLost
		}
		return tx.Update(ref, updates())
	})
}

// FinishOutbox implements OutboxQueue.
func (f *Firestore) FinishOutbox(ctx context.Context, env, eventID string, attempt int, res OutboxResult) error {
	return f.leased(ctx, env, eventID, attempt, func() []firestore.Update {
		u := []firestore.Update{
			{Path: "availableAt", Value: firestore.Delete},
			{Path: "status", Value: res.Status},
			{Path: "outcome", Value: res.Outcome},
			{Path: "finishedAt", Value: res.FinishedAt},
		}
		if len(res.RuleOutcomes) > 0 {
			u = append(u, firestore.Update{Path: "ruleOutcomes", Value: res.RuleOutcomes})
		}
		if res.LastError != "" {
			u = append(u, firestore.Update{Path: "lastError", Value: res.LastError})
		}
		return u
	})
}

// RetryOutbox implements OutboxQueue.
func (f *Firestore) RetryOutbox(ctx context.Context, env, eventID string, attempt int, at time.Time, lastError string, charge json.RawMessage) error {
	return f.leased(ctx, env, eventID, attempt, func() []firestore.Update {
		u := []firestore.Update{
			{Path: "availableAt", Value: at},
			{Path: "status", Value: OutboxRetry},
			{Path: "lastError", Value: lastError},
		}
		if charge != nil {
			u = append(u, firestore.Update{Path: "chargeJson", Value: string(charge)})
		}
		return u
	})
}

// DeferOutbox implements OutboxQueue.
func (f *Firestore) DeferOutbox(ctx context.Context, env, eventID string, attempt int, at time.Time, lastError string, charge json.RawMessage) error {
	return f.leased(ctx, env, eventID, attempt, func() []firestore.Update {
		u := []firestore.Update{
			{Path: "availableAt", Value: at},
			{Path: "status", Value: OutboxRetry},
			{Path: "lastError", Value: lastError},
			{Path: "deferrals", Value: firestore.Increment(1)},
		}
		if charge != nil {
			u = append(u, firestore.Update{Path: "chargeJson", Value: string(charge)})
		}
		return u
	})
}

func (f *Firestore) runnerMarkRef(env, key string) *firestore.DocumentRef {
	return f.c.Collection(runnerCollection).Doc(env).Collection("done").Doc(runnerMarkID(key))
}

type runnerMarkDoc struct {
	CreatedAt time.Time `firestore:"createdAt"`
	ExpireAt  time.Time `firestore:"expireAt"`
}

func newRunnerMark() runnerMarkDoc {
	now := Now()
	return runnerMarkDoc{CreatedAt: now, ExpireAt: now.Add(RunnerMarkTTL)}
}

// RunnerMarked implements RunnerWrites.
func (f *Firestore) RunnerMarked(ctx context.Context, env, key string) (bool, error) {
	_, err := f.runnerMarkRef(env, key).Get(ctx)
	if isNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

// MarkRunnerDone implements RunnerWrites.
func (f *Firestore) MarkRunnerDone(ctx context.Context, env, key string) error {
	_, err := f.runnerMarkRef(env, key).Set(ctx, newRunnerMark())
	return err
}

// ApplyRecordOnce implements RunnerWrites.
func (f *Firestore) ApplyRecordOnce(ctx context.Context, rs RecordSet, id, env, key string, fn func(map[string]any) map[string]any) (bool, error) {
	ref, mark := f.recRef(rs, id), f.runnerMarkRef(env, key)
	applied := false
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		applied = false
		if _, err := tx.Get(mark); err == nil {
			return nil
		} else if !isNotFound(err) {
			return err
		}
		snap, err := tx.Get(ref)
		if isNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		var d recordDoc
		if err := snap.DataTo(&d); err != nil {
			return err
		}
		data := fn(d.toRecord(id).Data)
		if data == nil {
			data = map[string]any{}
		}
		if err := tx.Update(ref, []firestore.Update{{Path: "data", Value: data}, {Path: "updatedAt", Value: Now()}}); err != nil {
			return err
		}
		if err := tx.Create(mark, newRunnerMark()); err != nil {
			return err
		}
		applied = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return applied, nil
}
