package store

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// DeleteUserData implements Accounts.
//
// Order matters for idempotency: shared records are found through the uid's
// own apps (posting requires the private copy, see appconfig.entityOf), so
// they are deleted before users/{uid}. A retry after any partial failure
// therefore still finds what is left.
func (f *Firestore) DeleteUserData(ctx context.Context, uid string) error {
	bw := f.c.BulkWriter(ctx)
	defer bw.End()

	appIDs, err := docIDs(f.c.Collection("users").Doc(uid).Collection("apps").DocumentRefs(ctx))
	if err != nil {
		return fmt.Errorf("list apps: %w", err)
	}
	for _, appID := range appIDs {
		if err := f.deleteAuthoredPublicRecords(ctx, bw, uid, appID); err != nil {
			return fmt.Errorf("public records of %s: %w", appID, err)
		}
	}
	// apps_public: only the document; other users' records stay in its subcollections.
	for _, col := range []string{publicCollection, sharesCollection, catalogCollection} {
		if err := f.deleteMatches(ctx, bw, f.c.Collection(col).Where("ownerUid", "==", uid)); err != nil {
			return fmt.Errorf("%s: %w", col, err)
		}
	}
	bw.Flush()
	if err := f.deleteTree(ctx, bw, f.c.Collection("users").Doc(uid)); err != nil {
		return fmt.Errorf("users/%s: %w", uid, err)
	}
	bw.Flush()
	return nil
}

// deleteAuthoredPublicRecords deletes apps_public/{appID}/entities/*/records
// whose author is uid. Entity documents are never written ("missing"), which
// DocumentRefs still lists.
func (f *Firestore) deleteAuthoredPublicRecords(ctx context.Context, bw *firestore.BulkWriter, uid, appID string) error {
	entities, err := docIDs(f.publicRef(appID).Collection("entities").DocumentRefs(ctx))
	if err != nil {
		return err
	}
	for _, e := range entities {
		q := f.recordsCol(PublicRecords(appID, e)).Where("author", "==", uid)
		if err := f.deleteMatches(ctx, bw, q); err != nil {
			return err
		}
	}
	return nil
}

// deleteMatches enqueues a delete for every document q matches (not their subcollections).
func (f *Firestore) deleteMatches(ctx context.Context, bw *firestore.BulkWriter, q firestore.Query) error {
	it := q.Select().Documents(ctx)
	defer it.Stop()
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := bw.Delete(snap.Ref); err != nil {
			return err
		}
	}
}

// deleteTree deletes doc and every document below it (the Go client has no
// RecursiveDelete). Missing parent documents are listed too, so records under
// a never-written entities/{e} are reached.
func (f *Firestore) deleteTree(ctx context.Context, bw *firestore.BulkWriter, doc *firestore.DocumentRef) error {
	cols := doc.Collections(ctx)
	for {
		col, err := cols.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return err
		}
		children, err := col.DocumentRefs(ctx).GetAll()
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := f.deleteTree(ctx, bw, child); err != nil {
				return err
			}
		}
	}
	_, err := bw.Delete(doc)
	return err
}

func docIDs(it *firestore.DocumentRefIterator) ([]string, error) {
	refs, err := it.GetAll()
	if err != nil {
		return nil, err
	}
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = r.ID
	}
	return out, nil
}
