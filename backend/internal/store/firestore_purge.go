package store

import (
	"context"
	"errors"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// PurgeApp implements Store. Deletes go through a BulkWriter; subcollections
// (versions, entities/*/records, installs) are walked explicitly because
// Firestore keeps them when their parent document is deleted.
func (f *Firestore) PurgeApp(ctx context.Context, uid, appID string) (PurgeResult, error) {
	var res PurgeResult
	app := f.appRef(uid, appID)
	if _, err := app.Get(ctx); isNotFound(err) {
		return res, ErrNotFound
	} else if err != nil {
		return res, err
	}
	n, err := f.purgeShares(ctx, uid, appID)
	res.Shares = n
	if err != nil {
		return res, err
	}
	if res.Listing, err = f.purgeOwned(ctx, f.catalogRef(appID), uid); err != nil {
		return res, err
	}
	if res.Mirror, err = f.purgeOwned(ctx, f.publicRef(appID), uid); err != nil {
		return res, err
	}
	counts, err := f.deleteTree(ctx, app)
	res.Versions, res.Records = counts["versions"], counts["records"]
	return res, err
}

func (f *Firestore) purgeShares(ctx context.Context, uid, appID string) (int, error) {
	it := f.c.Collection(sharesCollection).Where("ownerUid", "==", uid).Where("appId", "==", appID).Documents(ctx)
	defer it.Stop()
	var refs []*firestore.DocumentRef
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return 0, err
		}
		refs = append(refs, snap.Ref)
	}
	return len(refs), f.bulkDelete(ctx, refs)
}

// purgeOwned deletes ref (with its subcollections) when its ownerUid is uid.
func (f *Firestore) purgeOwned(ctx context.Context, ref *firestore.DocumentRef, uid string) (bool, error) {
	snap, err := ref.Get(ctx)
	if isNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if owner, _ := snap.Data()["ownerUid"].(string); owner != uid {
		return false, nil
	}
	_, err = f.deleteTree(ctx, ref)
	return err == nil, err
}

// deleteTree deletes ref and every document below it, deepest first, and
// returns how many documents it deleted per collection id.
func (f *Firestore) deleteTree(ctx context.Context, ref *firestore.DocumentRef) (map[string]int, error) {
	var refs []*firestore.DocumentRef
	counts := map[string]int{}
	var walk func(*firestore.DocumentRef) error
	walk = func(d *firestore.DocumentRef) error {
		cols := d.Collections(ctx)
		for {
			col, err := cols.Next()
			if errors.Is(err, iterator.Done) {
				break
			}
			if err != nil {
				return err
			}
			docs := col.DocumentRefs(ctx) // includes "missing" parents such as entities/{e}
			for {
				child, err := docs.Next()
				if errors.Is(err, iterator.Done) {
					break
				}
				if err != nil {
					return err
				}
				if err := walk(child); err != nil {
					return err
				}
				counts[col.ID]++
			}
		}
		refs = append(refs, d)
		return nil
	}
	if err := walk(ref); err != nil {
		return counts, err
	}
	return counts, f.bulkDelete(ctx, refs)
}

func (f *Firestore) bulkDelete(ctx context.Context, refs []*firestore.DocumentRef) error {
	if len(refs) == 0 {
		return nil
	}
	bw := f.c.BulkWriter(ctx)
	jobs := make([]*firestore.BulkWriterJob, 0, len(refs))
	for _, r := range refs {
		j, err := bw.Delete(r)
		if err != nil {
			bw.End()
			return err
		}
		jobs = append(jobs, j)
	}
	bw.End()
	for _, j := range jobs {
		if _, err := j.Results(); err != nil && !isNotFound(err) {
			return err
		}
	}
	return nil
}
