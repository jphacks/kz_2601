package runner

import (
	"context"
	"log/slog"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Stop switches (#346, decisions 337 / 339). Above them RUNNER_PAUSED stops
// everything (skipped_paused). The operator's global (the same without a
// new revision), app and rule stops finish a trigger as skipped_stopped
// (reason operator_stop, stop_level global / app / rule); scheduled runs go
// through the same check. A forced app / global stop also makes appconfig
// refuse the writes that would trigger rules (appconfig/app_stop.go);
// RUNNER_NOTIFY_PAUSED and an app's notify pause let the rule run and write
// but send nothing. A run that met a stop is finished: nothing is kept to
// run (or send) after the stop is lifted.

// Values of the stop_level / notify_paused log attrs.
const (
	StopLevelApp  = store.StopApp
	StopLevelRule = store.StopRule
	// StopLevelGlobal: runner/{env}/stops/global (moderate stop-all).
	StopLevelGlobal = store.StopGlobal
	// NotifyPausedEnv: RUNNER_NOTIFY_PAUSED. NotifyPausedApp: the app's notify pause.
	NotifyPausedEnv = "env"
	NotifyPausedApp = "app"
)

// AutoPause is the automatic notify pause: when Reporters different people
// reported an app (the app or a post in it) within Window, its
// notifications are paused for For. Only notifications: hiding, deleting
// and disabling accounts stay with people (#339).
type AutoPause struct {
	// Reporters (RUNNER_AUTO_NOTIFY_PAUSE_REPORTERS): 0 = DefaultAutoPauseReporters; < 0 = off.
	Reporters int
	// Window: 0 = DefaultAutoPauseWindow (decision: 24 hours).
	Window time.Duration
	// For (RUNNER_AUTO_NOTIFY_PAUSE_FOR): 0 = DefaultAutoPauseFor.
	For time.Duration
	// Scan: reports read per tick at most; 0 = DefaultAutoPauseScan.
	Scan int
}

// Defaults of AutoPause.
const (
	DefaultAutoPauseReporters = 3
	DefaultAutoPauseWindow    = 24 * time.Hour
	DefaultAutoPauseFor       = 72 * time.Hour
	DefaultAutoPauseScan      = 1000
)

func (a AutoPause) withDefaults() AutoPause {
	if a.Reporters == 0 {
		a.Reporters = DefaultAutoPauseReporters
	}
	if a.Window <= 0 {
		a.Window = DefaultAutoPauseWindow
	}
	if a.For <= 0 {
		a.For = DefaultAutoPauseFor
	}
	if a.Scan <= 0 {
		a.Scan = DefaultAutoPauseScan
	}
	return a
}

// stops are the operator's switches that apply to one event.
type stops struct {
	app    *store.Stop // the global stop, else the app's (Level tells which)
	notify string      // NotifyPaused* or ""
	rules  map[string]bool
}

// stopsFor reads the app's, the app's notify and the event's rules' stops
// in one batch.
func (r *Runner) stopsFor(ctx context.Context, ev store.OutboxEvent) (stops, error) {
	ids := []string{store.AppStopID(ev.AppID), store.NotifyStopID(ev.AppID), store.GlobalStopID}
	for _, id := range ev.RuleIDs {
		ids = append(ids, store.RuleStopID(ev.AppID, id))
	}
	got, err := r.cfg.Store.GetStops(ctx, r.cfg.Env, ids)
	if err != nil {
		return stops{}, err
	}
	now := r.cfg.Now()
	var out stops
	if s, ok := got[store.GlobalStopID]; ok && s.Active(now) {
		out.app = &s // stop_level=global
	} else if s, ok := got[ids[0]]; ok && s.Active(now) {
		out.app = &s
	}
	if r.cfg.NotifyPaused() {
		out.notify = NotifyPausedEnv
	} else if s, ok := got[ids[1]]; ok && s.Active(now) {
		out.notify = NotifyPausedApp
	}
	for _, id := range ev.RuleIDs {
		if s, ok := got[store.RuleStopID(ev.AppID, id)]; ok && s.Active(now) {
			if out.rules == nil {
				out.rules = map[string]bool{}
			}
			out.rules[id] = true
		}
	}
	return out, nil
}

// autoPause pauses the notifications of the apps that crossed the report
// threshold. Reports from before the app's last notify pause ended do not
// count again (a lifted or expired pause needs new reporters). Failures are
// logged; the tick goes on. It returns how many apps it paused.
func (r *Runner) autoPause(ctx context.Context) int {
	a := r.cfg.AutoPause
	if a.Reporters < 0 {
		return 0
	}
	now := r.cfg.Now()
	reps, err := r.cfg.Store.ReportsSince(ctx, now.Add(-a.Window), a.Scan)
	if err != nil {
		r.cfg.Logger.WarnContext(ctx, "runner: reading reports for the notify pause failed", slog.Any("error", err))
		return 0
	}
	if len(reps) >= a.Scan {
		r.cfg.Logger.WarnContext(ctx, "runner: report scan hit its limit", slog.Int("limit", a.Scan))
	}
	byApp := map[string][]store.Report{}
	for _, rep := range reps {
		if app := store.ReportedAppID(rep); app != "" && rep.Status != "dismissed" {
			byApp[app] = append(byApp[app], rep)
		}
	}
	paused := 0
	for app, list := range byApp {
		if distinctReporters(list, time.Time{}) < a.Reporters {
			continue
		}
		id := store.NotifyStopID(app)
		got, err := r.cfg.Store.GetStops(ctx, r.cfg.Env, []string{id})
		if err != nil {
			r.cfg.Logger.WarnContext(ctx, "runner: reading a notify pause failed", slog.String("app_id", app), slog.Any("error", err))
			continue
		}
		prev := got[id]
		if prev.Active(now) || distinctReporters(list, prev.Ended(now)) < a.Reporters {
			continue
		}
		s := store.Stop{ID: id, Level: store.StopNotify, AppID: app, By: store.StopByAuto,
			Reason: "reports", Until: now.Add(a.For), CreatedAt: now}
		put, err := r.cfg.Store.PutStopIfInactive(ctx, r.cfg.Env, s, now)
		if err != nil {
			r.cfg.Logger.WarnContext(ctx, "runner: writing a notify pause failed", slog.String("app_id", app), slog.Any("error", err))
			continue
		}
		if put {
			paused++
			// The operator's alert can key on this line. No uid, no content.
			r.cfg.Logger.LogAttrs(ctx, slog.LevelWarn, "runner.stop",
				slog.String("event", "runner.stop"), slog.String("app_id", app), slog.String("level", store.StopNotify),
				slog.String("by", store.StopByAuto), slog.Int("reporters", a.Reporters), slog.Time("until", s.Until))
		}
	}
	return paused
}

// distinctReporters counts the people who reported after `after`.
func distinctReporters(list []store.Report, after time.Time) int {
	seen := map[string]bool{}
	for _, rep := range list {
		if rep.CreatedAt.After(after) {
			seen[rep.ReporterUID] = true
		}
	}
	return len(seen)
}

// ruleStopped is a stopped rule's outcome.
func (r *Runner) ruleStopped(ctx context.Context, ev store.OutboxEvent, rule *dsl.ServerRule) string {
	r.logRun(ctx, ev, runLog{ruleID: rule.ID, outcome: OutcomeSkippedStopped, reason: ReasonOperatorStop, stopLevel: StopLevelRule})
	return OutcomeSkippedStopped
}
