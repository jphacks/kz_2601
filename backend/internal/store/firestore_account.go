package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// users/{uid} {firstSeenAt} (SeeUser), then {status: "deleted", deletedAt}
// (merged), later only {status: "purged", purgedAt}
// (firestore_account_purge.go). No status means active.
func (f *Firestore) userRef(uid string) *firestore.DocumentRef {
	return f.c.Collection("users").Doc(uid)
}

// DeactivateUser implements Accounts.
func (f *Firestore) DeactivateUser(ctx context.Context, uid string) error {
	if err := f.markDeleted(ctx, uid); err != nil {
		return fmt.Errorf("mark users/%s: %w", uid, err)
	}
	now := Now()
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
	if _, err := f.DeleteDevices(ctx, uid); err != nil {
		return fmt.Errorf("delete devices: %w", err)
	}
	return nil
}

// markDeleted writes the mark once (a retry keeps the first deletedAt).
func (f *Firestore) markDeleted(ctx context.Context, uid string) error {
	ref := f.userRef(uid)
	return f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err == nil {
			if st, _ := snap.DataAt("status"); st == AccountDeleted || st == AccountPurged {
				return nil // keep the first deletedAt; never re-arm a purged tombstone
			}
		} else if !isNotFound(err) {
			return err
		}
		return tx.Set(ref, map[string]any{"status": AccountDeleted, "deletedAt": Now()}, firestore.MergeAll)
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
	return accountGone(snap), nil
}

// SeeUser implements Accounts: the read is UserDeleted's; only an active uid
// without firstSeenAt pays a transaction (once per uid).
func (f *Firestore) SeeUser(ctx context.Context, uid string) (bool, error) {
	ref := f.userRef(uid)
	snap, err := ref.Get(ctx)
	if err != nil && !isNotFound(err) {
		return false, err
	}
	if err == nil {
		if accountGone(snap) {
			return true, nil
		}
		if _, ok := firstSeenOf(snap); ok {
			return false, nil
		}
	}
	at, err := f.earliestOwned(ctx, uid)
	if err != nil {
		return false, fmt.Errorf("first seen users/%s: %w", uid, err)
	}
	deleted := false
	err = f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		deleted = false
		snap, err := tx.Get(ref)
		if err == nil {
			if accountGone(snap) {
				deleted = true
				return nil // never write into a deleted account or a tombstone
			}
			if _, ok := firstSeenOf(snap); ok {
				return nil // another request won the race: keep the first time
			}
		} else if !isNotFound(err) {
			return err
		}
		return tx.Set(ref, map[string]any{"firstSeenAt": earlier(at, Now())}, firestore.MergeAll)
	})
	if err != nil {
		return false, fmt.Errorf("first seen users/%s: %w", uid, err)
	}
	return deleted, nil
}

// earliestOwned is the oldest server-set createdAt among uid's apps and
// devices (zero when none): accounts from before firstSeenAt existed keep
// their age. Two one-document queries on automatic single-field indexes,
// run only on the write path (once per uid).
func (f *Firestore) earliestOwned(ctx context.Context, uid string) (time.Time, error) {
	var at time.Time
	for _, c := range []*firestore.CollectionRef{f.userRef(uid).Collection("apps"), f.devices(uid)} {
		err := each(ctx, c.OrderBy("createdAt", firestore.Asc).Limit(1), func(s *firestore.DocumentSnapshot) error {
			if v, _ := s.DataAt("createdAt"); v != nil {
				if t, ok := v.(time.Time); ok {
					at = earlier(at, t)
				}
			}
			return nil
		})
		if err != nil {
			return time.Time{}, err
		}
	}
	return at, nil
}

// earlier is the earlier of a and b, ignoring a zero time.
func earlier(a, b time.Time) time.Time {
	if a.IsZero() || (!b.IsZero() && b.Before(a)) {
		return b
	}
	return a
}

// UserFirstSeen implements Accounts.
func (f *Firestore) UserFirstSeen(ctx context.Context, uid string) (time.Time, bool, error) {
	snap, err := f.userRef(uid).Get(ctx)
	if isNotFound(err) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	at, ok := firstSeenOf(snap)
	return at, ok, nil
}

func accountGone(snap *firestore.DocumentSnapshot) bool {
	st, _ := snap.DataAt("status")
	return st == AccountDeleted || st == AccountPurged
}

func firstSeenOf(snap *firestore.DocumentSnapshot) (time.Time, bool) {
	v, _ := snap.DataAt("firstSeenAt")
	at, ok := v.(time.Time)
	return at, ok && !at.IsZero()
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
