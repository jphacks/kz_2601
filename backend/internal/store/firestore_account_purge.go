package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// DeletedAccounts implements AccountPurger. deletedAt < cutoff ordered by
// deletedAt uses the automatic single-field index (no composite index);
// purged tombstones have no deletedAt and never match.
func (f *Firestore) DeletedAccounts(ctx context.Context, cutoff time.Time, limit int) ([]DeletedAccount, error) {
	q := f.c.Collection("users").Where("deletedAt", "<", cutoff).OrderBy("deletedAt", firestore.Asc)
	if limit > 0 {
		q = q.Limit(limit)
	}
	var out []DeletedAccount
	err := each(ctx, q, func(s *firestore.DocumentSnapshot) error {
		if st, _ := s.DataAt("status"); st != AccountDeleted {
			return nil
		}
		at, _ := s.DataAt("deletedAt")
		t, _ := at.(time.Time)
		out = append(out, DeletedAccount{UID: s.Ref.ID, DeletedAt: t})
		return nil
	})
	return out, err
}

// PurgeAccount implements AccountPurger. Every step only deletes what is
// still there, so a retry after a partial failure finishes the rest.
func (f *Firestore) PurgeAccount(ctx context.Context, uid string) (AccountPurgeResult, error) {
	var res AccountPurgeResult
	if gone, err := f.UserDeleted(ctx, uid); err != nil {
		return res, err
	} else if !gone {
		return res, ErrNotDeleted
	}
	// DocumentRefs also returns app documents a failed run already deleted
	// while their subcollections remain.
	err := eachRef(f.userRef(uid).Collection("apps").DocumentRefs(ctx), func(app *firestore.DocumentRef) error {
		if err := f.purgeAccountApp(ctx, uid, app, &res); err != nil {
			return fmt.Errorf("app: %w", err)
		}
		return nil
	})
	if err != nil {
		return res, err
	}
	steps := []struct {
		name string
		fn   func() error
	}{
		{"shares", func() error {
			n, err := f.deleteQuery(ctx, f.c.Collection(sharesCollection).Where("ownerUid", "==", uid), nil)
			res.Shares += n
			return err
		}},
		{"listings", func() error {
			return each(ctx, f.c.Collection(catalogCollection).Where("ownerUid", "==", uid), func(s *firestore.DocumentSnapshot) error {
				res.Listings++
				_, err := f.deleteTree(ctx, s.Ref)
				return err
			})
		}},
		{"releases", func() error {
			n, err := f.deleteQuery(ctx, f.c.Collection(releaseCollection).Where("ownerUid", "==", uid), nil)
			res.Releases += n
			return err
		}},
		{"installs", func() error {
			n, err := f.purgeInstalls(ctx, uid)
			res.Installs += n
			return err
		}},
		{"moderation", func() error {
			for _, field := range []string{"reporterUid", "targetUid"} {
				n, err := f.deleteQuery(ctx, f.c.Collection(reportsCollection).Where(field, "==", uid), nil)
				res.Reports += n
				if err != nil {
					return err
				}
			}
			if _, err := f.c.Collection(reportQuotaCollection).Doc(uid).Delete(ctx); err != nil {
				return err
			}
			n, err := f.deleteQuery(ctx, f.userRef(uid).Collection(blocksCollection).Query, nil)
			res.Blocks += n
			return err
		}},
		{"owners", func() error {
			// a mirror of uid that is still there was kept for other people:
			// its app_owners entry stays with it.
			var refs []*firestore.DocumentRef
			err := each(ctx, f.c.Collection(ownersCollection).Where("uid", "==", uid), func(s *firestore.DocumentSnapshot) error {
				owned, err := f.ownsMirror(ctx, s.Ref.ID, uid)
				if !owned && err == nil {
					refs = append(refs, s.Ref)
				}
				return err
			})
			if err != nil {
				return err
			}
			res.Owners += len(refs)
			return f.bulkDelete(ctx, refs)
		}},
	}
	for _, s := range steps {
		if err := s.fn(); err != nil {
			return res, fmt.Errorf("%s: %w", s.name, err)
		}
	}
	return res, nil
}

// purgeAccountApp erases users/{uid}/apps/{appId} with what uid made public
// for it. uid's apps_public mirror is erased too unless others take part.
func (f *Firestore) purgeAccountApp(ctx context.Context, uid string, app *firestore.DocumentRef, res *AccountPurgeResult) error {
	appID := app.ID
	n, err := f.purgeShares(ctx, uid, appID)
	res.Shares += n
	if err != nil {
		return err
	}
	listed, err := f.purgeOwned(ctx, f.catalogRef(appID), uid)
	if err != nil {
		return err
	}
	if listed {
		res.Listings++
	} else if owner, err := f.AppOwner(ctx, appID); err == nil && owner == uid {
		// DELETE /v1/me took the listing down but left its installs subcollection.
		if _, err := f.catalogRef(appID).Get(ctx); isNotFound(err) {
			if _, err := f.deleteTree(ctx, f.catalogRef(appID)); err != nil {
				return err
			}
		}
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}

	owned, err := f.ownsMirror(ctx, appID, uid)
	if err != nil {
		return err
	}
	kept := false
	if owned {
		if kept, err = f.othersIn(ctx, appID, uid); err != nil {
			return err
		}
		if !kept {
			if _, err := f.deleteTree(ctx, f.publicRef(appID)); err != nil {
				return err
			}
		}
	}
	if !owned || kept {
		if err := f.scrubPublic(ctx, appID, uid, res); err != nil {
			return err
		}
	}
	if kept {
		res.KeptMirrors++
	}
	if _, err := f.deleteTree(ctx, app); err != nil {
		return err
	}
	res.Apps++
	return nil
}

// ownsMirror reports whether apps_public/{appId} exists with ownerUid uid.
func (f *Firestore) ownsMirror(ctx context.Context, appID, uid string) (bool, error) {
	snap, err := f.publicRef(appID).Get(ctx)
	if isNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	owner, _ := snap.Data()["ownerUid"].(string)
	return owner == uid, nil
}

// othersIn reports whether anyone but uid joined or posted in apps_public/{appId}.
func (f *Firestore) othersIn(ctx context.Context, appID, uid string) (bool, error) {
	found := errors.New("found")
	err := eachRef(f.membersCol(appID).DocumentRefs(ctx), func(m *firestore.DocumentRef) error {
		if m.ID != uid {
			return found
		}
		return nil
	})
	if err == nil {
		err = f.eachPublicEntity(ctx, appID, func(e *firestore.DocumentRef) error {
			return each(ctx, e.Collection("records").Query, func(s *firestore.DocumentSnapshot) error {
				if a, _ := s.Data()["author"].(string); a != uid {
					return found
				}
				return nil
			})
		})
	}
	if errors.Is(err, found) {
		return true, nil
	}
	return false, err
}

// scrubPublic removes uid from apps_public/{appId} while keeping uid's posts:
// the membership and uid's marks go, the posts lose their authorName.
func (f *Firestore) scrubPublic(ctx context.Context, appID, uid string, res *AccountPurgeResult) error {
	if _, err := f.membersCol(appID).Doc(uid).Delete(ctx, firestore.Exists); err == nil {
		res.Memberships++
	} else if !isNotFound(err) {
		return err
	}
	return f.eachPublicEntity(ctx, appID, func(e *firestore.DocumentRef) error {
		recs := e.Collection("records")
		err := each(ctx, recs.Where("author", "==", uid), func(s *firestore.DocumentSnapshot) error {
			if name, _ := s.Data()["authorName"].(string); name == "" {
				return nil
			}
			res.PostsScrubbed++
			_, err := s.Ref.Update(ctx, []firestore.Update{{Path: "authorName", Value: firestore.Delete}})
			return err
		})
		if err != nil {
			return err
		}
		rs := PublicRecords(appID, e.ID)
		return eachRef(recs.DocumentRefs(ctx), func(rec *firestore.DocumentRef) error {
			return eachRef(rec.Collection("marks").DocumentRefs(ctx), func(mark *firestore.DocumentRef) error {
				by := mark.Collection("by").Doc(uid)
				if _, err := by.Get(ctx); isNotFound(err) {
					return nil
				} else if err != nil {
					return err
				}
				res.Marks++
				_, err := f.SetMark(ctx, rs, rec.ID, mark.ID, uid, false) // also decrements markCounts
				if errors.Is(err, ErrNotFound) {                          // the record itself is gone
					_, err = by.Delete(ctx)
				}
				return err
			})
		})
	})
}

func (f *Firestore) eachPublicEntity(ctx context.Context, appID string, fn func(*firestore.DocumentRef) error) error {
	return eachRef(f.publicRef(appID).Collection("entities").DocumentRefs(ctx), fn)
}

// purgeInstalls deletes public_apps/*/installs/{uid} (uid's installs of
// other people's listings), reading at most MaxCatalogApps markers.
func (f *Firestore) purgeInstalls(ctx context.Context, uid string) (int, error) {
	var refs []*firestore.DocumentRef
	err := eachRef(f.c.Collection(catalogCollection).DocumentRefs(ctx), func(l *firestore.DocumentRef) error {
		if len(refs) < MaxCatalogApps {
			refs = append(refs, l.Collection("installs").Doc(uid))
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	var hit []*firestore.DocumentRef
	for start := 0; start < len(refs); start += markedByChunk {
		snaps, err := f.c.GetAll(ctx, refs[start:min(start+markedByChunk, len(refs))])
		if err != nil {
			return 0, err
		}
		for _, s := range snaps {
			if s.Exists() {
				hit = append(hit, s.Ref)
			}
		}
	}
	return len(hit), f.bulkDelete(ctx, hit)
}

// deleteQuery deletes the documents q returns (those keep accepts, when set).
func (f *Firestore) deleteQuery(ctx context.Context, q firestore.Query, keep func(*firestore.DocumentSnapshot) bool) (int, error) {
	var refs []*firestore.DocumentRef
	err := each(ctx, q, func(s *firestore.DocumentSnapshot) error {
		if keep == nil || keep(s) {
			refs = append(refs, s.Ref)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(refs), f.bulkDelete(ctx, refs)
}

// MarkAccountPurged implements AccountPurger. The full overwrite drops deletedAt.
func (f *Firestore) MarkAccountPurged(ctx context.Context, uid string) error {
	ref := f.userRef(uid)
	return f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if isNotFound(err) {
			return ErrNotDeleted
		}
		if err != nil {
			return err
		}
		switch st, _ := snap.DataAt("status"); st {
		case AccountPurged:
			return nil
		case AccountDeleted:
			return tx.Set(ref, map[string]any{"status": AccountPurged, "purgedAt": time.Now().UTC()})
		default:
			return ErrNotDeleted
		}
	})
}

// eachRef calls fn for every reference it yields.
func eachRef(it *firestore.DocumentRefIterator, fn func(*firestore.DocumentRef) error) error {
	for {
		ref, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := fn(ref); err != nil {
			return err
		}
	}
}
