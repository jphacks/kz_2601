package store

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	reportsCollection     = "reports"
	reportQuotaCollection = "report_quota"
	blocksCollection      = "blocks" // under users/{uid}
)

type reportDoc struct {
	ReporterUID string    `firestore:"reporterUid"`
	Kind        string    `firestore:"kind"`
	TargetID    string    `firestore:"targetId"`
	TargetUID   string    `firestore:"targetUid,omitempty"`
	Reason      string    `firestore:"reason"`
	Status      string    `firestore:"status"`
	CreatedAt   time.Time `firestore:"createdAt"`
}

type quotaDoc struct {
	Day string `firestore:"day"`
	N   int64  `firestore:"n"`
}

type blockDoc struct {
	Name      string    `firestore:"name,omitempty"`
	CreatedAt time.Time `firestore:"createdAt"`
}

// AddReport implements Moderation: the report and the reporter's daily counter
// change in one transaction.
func (f *Firestore) AddReport(ctx context.Context, r Report, day string, limit int) (bool, error) {
	ref := f.c.Collection(reportsCollection).Doc(r.ID)
	qref := f.c.Collection(reportQuotaCollection).Doc(r.ReporterUID)
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	created := false
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		created = false
		if _, err := tx.Get(ref); err == nil {
			return nil
		} else if !isNotFound(err) {
			return err
		}
		var q quotaDoc
		if snap, err := tx.Get(qref); err == nil {
			if err := snap.DataTo(&q); err != nil {
				return err
			}
		} else if !isNotFound(err) {
			return err
		}
		if q.Day != day {
			q = quotaDoc{Day: day}
		}
		if limit > 0 && q.N >= int64(limit) {
			return ErrRateLimited
		}
		q.N++
		if err := tx.Set(qref, q); err != nil {
			return err
		}
		created = true
		return tx.Create(ref, reportDoc{ReporterUID: r.ReporterUID, Kind: r.Kind, TargetID: r.TargetID,
			TargetUID: r.TargetUID, Reason: r.Reason, Status: r.Status, CreatedAt: r.CreatedAt})
	})
	if status.Code(err) == codes.AlreadyExists {
		return false, nil
	}
	return created, err
}

// ListReports implements Moderation (createdAt desc: automatic single-field index).
func (f *Firestore) ListReports(ctx context.Context, limit int) ([]Report, error) {
	q := f.c.Collection(reportsCollection).OrderBy("createdAt", firestore.Desc)
	if limit > 0 {
		q = q.Limit(limit)
	}
	var out []Report
	err := each(ctx, q, func(s *firestore.DocumentSnapshot) error {
		var d reportDoc
		if err := s.DataTo(&d); err != nil {
			return err
		}
		out = append(out, Report{ID: s.Ref.ID, ReporterUID: d.ReporterUID, Kind: d.Kind, TargetID: d.TargetID,
			TargetUID: d.TargetUID, Reason: d.Reason, Status: d.Status, CreatedAt: d.CreatedAt})
		return nil
	})
	return out, err
}

// SetReportStatus implements Moderation.
func (f *Firestore) SetReportStatus(ctx context.Context, id, status string) error {
	_, err := f.c.Collection(reportsCollection).Doc(id).Update(ctx, []firestore.Update{{Path: "status", Value: status}})
	if isNotFound(err) {
		return ErrNotFound
	}
	return err
}

func (f *Firestore) blockRef(uid, blockedUID string) *firestore.DocumentRef {
	return f.userRef(uid).Collection(blocksCollection).Doc(blockedUID)
}

// PutBlock implements Moderation.
func (f *Firestore) PutBlock(ctx context.Context, uid string, b Block) error {
	ref := f.blockRef(uid, b.UID)
	return f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		d := blockDoc{Name: b.Name, CreatedAt: b.CreatedAt}
		if d.CreatedAt.IsZero() {
			d.CreatedAt = time.Now().UTC()
		}
		if snap, err := tx.Get(ref); err == nil {
			var old blockDoc
			if err := snap.DataTo(&old); err != nil {
				return err
			}
			d.CreatedAt = old.CreatedAt
		} else if !isNotFound(err) {
			return err
		}
		return tx.Set(ref, d)
	})
}

// DeleteBlock implements Moderation.
func (f *Firestore) DeleteBlock(ctx context.Context, uid, blockedUID string) error {
	_, err := f.blockRef(uid, blockedUID).Delete(ctx)
	return err
}

// ListBlocks implements Moderation.
func (f *Firestore) ListBlocks(ctx context.Context, uid string) ([]Block, error) {
	q := f.userRef(uid).Collection(blocksCollection).OrderBy("createdAt", firestore.Asc).Limit(MaxBlocks)
	out := []Block{}
	err := each(ctx, q, func(s *firestore.DocumentSnapshot) error {
		var d blockDoc
		if err := s.DataTo(&d); err != nil {
			return err
		}
		out = append(out, Block{UID: s.Ref.ID, Name: d.Name, CreatedAt: d.CreatedAt})
		return nil
	})
	return out, err
}

func hiddenUpdates(hidden bool, reason string) []firestore.Update {
	if !hidden {
		return []firestore.Update{{Path: "hidden", Value: firestore.Delete}, {Path: "hiddenReason", Value: firestore.Delete},
			{Path: "hiddenAt", Value: firestore.Delete}}
	}
	return []firestore.Update{{Path: "hidden", Value: true}, {Path: "hiddenReason", Value: reason},
		{Path: "hiddenAt", Value: time.Now().UTC()}}
}

// SetCatalogHidden implements Moderation.
func (f *Firestore) SetCatalogHidden(ctx context.Context, id string, hidden bool, reason string) error {
	_, err := f.catalogRef(id).Update(ctx, hiddenUpdates(hidden, reason))
	if isNotFound(err) {
		return ErrNotFound
	}
	return err
}

// SetPublicAppHidden implements Moderation.
func (f *Firestore) SetPublicAppHidden(ctx context.Context, appID string, hidden bool, reason string) error {
	_, err := f.publicRef(appID).Update(ctx, hiddenUpdates(hidden, reason))
	if isNotFound(err) {
		return ErrNotFound
	}
	return err
}
