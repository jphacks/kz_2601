package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// runnerCollection holds the runner's state per environment (#341):
// runner/{env}/outbox/{eventId}.
const runnerCollection = "runner"

type outboxDoc struct {
	Kind           string         `firestore:"kind"`
	AppID          string         `firestore:"appId"`
	Scope          string         `firestore:"scope"`
	UID            string         `firestore:"uid,omitempty"`
	Entity         string         `firestore:"entity"`
	RecordID       string         `firestore:"recordId"`
	Author         string         `firestore:"author,omitempty"`
	Version        int64          `firestore:"version"`
	RuleIDs        []string       `firestore:"ruleIds"`
	ChangedFields  []string       `firestore:"changedFields,omitempty"`
	Data           map[string]any `firestore:"data,omitempty"`
	Prev           map[string]any `firestore:"prev,omitempty"`
	ActorUID       string         `firestore:"actorUid"`
	ActorAnonymous bool           `firestore:"actorAnonymous"`
	TimeZone       string         `firestore:"timeZone,omitempty"`
	DueAt          time.Time      `firestore:"dueAt,omitempty"`
	ChargeJSON     string         `firestore:"chargeJson,omitempty"`
	AvailableAt    time.Time      `firestore:"availableAt"`
	Attempts       int64          `firestore:"attempts"`
	Status         string         `firestore:"status"`
	CreatedAt      time.Time      `firestore:"createdAt"`
	ExpireAt       time.Time      `firestore:"expireAt"`
	// written by the runner (firestore_runner.go)
	Outcome      string            `firestore:"outcome,omitempty"`
	RuleOutcomes map[string]string `firestore:"ruleOutcomes,omitempty"`
	LastError    string            `firestore:"lastError,omitempty"`
	FinishedAt   time.Time         `firestore:"finishedAt,omitempty"`
}

func toOutboxDoc(ev *OutboxEvent) outboxDoc {
	return outboxDoc{
		Kind: ev.Kind, AppID: ev.AppID, Scope: ev.Scope, UID: ev.UID, Entity: ev.Entity, RecordID: ev.RecordID, Author: ev.Author,
		Version: int64(ev.Version), RuleIDs: ev.RuleIDs, ChangedFields: ev.ChangedFields, Data: ev.Data, Prev: ev.Prev,
		ActorUID: ev.ActorUID, ActorAnonymous: ev.ActorAnonymous, TimeZone: ev.TimeZone, DueAt: ev.DueAt, ChargeJSON: string(ev.Charge),
		AvailableAt: ev.AvailableAt, Attempts: int64(ev.Attempts), Status: ev.Status, CreatedAt: ev.CreatedAt, ExpireAt: ev.ExpireAt,
	}
}

func (d *outboxDoc) toEvent(env, id string) OutboxEvent {
	return OutboxEvent{
		EventID: id, Env: env,
		Kind: d.Kind, AppID: d.AppID, Scope: d.Scope, UID: d.UID, Entity: d.Entity, RecordID: d.RecordID, Author: d.Author,
		Version: int(d.Version), RuleIDs: d.RuleIDs, ChangedFields: d.ChangedFields, Data: d.Data, Prev: d.Prev,
		ActorUID: d.ActorUID, ActorAnonymous: d.ActorAnonymous, TimeZone: d.TimeZone, DueAt: d.DueAt, Charge: rawOrNil(d.ChargeJSON),
		AvailableAt: d.AvailableAt, Attempts: int(d.Attempts), Status: d.Status, CreatedAt: d.CreatedAt, ExpireAt: d.ExpireAt,
		Outcome: d.Outcome, RuleOutcomes: d.RuleOutcomes, LastError: d.LastError, FinishedAt: d.FinishedAt,
	}
}

func rawOrNil(s string) json.RawMessage {
	if s == "" {
		return nil
	}
	return json.RawMessage(s)
}

func (f *Firestore) outboxCol(env string) *firestore.CollectionRef {
	return f.c.Collection(runnerCollection).Doc(env).Collection("outbox")
}

// emitTx runs emit and creates its events in tx (an existing eventId fails
// the whole transaction).
func (f *Firestore) emitTx(tx *firestore.Transaction, emit EmitFunc, prev *Record, next Record, now time.Time) error {
	if emit == nil {
		return nil
	}
	events := emit(prev, next)
	if err := prepareOutbox(events, now); err != nil {
		return err
	}
	for i := range events {
		ev := &events[i]
		if err := tx.Create(f.outboxCol(ev.Env).Doc(ev.EventID), toOutboxDoc(ev)); err != nil {
			return err
		}
	}
	return nil
}

func mapOutboxErr(err error) error {
	if status.Code(err) == codes.AlreadyExists {
		return ErrAlreadyExists
	}
	return err
}

// CreateRecordEmit implements Outbox.
func (f *Firestore) CreateRecordEmit(ctx context.Context, rs RecordSet, rec Record, emit EmitFunc) (*Record, bool, error) {
	ref := f.recRef(rs, rec.ID)
	var out *Record
	created := false
	err := f.c.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		created = false
		snap, err := tx.Get(ref)
		if err == nil {
			var d recordDoc
			if err := snap.DataTo(&d); err != nil {
				return err
			}
			out = d.toRecord(rec.ID)
			return nil
		}
		if !isNotFound(err) {
			return err
		}
		now := Now()
		d := recordDoc{Entity: rs.Entity, Data: rec.Data, ClientID: rec.ClientID, Author: rec.Author, AuthorName: rec.AuthorName, Readers: rec.Readers, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(ref, d); err != nil {
			return err
		}
		out = d.toRecord(rec.ID)
		if err := f.emitTx(tx, emit, nil, *out, now); err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return nil, false, mapOutboxErr(err)
	}
	return out, created, nil
}

// UpdateRecordEmit implements Outbox. Same cost as UpdateRecord (one read,
// one write) plus the events.
func (f *Firestore) UpdateRecordEmit(ctx context.Context, rs RecordSet, id string, data map[string]any, emit EmitFunc) (*Record, error) {
	ref := f.recRef(rs, id)
	var out *Record
	err := f.c.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
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
		prev := d.toRecord(id)
		now := Now()
		next := *prev
		next.Data, next.UpdatedAt = data, now
		if next.Data == nil {
			next.Data = map[string]any{}
		}
		if err := tx.Update(ref, []firestore.Update{
			{Path: "data", Value: data},
			{Path: "updatedAt", Value: now},
		}); err != nil {
			return err
		}
		if err := f.emitTx(tx, emit, prev, next, now); err != nil {
			return err
		}
		out = &next
		return nil
	})
	if err != nil {
		return nil, mapOutboxErr(err)
	}
	return out, nil
}

// OutboxEvents returns env's queued events (oldest first, up to limit; 0 = all).
func (f *Firestore) OutboxEvents(ctx context.Context, env string, limit int) ([]OutboxEvent, error) {
	q := f.outboxCol(env).OrderBy("createdAt", firestore.Asc)
	if limit > 0 {
		q = q.Limit(limit)
	}
	it := q.Documents(ctx)
	defer it.Stop()
	var out []OutboxEvent
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		var d outboxDoc
		if err := snap.DataTo(&d); err != nil {
			return nil, err
		}
		out = append(out, d.toEvent(env, snap.Ref.ID))
	}
}
