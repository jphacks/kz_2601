package runner_test

import (
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/feeds"
	"github.com/jphacks/kz_2601/backend/internal/runner"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/runner/schedule"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

func (f *fakeFeeds) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// A busy feed (the 07:00 burst used up the per-minute budget) defers the run
// past feeds.FailTTL with jitter, without using up its attempts: more
// deferrals than MaxAttempts still end in a saved record once the feed answers.
func TestRunnerSaveFeedBusyDefers(t *testing.T) {
	for _, cause := range []error{feeds.ErrBusy, feeds.ErrRecentFailure} {
		t.Run(cause.Error(), func(t *testing.T) {
			f := sampleFeeds()
			stores(t, options{feeds: f}, func(t *testing.T, h *harness) {
				f.setErr(cause) // per store: the memory run clears it at the end
				id, uid := "FEEDBUSY"+h.sfx, "uid-feedbusy"+h.sfx
				h.putDSLTZ(uid, "/v1/apps/"+id, []byte(feedRules), "Asia/Tokyo")
				h.tick()
				due := h.oneSchedule(id).NextRunAt
				now := due
				for i := 1; i <= 5; i++ {
					res := h.tickAt(now)
					if res.Retried != 1 || res.GaveUp != 0 || res.Events[runner.OutcomeDeferred] != 1 {
						t.Fatalf("tick %d: %+v", i, res)
					}
					ev := h.scheduleEvents(id)[0]
					wait := ev.AvailableAt.Sub(now)
					if ev.Attempts != i || ev.Deferrals != i || wait < feeds.FailTTL || wait >= 2*feeds.FailTTL {
						t.Fatalf("deferral %d: attempts %d deferrals %d wait %v", i, ev.Attempts, ev.Deferrals, wait)
					}
					now = ev.AvailableAt
				}
				f.setErr(nil)
				if res := h.tickAt(now); res.Done != 1 {
					t.Fatalf("after the feed answers: %+v", res)
				}
				if n := len(h.records(store.PrivateRecords(uid, id, "plog"))); n != 1 {
					t.Fatalf("plog: %d records", n)
				}
				if evs := h.scheduleEvents(id); len(evs) != 1 || evs[0].Outcome != quota.OutcomeOK {
					t.Fatalf("events: %+v", evs)
				}
			})
		})
	}
}

// Deferring never runs past MaxLate: near the end of the window the run is
// an ordinary (counted) retry, and is skipped_late when it comes back too late.
func TestRunnerSaveFeedDeferBoundedByMaxLate(t *testing.T) {
	f := sampleFeeds()
	f.err = feeds.ErrBusy
	stores(t, options{feeds: f}, func(t *testing.T, h *harness) {
		id, uid := "FEEDLATE"+h.sfx, "uid-feedlate"+h.sfx
		h.putDSLTZ(uid, "/v1/apps/"+id, []byte(feedRules), "Asia/Tokyo")
		h.tick()
		due := h.oneSchedule(id).NextRunAt
		res := h.tickAt(due.Add(schedule.DefaultMaxLate - 30*time.Second))
		if res.Retried != 1 || res.Events[runner.OutcomeDeferred] != 0 {
			t.Fatalf("near max late: %+v", res)
		}
		if ev := h.scheduleEvents(id)[0]; ev.Deferrals != 0 || ev.Attempts != 1 {
			t.Fatalf("counted retry: %+v", ev)
		}
		if res := h.tickAt(due.Add(schedule.DefaultMaxLate + time.Minute)); res.Done != 1 {
			t.Fatalf("late: %+v", res)
		}
		if evs := h.scheduleEvents(id); len(evs) != 1 || evs[0].Outcome != runner.OutcomeSkippedLate {
			t.Fatalf("events: %+v", evs)
		}
	})
}

// A second run of the same day (another event id: the rule's time changed
// and the schedule was rebuilt) does not add a second record of that day.
func TestRunnerSaveFeedOnePerDay(t *testing.T) {
	f := sampleFeeds()
	stores(t, options{feeds: f}, func(t *testing.T, h *harness) {
		id, uid := "FEEDDAY"+h.sfx, "uid-feedday"+h.sfx
		app := "/v1/apps/" + id
		h.putDSLTZ(uid, app, []byte(feedRules), "Asia/Tokyo")
		h.tick()
		due := h.oneSchedule(id).NextRunAt
		if res := h.tickAt(due); res.Done != 1 {
			t.Fatalf("first run: %+v", res)
		}
		h.putDSLTZ(uid, app, []byte(strings.Replace(feedRules, `"07:00"`, `"07:30"`, 1)), "Asia/Tokyo")
		h.tickAt(due)
		later := h.oneSchedule(id).NextRunAt
		if later.Sub(due) != 30*time.Minute {
			t.Fatalf("rescheduled to %v (due %v)", later, due)
		}
		if res := h.tickAt(later); res.Done != 1 {
			t.Fatalf("second run: %+v", res)
		}
		evs := h.scheduleEvents(id)
		if len(evs) != 2 || evs[0].EventID == evs[1].EventID {
			t.Fatalf("two runs: %+v", evs)
		}
		for _, e := range []string{"plog", "wlog"} {
			if n := len(h.records(store.PrivateRecords(uid, id, e))); n != 1 {
				t.Fatalf("%s: %d records for one day", e, n)
			}
		}
	})
}

// A required field the feed leaves empty (the weather without its text)
// skips that day's record and logs why; the run is ok, not retried.
func TestRunnerSaveFeedRequiredEmptySkips(t *testing.T) {
	f := sampleFeeds()
	f.resp["weather"] = &feeds.Result{Provider: "weather", Items: []feeds.Item{{Date: "2026-11-08", Title: "", Max: fp(16)}}}
	rules := strings.Replace(feedRules, `{"id": "weather", "name": "天気", "type": "string"}`,
		`{"id": "weather", "name": "天気", "type": "string", "required": true}`, 1)
	if rules == feedRules {
		t.Fatal("fixture not changed")
	}
	stores(t, options{feeds: f}, func(t *testing.T, h *harness) {
		id, uid := "FEEDREQ"+h.sfx, "uid-feedreq"+h.sfx
		h.putDSLTZ(uid, "/v1/apps/"+id, []byte(rules), "Asia/Tokyo")
		h.tick()
		if res := h.tickAt(h.oneSchedule(id).NextRunAt); res.Done != 1 || res.Retried != 0 {
			t.Fatalf("tick: %+v", res)
		}
		if n := len(h.records(store.PrivateRecords(uid, id, "wlog"))); n != 0 {
			t.Fatalf("wlog: %d records", n)
		}
		if n := len(h.records(store.PrivateRecords(uid, id, "plog"))); n != 1 {
			t.Fatalf("plog: %d records", n)
		}
		if evs := h.scheduleEvents(id); len(evs) != 1 || evs[0].Outcome != quota.OutcomeOK {
			t.Fatalf("events: %+v", evs)
		}
		if out := h.logs.String(); !strings.Contains(out, `"skip_reasons":["feed_required_empty"]`) {
			t.Fatalf("logs: %s", out)
		}
	})
}

// The record's day is the day in Japan (the forecasts' zone), not the
// creator's: 07:00 in Honolulu is the next day in Japan.
func TestRunnerSaveFeedDayInJapan(t *testing.T) {
	f := sampleFeeds()
	stores(t, options{feeds: f}, func(t *testing.T, h *harness) {
		id, uid := "FEEDJST"+h.sfx, "uid-feedjst"+h.sfx
		h.putDSLTZ(uid, "/v1/apps/"+id, []byte(feedRules), "Pacific/Honolulu")
		h.tick()
		due := h.oneSchedule(id).NextRunAt
		if res := h.tickAt(due); res.Done != 1 {
			t.Fatalf("tick: %+v", res)
		}
		jst := due.In(schedule.Location("Asia/Tokyo")).Format("2006-01-02")
		hst := due.In(schedule.Location("Pacific/Honolulu")).Format("2006-01-02")
		plog := h.records(store.PrivateRecords(uid, id, "plog"))
		if jst == hst || len(plog) != 1 || plog[0].Data["day"] != jst {
			t.Fatalf("day: %+v (JST %s, HST %s)", plog, jst, hst)
		}
	})
}
