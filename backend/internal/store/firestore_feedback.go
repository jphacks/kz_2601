package store

import (
	"context"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// Layout: see feedback.go. Queries:
//   - daily limit: uid == X && day == D (equality only; single-field indexes)
//   - operator list / retention: createdAt range + order on the same field
//   - account purge: uid == X
//
// None needs a composite index.
const feedbackCollection = "feedback"

type feedbackDoc struct {
	UID            string          `firestore:"uid"`
	Kind           string          `firestore:"kind"`
	Message        string          `firestore:"message"`
	Untrusted      bool            `firestore:"untrusted"` // always true (#366)
	ConsentVersion string          `firestore:"consentVersion"`
	Status         string          `firestore:"status"`
	Attachments    []string        `firestore:"attachments"`
	AppID          string          `firestore:"appId,omitempty"`
	DSLVersion     int64           `firestore:"dslVersion,omitempty"`
	DSLJSON        string          `firestore:"dslJson,omitempty"`
	Screenshot     []byte          `firestore:"screenshotJpeg,omitempty"`
	Device         *FeedbackDevice `firestore:"device,omitempty"`
	Day            string          `firestore:"day"`
	CreatedAt      time.Time       `firestore:"createdAt"`
}

func (d *feedbackDoc) toFeedback(id string, withBodies bool) *Feedback {
	fb := &Feedback{ID: id, UID: d.UID, Kind: d.Kind, Message: d.Message, ConsentVersion: d.ConsentVersion,
		Status: d.Status, AppID: d.AppID, DSLVersion: int(d.DSLVersion), Device: d.Device, Day: d.Day, CreatedAt: d.CreatedAt}
	if withBodies {
		if d.DSLJSON != "" {
			fb.DSL = []byte(d.DSLJSON)
		}
		fb.Screenshot = d.Screenshot
	}
	return fb
}

// CreateFeedback implements Store. The daily count and the create run in one
// transaction so parallel sends cannot exceed perDay.
func (f *Firestore) CreateFeedback(ctx context.Context, fb Feedback, perDay int) (*Feedback, error) {
	now := time.Now().UTC()
	fb.ID, fb.Status, fb.Day, fb.CreatedAt = NewID("fb_"), FeedbackReceived, FeedbackDay(now), now
	d := feedbackDoc{UID: fb.UID, Kind: fb.Kind, Message: fb.Message, Untrusted: true, ConsentVersion: fb.ConsentVersion,
		Status: fb.Status, Attachments: fb.Attachments(), AppID: fb.AppID, DSLVersion: int64(fb.DSLVersion),
		DSLJSON: string(fb.DSL), Screenshot: fb.Screenshot, Device: fb.Device, Day: fb.Day, CreatedAt: fb.CreatedAt}
	col := f.c.Collection(feedbackCollection)
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		if perDay > 0 {
			q := col.Where("uid", "==", fb.UID).Where("day", "==", fb.Day).Select().Limit(perDay)
			snaps, err := tx.Documents(q).GetAll()
			if err != nil {
				return err
			}
			if len(snaps) >= perDay {
				return ErrFeedbackQuota
			}
		}
		return tx.Create(col.Doc(fb.ID), d)
	})
	if err != nil {
		return nil, err
	}
	out := fb
	out.DSL, out.Screenshot = nil, nil
	return &out, nil
}

// ListFeedback implements FeedbackReader. Status is filtered in memory (an
// equality filter plus an order on createdAt would need a composite index).
func (f *Firestore) ListFeedback(ctx context.Context, q FeedbackQuery) ([]Feedback, error) {
	limit := feedbackLimit(q.Limit)
	fq := f.c.Collection(feedbackCollection).OrderBy("createdAt", firestore.Desc)
	if !q.Since.IsZero() {
		fq = fq.Where("createdAt", ">=", q.Since)
	}
	// without the bodies: a screenshot can be ~0.5 MB per document
	fq = fq.Select("uid", "kind", "message", "consentVersion", "status", "appId", "dslVersion", "device", "day", "createdAt")
	it := fq.Documents(ctx)
	defer it.Stop()
	out := []Feedback{}
	for len(out) < limit {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var d feedbackDoc
		if err := snap.DataTo(&d); err != nil {
			return nil, err
		}
		if q.Status == "" || d.Status == q.Status {
			out = append(out, *d.toFeedback(snap.Ref.ID, false))
		}
	}
	return out, nil
}

// GetFeedback implements FeedbackReader.
func (f *Firestore) GetFeedback(ctx context.Context, id string) (*Feedback, error) {
	snap, err := f.c.Collection(feedbackCollection).Doc(id).Get(ctx)
	if isNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var d feedbackDoc
	if err := snap.DataTo(&d); err != nil {
		return nil, err
	}
	return d.toFeedback(id, true), nil
}

// PurgeFeedbackBefore implements FeedbackPurger.
func (f *Firestore) PurgeFeedbackBefore(ctx context.Context, cutoff time.Time, limit int, dryRun bool) (int, error) {
	q := f.c.Collection(feedbackCollection).Where("createdAt", "<", cutoff).Select()
	if limit > 0 {
		q = q.Limit(limit)
	}
	var refs []*firestore.DocumentRef
	if err := each(ctx, q, func(s *firestore.DocumentSnapshot) error {
		refs = append(refs, s.Ref)
		return nil
	}); err != nil {
		return 0, err
	}
	if dryRun {
		return len(refs), nil
	}
	return len(refs), f.bulkDelete(ctx, refs)
}
