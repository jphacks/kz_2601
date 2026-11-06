package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/runner/schedule"
	"github.com/jphacks/kz_2601/backend/internal/runner/scope"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Time-triggered rules (#342). appconfig queues rules_changed when a DSL
// with (or that had) time triggers is saved; the runner rebuilds the app's
// rows of the schedule table from it. Each tick takes the due rows, moves
// them to their next time and queues one schedule event per run (its id is
// the row + the due time, so a run is queued once), which then runs like a
// record trigger.

// errOwnerPending: a shared app's appId is not claimed yet; retried.
var errOwnerPending = errors.New("runner: app owner not recorded yet")

// JitterMax spreads the runs of one minute over 0-4 minutes by appId
// (schedule.Jitter is in [0, JitterMax)).
const JitterMax = 5 * time.Minute

// specOf is the schedule of a stored row.
func specOf(s store.Schedule) schedule.Spec {
	return schedule.Spec{Every: schedule.Every(s.Every), Time: s.Time, Weekday: s.Weekday, Day: s.Day}
}

// nextRun is the first run of s strictly after `after` (jitter included),
// or zero when s cannot be scheduled.
func nextRun(s store.Schedule, after time.Time) time.Time {
	j := schedule.Jitter(s.AppID, JitterMax)
	base := schedule.Next(specOf(s), after.Add(-j), schedule.Location(s.TimeZone))
	if base.IsZero() {
		return time.Time{}
	}
	return base.Add(j)
}

// scheduleEventID is the id of the run of schedule id due at due: the same
// row and time always give the same id. Hashed because a private row's key
// holds the uid and event ids are logged.
func scheduleEventID(id string, due time.Time) string {
	sum := sha256.Sum256([]byte(id + "@" + due.UTC().Format(time.RFC3339)))
	return "sch_" + hex.EncodeToString(sum[:16])
}

// claimSchedules queues the runs that are due. Runs missed while the runner
// was down are not queued one by one: the row moves to its next time after
// now and the one run queued is skipped_late if it is too old.
func (r *Runner) claimSchedules(ctx context.Context, deadline time.Time, res *TickResult) error {
	for time.Now().Before(deadline) {
		now := r.cfg.Now()
		evs, err := r.cfg.Store.ClaimSchedules(ctx, r.cfg.Env, now, r.cfg.Batch, func(s store.Schedule) (time.Time, *store.OutboxEvent) {
			ev := &store.OutboxEvent{
				EventID: scheduleEventID(s.ID, s.NextRunAt), Env: r.cfg.Env, Kind: store.OutboxSchedule,
				AppID: s.AppID, Scope: s.Scope, UID: s.UID, RuleIDs: []string{s.RuleID}, Version: s.Version,
				ActorAnonymous: s.CreatorAnonymous, TimeZone: s.TimeZone, DueAt: s.NextRunAt, CreatedAt: now,
			}
			return nextRun(s, now), ev
		})
		res.Scheduled += len(evs)
		if err != nil {
			return err
		}
		if len(evs) < r.cfg.Batch {
			return nil
		}
	}
	return nil
}

// rebuild makes the app's schedule rows match the DSL version of a
// rules_changed event: rows of the time-triggered rules are written (a row
// whose timing did not change keeps its next run) and rows of rules that
// are gone are deleted. Rows of a newer version are left alone, so events
// handled out of order cannot roll the table back.
func (r *Runner) rebuild(ctx context.Context, ev store.OutboxEvent) (string, error) {
	st := r.cfg.Store
	v, err := st.GetVersion(ctx, ev.UID, ev.AppID, ev.Version)
	if errors.Is(err, store.ErrNotFound) {
		r.logRebuild(ctx, ev, OutcomeSkippedStopped, ReasonVersionMissing, 0)
		return OutcomeSkippedStopped, nil
	}
	if err != nil {
		return "", err
	}
	app, err := decodeDSL(v.DSL)
	if err != nil {
		r.logRebuild(ctx, ev, OutcomeSkippedStopped, ReasonVersionMissing, 0)
		return OutcomeSkippedStopped, nil
	}
	uid, scopeOf := ev.UID, dsl.ScopePrivate
	for i := range app.Entities {
		switch app.Entities[i].EffectiveScope() {
		case dsl.ScopePublic:
			scopeOf = dsl.ScopePublic
		case dsl.ScopeShared:
			if scopeOf != dsl.ScopePublic {
				scopeOf = dsl.ScopeShared
			}
		}
	}
	if scopeOf != dsl.ScopePrivate {
		// shared data: only the creator's DSL counts (#337); one row per app
		owner, err := st.AppOwner(ctx, ev.AppID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return "", err
		}
		if owner == "" {
			return "", errOwnerPending // the creator's first save claims the appId right after the commit
		}
		if owner != ev.UID {
			r.logRebuild(ctx, ev, OutcomeSkippedStopped, ReasonNotCreator, 0)
			return OutcomeSkippedStopped, nil
		}
		uid = ""
	}
	tz := schedule.Location(ev.TimeZone).String()
	if err := st.SetAppTimeZone(ctx, r.cfg.Env, store.TenantKey(ev.AppID, uid), tz, ev.Version); err != nil {
		return "", err
	}

	now := r.cfg.Now()
	keep := map[string]bool{}
	for i := range app.ServerRules {
		rule := &app.ServerRules[i]
		if !rule.IsScheduled() {
			continue
		}
		s := store.Schedule{
			ID: store.ScheduleKey(ev.AppID, uid, rule.ID), AppID: ev.AppID, UID: uid, Scope: string(scopeOf), RuleID: rule.ID,
			Every: rule.On.Every, Time: rule.On.Time, Weekday: rule.On.Weekday, Day: rule.On.Day, TimeZone: tz,
			Version: ev.Version, CreatorAnonymous: ev.ActorAnonymous, Status: store.ScheduleActive,
		}
		if s.NextRunAt = nextRun(s, now); s.NextRunAt.IsZero() {
			s.Status = store.ScheduleInvalid
		}
		if _, err := st.PutSchedule(ctx, r.cfg.Env, s); err != nil {
			return "", err
		}
		keep[s.ID] = true
	}
	rows, err := st.ListSchedules(ctx, r.cfg.Env, ev.AppID)
	if err != nil {
		return "", err
	}
	for _, s := range rows {
		if s.UID != uid || keep[s.ID] {
			continue
		}
		if _, err := st.DeleteSchedule(ctx, r.cfg.Env, s.ID, ev.Version); err != nil {
			return "", err
		}
	}
	r.logRebuild(ctx, ev, "ok", "", len(keep))
	return "ok", nil
}

func (r *Runner) logRebuild(ctx context.Context, ev store.OutboxEvent, outcome, reason string, rules int) {
	attrs := []slog.Attr{
		slog.String("event", "runner.schedules"),
		slog.String("event_id", ev.EventID),
		slog.String("app_id", ev.AppID),
		slog.Int("version", ev.Version),
		slog.String("outcome", outcome),
		slog.Int("scheduled_rules", rules),
	}
	if reason != "" {
		attrs = append(attrs, slog.String("reason", reason))
	}
	r.cfg.Logger.LogAttrs(ctx, slog.LevelInfo, "runner.schedules", attrs...)
}

// dropSchedule deletes the row of a schedule event whose app (or creator)
// is gone, so it stops queueing runs.
func (r *Runner) dropSchedule(ctx context.Context, ev store.OutboxEvent) {
	uid := ev.UID
	if ev.Scope != string(dsl.ScopePrivate) && ev.Scope != "" {
		uid = ""
	}
	for _, id := range ev.RuleIDs {
		if _, err := r.cfg.Store.DeleteSchedule(ctx, r.cfg.Env, store.ScheduleKey(ev.AppID, uid, id), ev.Version); err != nil {
			r.cfg.Logger.WarnContext(ctx, "runner: deleting the schedule of a deleted app failed",
				slog.String("app_id", ev.AppID), slog.Any("error", err))
		}
	}
}

// inactive reports whether nobody opened the app for InactiveDays before
// the run was due (#338). An app never opened counts from its creation.
func (r *Runner) inactive(ctx context.Context, h *scope.Handle, ev store.OutboxEvent) (bool, error) {
	ra, err := h.RunnerApp(ctx)
	if err != nil {
		return false, err
	}
	last := ra.LastOpenedAt
	if last.IsZero() {
		if last, err = h.CreatedAt(ctx); err != nil {
			return false, err
		}
	}
	return schedule.Inactive(last, ev.DueAt, r.cfg.InactiveDays), nil
}

// location is the creator's time zone of ev: the event's own, else the
// one the runner kept from the creator's last DSL save, else the default.
func (r *Runner) location(ctx context.Context, h *scope.Handle, ev store.OutboxEvent) (*time.Location, error) {
	tz := ev.TimeZone
	if tz == "" {
		ra, err := h.RunnerApp(ctx)
		if err != nil {
			return nil, err
		}
		tz = ra.TimeZone
	}
	return schedule.Location(tz), nil
}
