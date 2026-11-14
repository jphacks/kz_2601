package loop

import (
	"context"
	"sort"
	"time"

	"cloud.google.com/go/firestore"
)

var _ Control = (*Firestore)(nil)

// AppendAudit implements Control.
func (f *Firestore) AppendAudit(ctx context.Context, e Event) error {
	_, err := f.c.Collection(colAudit).Doc(e.ID).Create(ctx, e)
	return err
}

// ListAudit implements Control. One equality filter or the time range, so no
// composite index; the rest is filtered in memory.
func (f *Firestore) ListAudit(ctx context.Context, q AuditQuery) ([]Event, error) {
	query := f.c.Collection(colAudit).Query
	switch {
	case q.DraftID != "":
		query = query.Where("draftId", "==", q.DraftID)
	case q.ItemID != "":
		query = query.Where("itemId", "==", q.ItemID)
	case !q.Since.IsZero():
		query = query.Where("at", ">=", q.Since)
	}
	out := []Event{}
	err := each(ctx, query.Limit(5000), func(s *firestore.DocumentSnapshot) error {
		var e Event
		if err := s.DataTo(&e); err != nil {
			return err
		}
		if e.At.Before(q.Since) || (q.ItemID != "" && e.ItemID != q.ItemID) {
			return nil
		}
		e.ID = s.Ref.ID
		out = append(out, e)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	if n := limitOr(q.Limit, 200); len(out) > n {
		out = out[len(out)-n:]
	}
	return out, nil
}

func (f *Firestore) switchesRef() *firestore.DocumentRef {
	return f.c.Collection(colControl).Doc("switches")
}

// Switches implements Control. Only boolean fields count.
func (f *Firestore) Switches(ctx context.Context) (map[string]bool, error) {
	out := map[string]bool{}
	snap, err := f.switchesRef().Get(ctx)
	if isNotFound(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for k, v := range snap.Data() {
		if b, ok := v.(bool); ok {
			out[k] = b
		}
	}
	return out, nil
}

// SetSwitch implements Control.
func (f *Firestore) SetSwitch(ctx context.Context, name string, on bool, by string, at time.Time) error {
	_, err := f.switchesRef().Set(ctx, map[string]any{name: on, "updatedBy": by, "updatedAt": at}, firestore.MergeAll)
	return err
}

func (f *Firestore) usageRef(month string) *firestore.DocumentRef {
	return f.c.Collection(colControl).Doc("usage-" + month)
}

type usageDoc struct {
	Tokens int64 `firestore:"tokens"`
}

// AddUsage implements Control.
func (f *Firestore) AddUsage(ctx context.Context, month string, tokens int) (int, error) {
	ref := f.usageRef(month)
	if _, err := ref.Set(ctx, map[string]any{"tokens": firestore.Increment(tokens)}, firestore.MergeAll); err != nil {
		return 0, err
	}
	return f.Usage(ctx, month)
}

// Usage implements Control.
func (f *Firestore) Usage(ctx context.Context, month string) (int, error) {
	snap, err := f.usageRef(month).Get(ctx)
	if isNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var d usageDoc
	if err := snap.DataTo(&d); err != nil {
		return 0, err
	}
	return int(d.Tokens), nil
}
