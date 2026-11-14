package store

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
)

const followsCollection = "follows" // under apps_public/{appId}

type followsDoc struct {
	UIDs      []string  `firestore:"uids"`
	UpdatedAt time.Time `firestore:"updatedAt"`
}

func (f *Firestore) followsCol(appID string) *firestore.CollectionRef {
	return f.publicRef(appID).Collection(followsCollection)
}

// SetFollow implements Follows in one transaction, so concurrent follows of
// the same person never lose an entry or pass MaxFollows.
func (f *Firestore) SetFollow(ctx context.Context, appID, uid, target string, on bool) (int, error) {
	ref := f.followsCol(appID).Doc(uid)
	var n int
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		var d followsDoc
		snap, err := tx.Get(ref)
		switch {
		case err == nil:
			if err := snap.DataTo(&d); err != nil {
				return err
			}
		case !isNotFound(err):
			return err
		}
		next, changed, err := nextFollows(d.UIDs, target, on)
		n = len(next)
		if err != nil || !changed {
			return err
		}
		if len(next) == 0 {
			return tx.Delete(ref)
		}
		return tx.Set(ref, followsDoc{UIDs: next, UpdatedAt: Now()})
	})
	return n, err
}

// ListFollows implements Follows.
func (f *Firestore) ListFollows(ctx context.Context, appID, uid string) ([]string, error) {
	snap, err := f.followsCol(appID).Doc(uid).Get(ctx)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var d followsDoc
	if err := snap.DataTo(&d); err != nil {
		return nil, err
	}
	return d.UIDs, nil
}

// purgeFollows drops uid's follows in appID and uid from everyone's follows
// there (array-contains: automatic single-field index).
func (f *Firestore) purgeFollows(ctx context.Context, appID, uid string, res *AccountPurgeResult) error {
	if _, err := f.followsCol(appID).Doc(uid).Delete(ctx, firestore.Exists); err == nil {
		res.Follows++
	} else if !isNotFound(err) {
		return err
	}
	return each(ctx, f.followsCol(appID).Where("uids", "array-contains", uid), func(s *firestore.DocumentSnapshot) error {
		_, err := s.Ref.Update(ctx, []firestore.Update{{Path: "uids", Value: firestore.ArrayRemove(uid)}})
		if err == nil {
			res.Follows++
		}
		return err
	})
}
