package runner_test

import (
	"net/http"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/runner"
)

// A due run of a deleted app is skipped_stopped and its
// schedule row is dropped, so it queues no more runs.
func TestScheduleDroppedWhenAppDeleted(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		id, uid := "SCH-DEL"+h.sfx, "uid-sch-del"+h.sfx
		app, row := h.morningApp(id, uid, "Asia/Tokyo")
		if code, out := h.do(uid, http.MethodDelete, app, "", nil); code/100 != 2 {
			t.Fatalf("delete: %d %v", code, out)
		}
		h.tickAt(row.NextRunAt)
		evs := h.scheduleEvents(id)
		if len(evs) != 1 || evs[0].Outcome != runner.OutcomeSkippedStopped {
			t.Fatalf("schedule events: %+v", evs)
		}
		if rows := h.schedules(id); len(rows) != 0 {
			t.Fatalf("schedules left: %+v", rows)
		}
		// later ticks queue nothing
		if res := h.tickAt(row.NextRunAt.AddDate(0, 0, 1)); res.Scheduled != 0 {
			t.Fatalf("next day tick: %+v", res)
		}
	})
}
