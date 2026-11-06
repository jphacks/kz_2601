package store

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
)

var _ RunnerStops = (*Firestore)(nil)

type stopDoc struct {
	Level     string    `firestore:"level"`
	AppID     string    `firestore:"appId"`
	RuleID    string    `firestore:"ruleId,omitempty"`
	By        string    `firestore:"by"`
	Reason    string    `firestore:"reason,omitempty"`
	Until     time.Time `firestore:"until,omitempty"`
	CreatedAt time.Time `firestore:"createdAt"`
	LiftedAt  time.Time `firestore:"liftedAt,omitempty"`
}

func toStopDoc(s Stop) stopDoc {
	if s.CreatedAt.IsZero() {
		s.CreatedAt = Now()
	}
	return stopDoc{Level: s.Level, AppID: s.AppID, RuleID: s.RuleID, By: s.By, Reason: s.Reason,
		Until: s.Until.UTC(), CreatedAt: s.CreatedAt.UTC(), LiftedAt: s.LiftedAt.UTC()}
}

func (d *stopDoc) toStop(id string) Stop {
	return Stop{ID: id, Level: d.Level, AppID: d.AppID, RuleID: d.RuleID, By: d.By, Reason: d.Reason,
		Until: d.Until, CreatedAt: d.CreatedAt, LiftedAt: d.LiftedAt}
}

func (f *Firestore) stopsCol(env string) *firestore.CollectionRef {
	return f.c.Collection(runnerCollection).Doc(env).Collection("stops")
}

func readStop(snap *firestore.DocumentSnapshot) (Stop, bool, error) {
	if snap == nil || !snap.Exists() {
		return Stop{}, false, nil
	}
	var d stopDoc
	if err := snap.DataTo(&d); err != nil {
		return Stop{}, false, err
	}
	return d.toStop(snap.Ref.ID), true, nil
}

// GetStops implements RunnerStops (one batched read).
func (f *Firestore) GetStops(ctx context.Context, env string, ids []string) (map[string]Stop, error) {
	out := map[string]Stop{}
	if len(ids) == 0 {
		return out, nil
	}
	refs := make([]*firestore.DocumentRef, len(ids))
	for i, id := range ids {
		refs[i] = f.stopsCol(env).Doc(id)
	}
	snaps, err := f.c.GetAll(ctx, refs)
	if err != nil {
		return nil, err
	}
	for _, snap := range snaps {
		s, ok, err := readStop(snap)
		if err != nil {
			return nil, err
		}
		if ok {
			out[s.ID] = s
		}
	}
	return out, nil
}

// PutStop implements RunnerStops.
func (f *Firestore) PutStop(ctx context.Context, env string, s Stop) error {
	_, err := f.stopsCol(env).Doc(s.ID).Set(ctx, toStopDoc(s))
	return err
}

// PutStopIfInactive implements RunnerStops.
func (f *Firestore) PutStopIfInactive(ctx context.Context, env string, s Stop, now time.Time) (bool, error) {
	ref := f.stopsCol(env).Doc(s.ID)
	put := false
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		put = false
		snap, err := tx.Get(ref)
		if err != nil && !isNotFound(err) {
			return err
		}
		if err == nil {
			old, _, err := readStop(snap)
			if err != nil {
				return err
			}
			if old.Active(now) {
				return nil
			}
		}
		put = true
		return tx.Set(ref, toStopDoc(s))
	})
	return put, err
}

// LiftStop implements RunnerStops.
func (f *Firestore) LiftStop(ctx context.Context, env, id string, at time.Time) (bool, error) {
	ref := f.stopsCol(env).Doc(id)
	lifted := false
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		lifted = false
		snap, err := tx.Get(ref)
		if isNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		old, _, err := readStop(snap)
		if err != nil || !old.Active(at) {
			return err
		}
		lifted = true
		return tx.Update(ref, []firestore.Update{{Path: "liftedAt", Value: at.UTC().Truncate(time.Microsecond)}})
	})
	return lifted, err
}

// ListStops implements RunnerStops.
func (f *Firestore) ListStops(ctx context.Context, env string) ([]Stop, error) {
	out := []Stop{}
	err := each(ctx, f.stopsCol(env).OrderBy(firestore.DocumentID, firestore.Asc), func(snap *firestore.DocumentSnapshot) error {
		s, _, err := readStop(snap)
		if err == nil {
			out = append(out, s)
		}
		return err
	})
	return out, err
}

// ReportsSince implements RunnerStops (createdAt: automatic single-field index).
func (f *Firestore) ReportsSince(ctx context.Context, since time.Time, limit int) ([]Report, error) {
	q := f.c.Collection(reportsCollection).Where("createdAt", ">=", since.UTC()).OrderBy("createdAt", firestore.Desc)
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
