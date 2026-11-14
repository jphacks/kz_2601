package store

import (
	"context"
	"sort"
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
	Force     bool      `firestore:"force,omitempty"`
	Operator  string    `firestore:"operator,omitempty"`
}

func toStopDoc(s Stop) stopDoc {
	if s.CreatedAt.IsZero() {
		s.CreatedAt = Now()
	}
	return stopDoc{Level: s.Level, AppID: s.AppID, RuleID: s.RuleID, By: s.By, Reason: s.Reason,
		Until: s.Until.UTC(), CreatedAt: s.CreatedAt.UTC(), LiftedAt: s.LiftedAt.UTC(), Force: s.Force, Operator: s.Operator}
}

func (d *stopDoc) toStop(id string) Stop {
	return Stop{ID: id, Level: d.Level, AppID: d.AppID, RuleID: d.RuleID, By: d.By, Reason: d.Reason,
		Until: d.Until, CreatedAt: d.CreatedAt, LiftedAt: d.LiftedAt, Force: d.Force, Operator: d.Operator}
}

func (f *Firestore) stopsCol(env string) *firestore.CollectionRef {
	return f.c.Collection(runnerCollection).Doc(env).Collection("stops")
}

// stop_log: one document per set / lift, never changed (appId: automatic
// single-field index).
func (f *Firestore) stopLogCol(env string) *firestore.CollectionRef {
	return f.c.Collection(runnerCollection).Doc(env).Collection("stop_log")
}

type stopEventDoc struct {
	Action   string    `firestore:"action"`
	StopID   string    `firestore:"stopId"`
	Level    string    `firestore:"level"`
	AppID    string    `firestore:"appId"`
	RuleID   string    `firestore:"ruleId,omitempty"`
	By       string    `firestore:"by"`
	Operator string    `firestore:"operator,omitempty"`
	Reason   string    `firestore:"reason,omitempty"`
	Force    bool      `firestore:"force,omitempty"`
	Until    time.Time `firestore:"until,omitempty"`
	At       time.Time `firestore:"at"`
}

// logStop adds e to the stop log in tx.
func (f *Firestore) logStop(tx *firestore.Transaction, env string, e StopEvent) error {
	return tx.Create(f.stopLogCol(env).NewDoc(), stopEventDoc{Action: e.Action, StopID: e.StopID, Level: e.Level,
		AppID: e.AppID, RuleID: e.RuleID, By: e.By, Operator: e.Operator, Reason: e.Reason, Force: e.Force,
		Until: e.Until.UTC(), At: e.At.UTC().Truncate(time.Microsecond)})
}

// StopLog implements RunnerStops. With appID it reads the app's entries
// (an equality query, no composite index) and orders them here.
func (f *Firestore) StopLog(ctx context.Context, env, appID string, limit int) ([]StopEvent, error) {
	q := f.stopLogCol(env).OrderBy("at", firestore.Desc)
	if appID != "" {
		q = f.stopLogCol(env).Where("appId", "==", appID)
	} else if limit > 0 {
		q = q.Limit(limit)
	}
	out := []StopEvent{}
	err := each(ctx, q, func(snap *firestore.DocumentSnapshot) error {
		var d stopEventDoc
		if err := snap.DataTo(&d); err != nil {
			return err
		}
		out = append(out, StopEvent{ID: snap.Ref.ID, Action: d.Action, StopID: d.StopID, Level: d.Level, AppID: d.AppID,
			RuleID: d.RuleID, By: d.By, Operator: d.Operator, Reason: d.Reason, Force: d.Force, Until: d.Until, At: d.At})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return page(out, limit, 0), nil
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
	if s.CreatedAt.IsZero() {
		s.CreatedAt = Now()
	}
	return f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		if err := tx.Set(f.stopsCol(env).Doc(s.ID), toStopDoc(s)); err != nil {
			return err
		}
		return f.logStop(tx, env, setEvent(s))
	})
}

// PutStopIfInactive implements RunnerStops.
func (f *Firestore) PutStopIfInactive(ctx context.Context, env string, s Stop, now time.Time) (bool, error) {
	if s.CreatedAt.IsZero() {
		s.CreatedAt = Now()
	}
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
		if err := tx.Set(ref, toStopDoc(s)); err != nil {
			return err
		}
		return f.logStop(tx, env, setEvent(s))
	})
	return put, err
}

// LiftStop implements RunnerStops.
func (f *Firestore) LiftStop(ctx context.Context, env, id string, at time.Time, operator string) (bool, error) {
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
		if err := tx.Update(ref, []firestore.Update{{Path: "liftedAt", Value: at.UTC().Truncate(time.Microsecond)}}); err != nil {
			return err
		}
		return f.logStop(tx, env, liftEvent(old, at, operator))
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
