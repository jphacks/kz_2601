package store

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
)

// uniqueDoc is entities/{e}/unique/{key}: the record that holds the values.
type uniqueDoc struct {
	RecordID  string    `firestore:"recordId"`
	CreatedAt time.Time `firestore:"createdAt"`
}

// uniqueCol is entities/{e}/unique next to the records collection.
func (f *Firestore) uniqueCol(rs RecordSet) *firestore.CollectionRef {
	return f.recordsCol(rs).Parent.Collection("unique")
}

// checkClaim fails with ErrDuplicate when a record other than id holds the
// values of c. Reads only (before any write of tx).
func (f *Firestore) checkClaim(tx *firestore.Transaction, rs RecordSet, id string, c UniqueClaim) error {
	snap, err := tx.Get(f.uniqueCol(rs).Doc(c.Key))
	switch {
	case err == nil:
		var d uniqueDoc
		if err := snap.DataTo(&d); err != nil {
			return err
		}
		if d.RecordID != id {
			held, err := f.holds(tx, rs, d.RecordID, c.Key)
			if err != nil {
				return err
			}
			if held {
				return ErrDuplicate
			}
		}
	case !isNotFound(err):
		return err
	}
	// records without a claim (written before unique, or by a server rule)
	if len(c.Match) == 0 {
		return nil
	}
	q := f.recordsCol(rs).Query
	for k, v := range c.Match {
		path := firestore.FieldPath{"data", k}
		if k == UniqueAuthorKey {
			path = firestore.FieldPath{"author"}
		}
		q = q.WherePath(path, "==", v)
	}
	docs, err := tx.Documents(q.Limit(2)).GetAll()
	if err != nil {
		return err
	}
	for _, s := range docs {
		if s.Ref.ID != id {
			return ErrDuplicate
		}
	}
	return nil
}

// holds reports whether record id still exists and claims key (else the lock is stale).
func (f *Firestore) holds(tx *firestore.Transaction, rs RecordSet, id, key string) (bool, error) {
	snap, err := tx.Get(f.recRef(rs, id))
	if isNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var d recordDoc
	if err := snap.DataTo(&d); err != nil {
		return false, err
	}
	return d.UniqueKey == key, nil
}

// CreateRecordUnique implements UniqueRecords.
func (f *Firestore) CreateRecordUnique(ctx context.Context, rs RecordSet, rec Record, claim UniqueClaim, emit EmitFunc) (*Record, bool, error) {
	ref := f.recRef(rs, rec.ID)
	var out *Record
	created := false
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
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
		if claim.Key != "" {
			if err := f.checkClaim(tx, rs, rec.ID, claim); err != nil {
				return err
			}
		}
		now := Now()
		d := recordDoc{Entity: rs.Entity, Data: rec.Data, ClientID: rec.ClientID, Author: rec.Author, AuthorName: rec.AuthorName, Readers: rec.Readers, OwnerAgeConfirmed: rec.OwnerAgeConfirmed, Origin: toOriginDoc(rec.Origin), UniqueKey: claim.Key, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(ref, d); err != nil {
			return err
		}
		if claim.Key != "" {
			if err := tx.Set(f.uniqueCol(rs).Doc(claim.Key), uniqueDoc{RecordID: rec.ID, CreatedAt: now}); err != nil {
				return err
			}
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

// UpdateRecordUnique implements UniqueRecords.
func (f *Firestore) UpdateRecordUnique(ctx context.Context, rs RecordSet, id string, data map[string]any, claim UniqueClaim, emit EmitFunc) (*Record, error) {
	ref := f.recRef(rs, id)
	var out *Record
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
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
		moved := claim.Key != prev.UniqueKey
		releaseOld := false
		if moved && prev.UniqueKey != "" {
			lock, err := tx.Get(f.uniqueCol(rs).Doc(prev.UniqueKey))
			if err != nil && !isNotFound(err) {
				return err
			}
			if err == nil {
				var u uniqueDoc
				if err := lock.DataTo(&u); err != nil {
					return err
				}
				releaseOld = u.RecordID == id
			}
		}
		if moved && claim.Key != "" {
			if err := f.checkClaim(tx, rs, id, claim); err != nil {
				return err
			}
		}
		now := Now()
		next := *prev
		next.Data, next.UpdatedAt, next.UniqueKey = data, now, claim.Key
		if next.Data == nil {
			next.Data = map[string]any{}
		}
		updates := []firestore.Update{
			{Path: "data", Value: data},
			{Path: "updatedAt", Value: now},
		}
		if moved {
			if claim.Key == "" {
				updates = append(updates, firestore.Update{Path: "uniqueKey", Value: firestore.Delete})
			} else {
				updates = append(updates, firestore.Update{Path: "uniqueKey", Value: claim.Key})
			}
		}
		if err := tx.Update(ref, updates); err != nil {
			return err
		}
		if releaseOld {
			if err := tx.Delete(f.uniqueCol(rs).Doc(prev.UniqueKey)); err != nil {
				return err
			}
		}
		if moved && claim.Key != "" {
			if err := tx.Set(f.uniqueCol(rs).Doc(claim.Key), uniqueDoc{RecordID: id, CreatedAt: now}); err != nil {
				return err
			}
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

// deleteRecordAndClaim deletes record id with the lock it holds (#273), in
// one transaction.
func (f *Firestore) deleteRecordAndClaim(ctx context.Context, rs RecordSet, id string) error {
	ref := f.recRef(rs, id)
	return f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
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
		release := false
		if d.UniqueKey != "" {
			lock, err := tx.Get(f.uniqueCol(rs).Doc(d.UniqueKey))
			if err != nil && !isNotFound(err) {
				return err
			}
			if err == nil {
				var u uniqueDoc
				if err := lock.DataTo(&u); err != nil {
					return err
				}
				release = u.RecordID == id
			}
		}
		if release {
			if err := tx.Delete(f.uniqueCol(rs).Doc(d.UniqueKey)); err != nil {
				return err
			}
		}
		return tx.Delete(ref)
	})
}
