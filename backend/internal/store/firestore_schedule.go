package store

import (
	"context"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

var _ Schedules = (*Firestore)(nil)

type scheduleDoc struct {
	AppID            string    `firestore:"appId"`
	UID              string    `firestore:"uid,omitempty"`
	Scope            string    `firestore:"scope"`
	RuleID           string    `firestore:"ruleId"`
	Every            string    `firestore:"every"`
	Time             string    `firestore:"time"`
	Weekday          string    `firestore:"weekday,omitempty"`
	Day              int64     `firestore:"day,omitempty"`
	TimeZone         string    `firestore:"tz"`
	Version          int64     `firestore:"version"`
	CreatorAnonymous bool      `firestore:"creatorAnonymous,omitempty"`
	Status           string    `firestore:"status"`
	NextRunAt        time.Time `firestore:"nextRunAt,omitempty"` // absent = out of the due range
	LastDueAt        time.Time `firestore:"lastDueAt,omitempty"`
	CreatedAt        time.Time `firestore:"createdAt"`
	UpdatedAt        time.Time `firestore:"updatedAt"`
}

func toScheduleDoc(s *Schedule) scheduleDoc {
	return scheduleDoc{
		AppID: s.AppID, UID: s.UID, Scope: s.Scope, RuleID: s.RuleID,
		Every: s.Every, Time: s.Time, Weekday: s.Weekday, Day: int64(s.Day), TimeZone: s.TimeZone,
		Version: int64(s.Version), CreatorAnonymous: s.CreatorAnonymous, Status: s.Status,
		NextRunAt: s.NextRunAt, LastDueAt: s.LastDueAt, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}

func (d *scheduleDoc) toSchedule(id string) Schedule {
	return Schedule{
		ID: id, AppID: d.AppID, UID: d.UID, Scope: d.Scope, RuleID: d.RuleID,
		Every: d.Every, Time: d.Time, Weekday: d.Weekday, Day: int(d.Day), TimeZone: d.TimeZone,
		Version: int(d.Version), CreatorAnonymous: d.CreatorAnonymous, Status: d.Status,
		NextRunAt: d.NextRunAt, LastDueAt: d.LastDueAt, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

func (f *Firestore) schedulesCol(env string) *firestore.CollectionRef {
	return f.c.Collection(runnerCollection).Doc(env).Collection("schedules")
}

func (f *Firestore) runnerAppRef(env, key string) *firestore.DocumentRef {
	return f.c.Collection(runnerCollection).Doc(env).Collection("apps").Doc(key)
}

// PutSchedule implements Schedules.
func (f *Firestore) PutSchedule(ctx context.Context, env string, s Schedule) (bool, error) {
	ref := f.schedulesCol(env).Doc(s.ID)
	put := false
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		put = false
		next := s
		snap, err := tx.Get(ref)
		switch {
		case err == nil:
			var d scheduleDoc
			if err := snap.DataTo(&d); err != nil {
				return err
			}
			old := d.toSchedule(s.ID)
			if old.Version > s.Version {
				return nil
			}
			if old.SameTiming(s) && old.Status == ScheduleActive {
				next.NextRunAt, next.LastDueAt, next.CreatedAt = old.NextRunAt, old.LastDueAt, old.CreatedAt
			}
		case !isNotFound(err):
			return err
		}
		now := Now()
		if next.CreatedAt.IsZero() {
			next.CreatedAt = now
		}
		next.UpdatedAt = now
		if next.Status == "" {
			next.Status = ScheduleActive
		}
		put = true
		return tx.Set(ref, toScheduleDoc(&next))
	})
	return put, err
}

// DeleteSchedule implements Schedules.
func (f *Firestore) DeleteSchedule(ctx context.Context, env, id string, maxVersion int) (bool, error) {
	ref := f.schedulesCol(env).Doc(id)
	deleted := false
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		deleted = false
		snap, err := tx.Get(ref)
		if isNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		var d scheduleDoc
		if err := snap.DataTo(&d); err != nil {
			return err
		}
		if int(d.Version) > maxVersion {
			return nil
		}
		deleted = true
		return tx.Delete(ref)
	})
	return deleted, err
}

// ListSchedules implements Schedules (one equality on the single field appId).
func (f *Firestore) ListSchedules(ctx context.Context, env, appID string) ([]Schedule, error) {
	it := f.schedulesCol(env).Where("appId", "==", appID).Documents(ctx)
	defer it.Stop()
	var out []Schedule
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		var d scheduleDoc
		if err := snap.DataTo(&d); err != nil {
			return nil, err
		}
		out = append(out, d.toSchedule(snap.Ref.ID))
	}
}

// ClaimSchedules implements Schedules: one range query on the single field
// nextRunAt, then one transaction per schedule.
func (f *Firestore) ClaimSchedules(ctx context.Context, env string, now time.Time, limit int, advance ScheduleAdvance) ([]OutboxEvent, error) {
	q := f.schedulesCol(env).Where("nextRunAt", "<=", now).OrderBy("nextRunAt", firestore.Asc)
	if limit > 0 {
		q = q.Limit(limit)
	}
	it := q.Documents(ctx)
	var refs []*firestore.DocumentRef
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			it.Stop()
			return nil, err
		}
		refs = append(refs, snap.Ref)
	}
	it.Stop()
	var out []OutboxEvent
	for _, ref := range refs {
		var created *OutboxEvent
		err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
			created = nil
			snap, err := tx.Get(ref)
			if err != nil {
				return err
			}
			var d scheduleDoc
			if err := snap.DataTo(&d); err != nil {
				return err
			}
			if _, err := snap.DataAt("nextRunAt"); err != nil || d.Status != ScheduleActive || d.NextRunAt.After(now) {
				return nil // claimed by another runner in between, or not active
			}
			next, ev := advance(d.toSchedule(ref.ID))
			var evRef *firestore.DocumentRef
			if ev != nil {
				events := []OutboxEvent{*ev}
				if err := prepareOutbox(events, now); err != nil {
					return err
				}
				ev = &events[0]
				evRef = f.outboxCol(ev.Env).Doc(ev.EventID)
				if _, err := tx.Get(evRef); err == nil {
					evRef = nil // this run is already queued
				} else if !isNotFound(err) {
					return err
				}
			}
			upd := []firestore.Update{
				{Path: "lastDueAt", Value: d.NextRunAt},
				{Path: "updatedAt", Value: Now()},
			}
			if next.IsZero() {
				upd = append(upd, firestore.Update{Path: "nextRunAt", Value: firestore.Delete}, firestore.Update{Path: "status", Value: ScheduleInvalid})
			} else {
				upd = append(upd, firestore.Update{Path: "nextRunAt", Value: next})
			}
			if err := tx.Update(ref, upd); err != nil {
				return err
			}
			if evRef != nil {
				if err := tx.Create(evRef, toOutboxDoc(ev)); err != nil {
					return err
				}
				created = ev
			}
			return nil
		})
		if isNotFound(err) {
			continue // deleted in between
		}
		if err != nil {
			return out, err
		}
		if created != nil {
			out = append(out, *created)
		}
	}
	return out, nil
}

type runnerAppDoc struct {
	LastOpenedAt    time.Time `firestore:"lastOpenedAt,omitempty"`
	TimeZone        string    `firestore:"timeZone,omitempty"`
	TimeZoneVersion int64     `firestore:"timeZoneVersion,omitempty"`
}

// TouchAppOpened implements Schedules (one merge write, no read).
func (f *Firestore) TouchAppOpened(ctx context.Context, env, key string, at time.Time) error {
	_, err := f.runnerAppRef(env, key).Set(ctx, map[string]any{"lastOpenedAt": at.UTC()}, firestore.MergeAll)
	return err
}

// SetAppTimeZone implements Schedules.
func (f *Firestore) SetAppTimeZone(ctx context.Context, env, key, tz string, version int) error {
	ref := f.runnerAppRef(env, key)
	return f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err == nil {
			var d runnerAppDoc
			if err := snap.DataTo(&d); err != nil {
				return err
			}
			if int(d.TimeZoneVersion) > version {
				return nil
			}
		} else if !isNotFound(err) {
			return err
		}
		return tx.Set(ref, map[string]any{"timeZone": tz, "timeZoneVersion": version}, firestore.MergeAll)
	})
}

// RunnerApp implements Schedules.
func (f *Firestore) RunnerApp(ctx context.Context, env, key string) (RunnerApp, error) {
	snap, err := f.runnerAppRef(env, key).Get(ctx)
	if isNotFound(err) {
		return RunnerApp{}, nil
	}
	if err != nil {
		return RunnerApp{}, err
	}
	var d runnerAppDoc
	if err := snap.DataTo(&d); err != nil {
		return RunnerApp{}, err
	}
	return RunnerApp{LastOpenedAt: d.LastOpenedAt, TimeZone: d.TimeZone, TimeZoneVersion: int(d.TimeZoneVersion)}, nil
}
