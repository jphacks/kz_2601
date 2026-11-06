package runner_test

import (
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/runner"
	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/runner/schedule"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// morningRules: a private app whose daily 07:00 rule writes a log record
// dated "today" (in the creator's zone).
const morningRules = `{
  "schemaVersion": 1,
  "app": {"name": "朝", "voiceEntry": "floating"},
  "initialScreen": "home",
  "entities": [
    {"id": "log", "name": "記録", "fields": [
      {"id": "day", "name": "日", "type": "date"},
      {"id": "note", "name": "メモ", "type": "string"}
    ]}
  ],
  "screens": [
    {"id": "home", "title": "朝", "components": [{"type": "list", "entity": "log", "item": {"title": "note"}}]}
  ],
  "serverRules": [
    {"id": "morning", "on": {"every": "daily", "time": "07:00"},
     "do": [{"createRecord": {"entity": "log", "values": {"day": {"op": "today", "args": []}, "note": {"op": "concat", "args": ["おは", "よう"]}}}}]}
  ]
}`

func (h *harness) schedules(appID string) []store.Schedule {
	h.t.Helper()
	rows, err := h.st.ListSchedules(h.t.Context(), h.env, appID)
	if err != nil {
		h.t.Fatal(err)
	}
	return rows
}

func (h *harness) oneSchedule(appID string) store.Schedule {
	h.t.Helper()
	rows := h.schedules(appID)
	if len(rows) != 1 {
		h.t.Fatalf("want 1 schedule of %s, got %+v", appID, rows)
	}
	return rows[0]
}

// scheduleEvents returns appID's schedule runs.
func (h *harness) scheduleEvents(appID string) []store.OutboxEvent {
	h.t.Helper()
	all, err := h.st.(outboxLister).OutboxEvents(h.t.Context(), h.env, 0)
	if err != nil {
		h.t.Fatal(err)
	}
	var out []store.OutboxEvent
	for _, ev := range all {
		if ev.AppID == appID && ev.Kind == store.OutboxSchedule {
			out = append(out, ev)
		}
	}
	return out
}

// tickAt runs the runner with the clock at t.
func (h *harness) tickAt(t time.Time) runner.TickResult {
	h.t.Helper()
	h.clock = t
	res, err := h.r.Tick(h.t.Context(), 30*time.Second)
	if err != nil {
		h.t.Fatal(err)
	}
	return res
}

// morningApp saves uid's morning app from a device in tz and lets the
// runner build its schedule.
func (h *harness) morningApp(id, uid, tz string) (app string, row store.Schedule) {
	h.t.Helper()
	app = "/v1/apps/" + id
	h.putDSLTZ(uid, app, []byte(morningRules), tz)
	h.tick()
	return app, h.oneSchedule(id)
}

var newYork = schedule.Location("America/New_York")

// #342 完了の条件: a New York creator's 07:00 rule runs at 07:00 New York
// time (plus the appId's jitter), once, and "today" is the New York date.
// A second tick at the same time queues nothing.
func TestScheduleRunsOnceInCreatorZone(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		id, uid := "SCH-NY"+h.sfx, "uid-sch-ny"+h.sfx
		_, row := h.morningApp(id, uid, "America/New_York")
		j := schedule.Jitter(id, runner.JitterMax)
		local := row.NextRunAt.Add(-j).In(newYork)
		if local.Hour() != 7 || local.Minute() != 0 || j >= 5*time.Minute || row.TimeZone != "America/New_York" ||
			row.Status != store.ScheduleActive || row.Version < 2 || row.UID != uid || row.ID != store.ScheduleKey(id, uid, "morning") {
			t.Fatalf("schedule: %+v (local %v, jitter %v)", row, local, j)
		}
		if !row.NextRunAt.After(h.clock) || row.NextRunAt.Sub(h.clock) > 25*time.Hour {
			t.Fatalf("next run %v not within a day after %v", row.NextRunAt, h.clock)
		}

		due := row.NextRunAt
		res := h.tickAt(due)
		if res.Scheduled != 1 || res.Done != 1 {
			t.Fatalf("tick at due: %+v", res)
		}
		logs := h.records(store.PrivateRecords(uid, id, "log"))
		if len(logs) != 1 || logs[0].Data["day"] != due.In(newYork).Format("2006-01-02") || logs[0].Origin == nil || logs[0].Origin.RuleID != "morning" {
			t.Fatalf("log records: %+v", logs)
		}
		evs := h.scheduleEvents(id)
		if len(evs) != 1 || evs[0].Outcome != quota.OutcomeOK || !evs[0].DueAt.Equal(due) || evs[0].TimeZone != "America/New_York" {
			t.Fatalf("schedule events: %+v", evs)
		}

		// the same time again: nothing new
		if res := h.tickAt(due); res.Scheduled != 0 || res.Claimed != 0 {
			t.Fatalf("second tick: %+v", res)
		}
		if n := len(h.records(store.PrivateRecords(uid, id, "log"))); n != 1 {
			t.Fatalf("logs after second tick: %d", n)
		}
		next := h.oneSchedule(id)
		if l := next.NextRunAt.Add(-j).In(newYork); l.Hour() != 7 || l.Minute() != 0 || next.NextRunAt.Sub(due) < 23*time.Hour || !next.LastDueAt.Equal(due) {
			t.Fatalf("next run: %+v", next)
		}

		// usage is logged as a schedule trigger, without the raw uid
		if out := h.logs.String(); !strings.Contains(out, `"trigger":"schedule"`) || strings.Contains(out, uid) {
			t.Fatalf("logs: %s", out)
		}
	})
}

// #342: a run more than 30 minutes late is skipped_late and not run; runs
// missed while the runner was away are not queued one by one.
func TestScheduleSkipsLate(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		id, uid := "SCH-LATE"+h.sfx, "uid-sch-late"+h.sfx
		_, row := h.morningApp(id, uid, "")
		if row.TimeZone != schedule.DefaultZone {
			t.Fatalf("default zone: %+v", row)
		}
		// exactly 30 minutes late still runs
		if res := h.tickAt(row.NextRunAt.Add(30 * time.Minute)); res.Scheduled != 1 {
			t.Fatalf("30 min late: %+v", res)
		}
		if evs := h.scheduleEvents(id); len(evs) != 1 || evs[0].Outcome != quota.OutcomeOK {
			t.Fatalf("30 min late: %+v", evs)
		}

		// three days later: one run queued, skipped_late; the row moves past now
		row = h.oneSchedule(id)
		now := row.NextRunAt.Add(3*24*time.Hour + time.Hour)
		if res := h.tickAt(now); res.Scheduled != 1 || res.Done != 1 {
			t.Fatalf("3 days late: %+v", res)
		}
		evs := h.scheduleEvents(id)
		if len(evs) != 2 {
			t.Fatalf("events after 3 days: %+v", evs)
		}
		late := 0
		for _, ev := range evs {
			if ev.Outcome == runner.OutcomeSkippedLate {
				late++
				if !ev.DueAt.Equal(row.NextRunAt) {
					t.Fatalf("late run due %v, want %v", ev.DueAt, row.NextRunAt)
				}
			}
		}
		if late != 1 {
			t.Fatalf("late runs: %+v", evs)
		}
		if n := len(h.records(store.PrivateRecords(uid, id, "log"))); n != 1 {
			t.Fatalf("logs: %d", n)
		}
		if next := h.oneSchedule(id).NextRunAt; !next.After(now) || next.Sub(now) > 25*time.Hour {
			t.Fatalf("next run %v after %v", next, now)
		}
	})
}

// #342: an app nobody opened for 31 days is skipped_inactive (the schedule
// keeps going) and runs again once it is opened.
func TestScheduleInactiveApp(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		id, uid := "SCH-IDLE"+h.sfx, "uid-sch-idle"+h.sfx
		h.morningApp(id, uid, "Asia/Tokyo")
		opened := time.Now() // putDSL opened the app

		// move the row past day 31 (that run is late), then run the next one
		h.tickAt(opened.Add(31 * 24 * time.Hour))
		row := h.oneSchedule(id)
		if res := h.tickAt(row.NextRunAt); res.Scheduled != 1 {
			t.Fatalf("day 32: %+v", res)
		}
		var idle int
		for _, ev := range h.scheduleEvents(id) {
			if ev.Outcome == runner.OutcomeSkippedInactive {
				idle++
			}
		}
		if idle != 1 || len(h.records(store.PrivateRecords(uid, id, "log"))) != 0 {
			t.Fatalf("inactive: %+v", h.scheduleEvents(id))
		}
		if h.oneSchedule(id).Status != store.ScheduleActive {
			t.Fatal("the schedule stopped")
		}

		// opened again: the next run goes
		if err := h.st.TouchAppOpened(t.Context(), h.env, store.TenantKey(id, uid), h.clock.Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
		row = h.oneSchedule(id)
		if res := h.tickAt(row.NextRunAt); res.Scheduled != 1 || res.Done != 1 {
			t.Fatalf("after opening: %+v", res)
		}
		if n := len(h.records(store.PrivateRecords(uid, id, "log"))); n != 1 {
			t.Fatalf("logs after opening: %d", n)
		}
	})
}

// #342: removing the rule from the DSL deletes its schedule; changing only
// other parts of the DSL keeps the next run.
func TestScheduleFollowsRules(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		id, uid := "SCH-DEL"+h.sfx, "uid-sch-del"+h.sfx
		app, row := h.morningApp(id, uid, "Asia/Tokyo")

		renamed := strings.Replace(morningRules, `"name": "朝"`, `"name": "朝の記録"`, 1)
		h.putDSLTZ(uid, app, []byte(renamed), "Asia/Tokyo")
		h.tick()
		same := h.oneSchedule(id)
		if !same.NextRunAt.Equal(row.NextRunAt) || same.Version <= row.Version {
			t.Fatalf("unchanged timing: %+v (was %+v)", same, row)
		}

		h.putDSL(uid, app, withoutRules(t, []byte(morningRules)))
		h.tick()
		if rows := h.schedules(id); len(rows) != 0 {
			t.Fatalf("schedules after removing the rule: %+v", rows)
		}
		if res := h.tickAt(row.NextRunAt.Add(time.Minute)); res.Scheduled != 0 {
			t.Fatalf("a removed rule ran: %+v", res)
		}
	})
}

// #342 (from #341): an event that gives up refunds what its first attempt
// counted, so a failure does not use the creator's daily runs.
func TestRunnerGaveUpRefunds(t *testing.T) {
	limits := func(l *quota.Limits) { l.AppExecFree, l.CreatorExecFree = 1, 1 }
	stores(t, options{limits: limits}, func(t *testing.T, h *harness) {
		id, uid := "RUN-REFUND"+h.sfx, "uid-refund"+h.sfx
		h.tokens[uid] = []string{"tok-refund"}
		h.sender.Results = map[string]notify.Result{"tok-refund": {Status: 503, Retryable: true}}
		app, project, _ := h.privateTodo(id, uid)
		q := h.r.Queue()
		for attempt := 1; attempt <= 4; attempt++ {
			h.tick()
			if attempt == 1 && len(h.oneEvent(id).Charge) == 0 {
				t.Fatalf("the first attempt kept no charge: %+v", h.oneEvent(id))
			}
			h.clock = h.clock.Add(q.Delay(attempt) + time.Second)
		}
		if ev := h.oneEvent(id); ev.Status != store.OutboxGaveUp {
			t.Fatalf("not given up: %+v", ev)
		}

		// the day's single run is free again
		h.sender.Results = nil
		h.post(uid, app+"/entities/task/records", `{"data":{"title":"二件目","project":"`+project+`"}}`)
		if res := h.tick(); res.Done != 1 || res.Events[quota.OutcomeOK] != 1 {
			t.Fatalf("run after the refund: %+v", res)
		}
	})
}
