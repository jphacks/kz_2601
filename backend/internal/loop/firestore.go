package loop

import (
	"context"
	"errors"
	"sort"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Collections (layout: loop.go). Queries use equality filters or a range on
// one field only, so no composite index is needed.
const (
	colInbox   = "loop_inbox"
	colDrafts  = "loop_drafts"
	colNotices = "loop_notices"
	colAudit   = "loop_audit"
	colControl = "loop_control"
	// colFeedback is store's feedback collection (#319), read only to sweep
	// copies whose source is gone (purge.go).
	colFeedback = "feedback"
)

// Firestore is the production Store.
type Firestore struct {
	c *firestore.Client
}

// NewFirestore wraps a client (the binary owns and closes it).
func NewFirestore(c *firestore.Client) *Firestore { return &Firestore{c: c} }

var _ Store = (*Firestore)(nil)

func isNotFound(err error) bool { return status.Code(err) == codes.NotFound }

func isAlreadyExists(err error) bool { return status.Code(err) == codes.AlreadyExists }

// PutItem implements Store.
func (f *Firestore) PutItem(ctx context.Context, it Item) (bool, error) {
	_, err := f.c.Collection(colInbox).Doc(it.ID).Create(ctx, it)
	if isAlreadyExists(err) {
		return false, nil
	}
	return err == nil, err
}

// GetItem implements Store.
func (f *Firestore) GetItem(ctx context.Context, id string) (*Item, error) {
	snap, err := f.c.Collection(colInbox).Doc(id).Get(ctx)
	if isNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var it Item
	if err := snap.DataTo(&it); err != nil {
		return nil, err
	}
	it.ID = id
	return &it, nil
}

// UpdateItem implements Store (the whole document is replaced; it must exist).
func (f *Firestore) UpdateItem(ctx context.Context, it Item) error {
	ref := f.c.Collection(colInbox).Doc(it.ID)
	return f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		if _, err := tx.Get(ref); err != nil {
			if isNotFound(err) {
				return ErrNotFound
			}
			return err
		}
		return tx.Set(ref, it)
	})
}

// ListItems implements Store. Sorted in memory (status equality + order on
// another field would need a composite index); the status filter keeps the
// page small.
func (f *Firestore) ListItems(ctx context.Context, q ItemQuery) ([]Item, error) {
	query := f.c.Collection(colInbox).Query
	if q.Status != "" {
		query = query.Where("status", "==", q.Status)
	}
	out := []Item{}
	err := each(ctx, query.Limit(2000), func(s *firestore.DocumentSnapshot) error {
		var it Item
		if err := s.DataTo(&it); err != nil {
			return err
		}
		it.ID = s.Ref.ID
		out = append(out, it)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].IngestedAt.Equal(out[j].IngestedAt) {
			return out[i].IngestedAt.Before(out[j].IngestedAt)
		}
		return out[i].ID < out[j].ID
	})
	if n := limitOr(q.Limit, 100); len(out) > n {
		out = out[:n]
	}
	return out, nil
}

// PutDraft implements Store.
func (f *Firestore) PutDraft(ctx context.Context, d Draft) error {
	_, err := f.c.Collection(colDrafts).Doc(d.ID).Set(ctx, d)
	return err
}

// GetDraft implements Store.
func (f *Firestore) GetDraft(ctx context.Context, id string) (*Draft, error) {
	snap, err := f.c.Collection(colDrafts).Doc(id).Get(ctx)
	if isNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var d Draft
	if err := snap.DataTo(&d); err != nil {
		return nil, err
	}
	d.ID = id
	return &d, nil
}

// ListDrafts implements Store.
func (f *Firestore) ListDrafts(ctx context.Context, q DraftQuery) ([]Draft, error) {
	query := f.c.Collection(colDrafts).Query
	if q.Status != "" {
		query = query.Where("status", "==", q.Status)
	}
	out := []Draft{}
	err := each(ctx, query.Limit(2000), func(s *firestore.DocumentSnapshot) error {
		var d Draft
		if err := s.DataTo(&d); err != nil {
			return err
		}
		d.ID = s.Ref.ID
		out = append(out, d)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	if n := limitOr(q.Limit, 100); len(out) > n {
		out = out[:n]
	}
	return out, nil
}

// PutNotice implements Store.
func (f *Firestore) PutNotice(ctx context.Context, n Notice) (bool, error) {
	_, err := f.c.Collection(colNotices).Doc(n.ID).Create(ctx, n)
	if isAlreadyExists(err) {
		return false, nil
	}
	return err == nil, err
}

func (f *Firestore) notices(ctx context.Context, uid string) ([]Notice, error) {
	out := []Notice{}
	err := each(ctx, f.c.Collection(colNotices).Where("uid", "==", uid).Limit(500), func(s *firestore.DocumentSnapshot) error {
		var n Notice
		if err := s.DataTo(&n); err != nil {
			return err
		}
		n.ID = s.Ref.ID
		out = append(out, n)
		return nil
	})
	return out, err
}

// LastPush implements Store.
func (f *Firestore) LastPush(ctx context.Context, uid string) (time.Time, error) {
	ns, err := f.notices(ctx, uid)
	if err != nil {
		return time.Time{}, err
	}
	var last time.Time
	for _, n := range ns {
		if n.Pushed && n.CreatedAt.After(last) {
			last = n.CreatedAt
		}
	}
	return last, nil
}

// ListNotices implements Store.
func (f *Firestore) ListNotices(ctx context.Context, uid string, limit int) ([]Notice, error) {
	ns, err := f.notices(ctx, uid)
	if err != nil {
		return nil, err
	}
	sort.Slice(ns, func(i, j int) bool { return ns[i].CreatedAt.After(ns[j].CreatedAt) })
	if n := limitOr(limit, 50); len(ns) > n {
		ns = ns[:n]
	}
	return ns, nil
}

type cursorDoc struct {
	At time.Time `firestore:"at"`
}

// Cursor implements Store.
func (f *Firestore) Cursor(ctx context.Context, src Source) (time.Time, error) {
	snap, err := f.c.Collection(colControl).Doc("cursor-" + string(src)).Get(ctx)
	if isNotFound(err) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	var d cursorDoc
	if err := snap.DataTo(&d); err != nil {
		return time.Time{}, err
	}
	return d.At, nil
}

// SetCursor implements Store.
func (f *Firestore) SetCursor(ctx context.Context, src Source, t time.Time) error {
	_, err := f.c.Collection(colControl).Doc("cursor-"+string(src)).Set(ctx, cursorDoc{At: t})
	return err
}

// PurgeExpired implements Store.
func (f *Firestore) PurgeExpired(ctx context.Context, now time.Time, limit int, dryRun bool) (int, error) {
	total := 0
	for _, col := range []string{colInbox, colDrafts, colNotices, colAudit} {
		q := f.c.Collection(col).Where("expiresAt", "<", now).Select()
		if limit > 0 {
			q = q.Limit(limit)
		}
		var refs []*firestore.DocumentRef
		if err := each(ctx, q, func(s *firestore.DocumentSnapshot) error {
			refs = append(refs, s.Ref)
			return nil
		}); err != nil {
			return total, err
		}
		total += len(refs)
		if !dryRun {
			if err := f.deleteRefs(ctx, refs); err != nil {
				return total, err
			}
		}
	}
	return total, nil
}

// DeleteItems implements Store.
func (f *Firestore) DeleteItems(ctx context.Context, ids []string) error {
	return f.deleteIDs(ctx, colInbox, ids)
}

// DeleteNotices implements Store.
func (f *Firestore) DeleteNotices(ctx context.Context, ids []string) error {
	return f.deleteIDs(ctx, colNotices, ids)
}

// ItemsByUID implements Store.
func (f *Firestore) ItemsByUID(ctx context.Context, uid string) ([]string, []string, error) {
	ids := func(col string) ([]string, error) {
		out := []string{}
		err := each(ctx, f.c.Collection(col).Where("uid", "==", uid).Select(), func(s *firestore.DocumentSnapshot) error {
			out = append(out, s.Ref.ID)
			return nil
		})
		sort.Strings(out)
		return out, err
	}
	items, err := ids(colInbox)
	if err != nil {
		return nil, nil, err
	}
	notices, err := ids(colNotices)
	return items, notices, err
}

// FeedbackExists reports which of the feedback ids still exist (purge.go:
// a copy whose source was erased goes too).
func (f *Firestore) FeedbackExists(ctx context.Context, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	for start := 0; start < len(ids); start += 30 {
		chunk := ids[start:min(start+30, len(ids))]
		refs := make([]*firestore.DocumentRef, len(chunk))
		for i, id := range chunk {
			refs[i] = f.c.Collection(colFeedback).Doc(id)
		}
		q := f.c.Collection(colFeedback).Where(firestore.DocumentID, "in", refs).Select()
		if err := each(ctx, q, func(s *firestore.DocumentSnapshot) error {
			out[s.Ref.ID] = true
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (f *Firestore) deleteIDs(ctx context.Context, col string, ids []string) error {
	refs := make([]*firestore.DocumentRef, len(ids))
	for i, id := range ids {
		refs[i] = f.c.Collection(col).Doc(id)
	}
	return f.deleteRefs(ctx, refs)
}

func (f *Firestore) deleteRefs(ctx context.Context, refs []*firestore.DocumentRef) error {
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
