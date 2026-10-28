package store

import (
	"context"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

func (f *Firestore) markByRef(rs RecordSet, recordID, markID, uid string) *firestore.DocumentRef {
	return f.recRef(rs, recordID).Collection("marks").Doc(markID).Collection("by").Doc(uid)
}

// SetMark implements Marks. The per-user doc and the record's markCounts
// change in one transaction, so the count always equals the number of docs.
func (f *Firestore) SetMark(ctx context.Context, rs RecordSet, recordID, markID, uid string, on bool) (MarkState, error) {
	rec := f.recRef(rs, recordID)
	by := f.markByRef(rs, recordID, markID, uid)
	var st MarkState
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(rec)
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
		mine := true
		if _, err := tx.Get(by); isNotFound(err) {
			mine = false
		} else if err != nil {
			return err
		}
		count := int(d.MarkCounts[markID])
		st = MarkState{Count: count, Mine: on}
		if mine == on {
			return nil
		}
		if on {
			count++
			if err := tx.Set(by, map[string]any{"at": time.Now().UTC()}); err != nil {
				return err
			}
		} else {
			count = max(0, count-1)
			if err := tx.Delete(by); err != nil {
				return err
			}
		}
		st.Count = count
		return tx.Update(rec, []firestore.Update{{FieldPath: firestore.FieldPath{"markCounts", markID}, Value: count}})
	})
	return st, err
}

// markedByChunk bounds one GetAll call.
const markedByChunk = 300

// MarkedBy implements Marks with batched reads of the caller's own docs.
func (f *Firestore) MarkedBy(ctx context.Context, rs RecordSet, recordIDs, markIDs []string, uid string) (map[string]map[string]bool, error) {
	type key struct{ rid, mid string }
	var refs []*firestore.DocumentRef
	var keys []key
	for _, rid := range recordIDs {
		for _, mid := range markIDs {
			refs = append(refs, f.markByRef(rs, rid, mid, uid))
			keys = append(keys, key{rid, mid})
		}
	}
	out := map[string]map[string]bool{}
	for start := 0; start < len(refs); start += markedByChunk {
		end := min(start+markedByChunk, len(refs))
		snaps, err := f.c.GetAll(ctx, refs[start:end])
		if err != nil {
			return nil, err
		}
		for i, s := range snaps {
			if !s.Exists() {
				continue
			}
			k := keys[start+i]
			if out[k.rid] == nil {
				out[k.rid] = map[string]bool{}
			}
			out[k.rid][k.mid] = true
		}
	}
	return out, nil
}

// deleteMarks removes records/{id}/marks/** (Firestore keeps subcollections
// when their parent is deleted).
func (f *Firestore) deleteMarks(ctx context.Context, rec *firestore.DocumentRef) error {
	it := rec.Collection("marks").DocumentRefs(ctx)
	for {
		ref, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := f.deleteTree(ctx, ref); err != nil {
			return err
		}
	}
}
