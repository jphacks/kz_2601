package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// users/{uid} {status: "deleted", deletedAt}. The document is otherwise never
// written (apps live in its subcollection), so absence means active.
func (f *Firestore) userRef(uid string) *firestore.DocumentRef {
	return f.c.Collection("users").Doc(uid)
}

// DeactivateUser implements Accounts.
func (f *Firestore) DeactivateUser(ctx context.Context, uid string) error {
	if err := f.markDeleted(ctx, uid); err != nil {
		return fmt.Errorf("mark users/%s: %w", uid, err)
	}
	now := time.Now().UTC()
	bw := f.c.BulkWriter(ctx)
	defer bw.End()
	err := each(ctx, f.userRef(uid).Collection("apps").Query, func(s *firestore.DocumentSnapshot) error {
		if st, _ := s.DataAt("status"); st == "archived" {
			return nil
		}
		_, err := bw.Update(s.Ref, []firestore.Update{{Path: "status", Value: "archived"}, {Path: "updatedAt", Value: now}})
		return err
	})
	if err != nil {
		return fmt.Errorf("archive apps: %w", err)
	}
	err = each(ctx, f.c.Collection(sharesCollection).Where("ownerUid", "==", uid), func(s *firestore.DocumentSnapshot) error {
		if r, _ := s.DataAt("revoked"); r == true {
			return nil
		}
		_, err := bw.Update(s.Ref, []firestore.Update{{Path: "revoked", Value: true}})
		return err
	})
	if err != nil {
		return fmt.Errorf("revoke shares: %w", err)
	}
	err = each(ctx, f.c.Collection(catalogCollection).Where("ownerUid", "==", uid), func(s *firestore.DocumentSnapshot) error {
		_, err := bw.Delete(s.Ref)
		return err
	})
	if err != nil {
		return fmt.Errorf("unlist: %w", err)
	}
	bw.Flush()
	return nil
}

// markDeleted writes the mark once (a retry keeps the first deletedAt).
func (f *Firestore) markDeleted(ctx context.Context, uid string) error {
	ref := f.userRef(uid)
	return f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err == nil {
			if st, _ := snap.DataAt("status"); st == AccountDeleted {
				return nil
			}
		} else if !isNotFound(err) {
			return err
		}
		return tx.Set(ref, map[string]any{"status": AccountDeleted, "deletedAt": time.Now().UTC()}, firestore.MergeAll)
	})
}

// UserDeleted implements Accounts.
func (f *Firestore) UserDeleted(ctx context.Context, uid string) (bool, error) {
	snap, err := f.userRef(uid).Get(ctx)
	if isNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	st, _ := snap.DataAt("status")
	return st == AccountDeleted, nil
}

// each calls fn for every document q returns.
func each(ctx context.Context, q firestore.Query, fn func(*firestore.DocumentSnapshot) error) error {
	it := q.Documents(ctx)
	defer it.Stop()
	for {
		s, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := fn(s); err != nil {
			return err
		}
	}
}
