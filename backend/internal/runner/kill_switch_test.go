package runner_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/runner"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// postCode posts and returns the status and error code ("" on success).
func (h *harness) postCode(uid, path, body string) (int, string, map[string]any) {
	h.t.Helper()
	code, out := h.do(uid, http.MethodPost, path, body, nil)
	e, _ := out["error"].(map[string]any)
	c, _ := e["code"].(string)
	d, _ := e["details"].(map[string]any)
	return code, c, d
}

// #346: a forced app stop refuses the writes that would trigger the app's
// rules (403 app_stopped) and leaves the other writes alone; lifting it
// lets them through again. A plain stop keeps accepting writes.
func TestForcedAppStopRefusesRuleWrites(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		uid := "uid-force" + h.sfx
		h.tokens[uid] = []string{"tok-force"}
		id := "RUN-FORCE" + h.sfx
		app := "/v1/apps/" + id
		h.putDSL(uid, app, []byte(todoRules))
		until := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
		h.putStop(store.Stop{ID: store.AppStopID(id), Level: store.StopApp, AppID: id, Reason: "secret-note", Force: true, Until: until})

		code, c, d := h.postCode(uid, app+"/entities/task/records", `{"data":{"title":"止めた後"}}`)
		if code != http.StatusForbidden || c != httpx.CodeAppStopped || d["level"] != store.StopApp || d["until"] != until.Format(time.RFC3339) {
			t.Fatalf("rule write under a forced stop: %d %s %v", code, c, d)
		}
		if _, out := h.do(uid, http.MethodPost, app+"/entities/task/records", `{"data":{"title":"x"}}`, nil); strings.Contains(fmt.Sprint(out), "secret-note") {
			t.Fatalf("reason leaked: %v", out)
		}
		h.post(uid, app+"/entities/project/records", `{"data":{"name":"家","count":0}}`) // no rule: accepted
		h.tick()
		if evs := h.events(id); len(evs) != 0 {
			t.Fatalf("refused writes queued events: %+v", evs)
		}

		h.lift(store.AppStopID(id))
		h.post(uid, app+"/entities/task/records", `{"data":{"title":"解いた後"}}`)
		h.tick()
		if got := h.outcomes(id); len(got) != 1 || got[0] != "ok" {
			t.Fatalf("after lift: %v", got)
		}
	})
}

// #346: the global stop (moderate stop-all) stops every app's rules without
// a new revision (stop_level=global); forced, it refuses their writes too.
func TestGlobalStop(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		uid := "uid-global" + h.sfx
		h.tokens[uid] = []string{"tok-global"}
		id := "RUN-GLOBAL" + h.sfx
		h.putStop(store.Stop{ID: store.GlobalStopID, Level: store.StopGlobal, Reason: "incident"})
		app, _, _ := h.privateTodo(id, uid)
		h.tick()
		if ev := h.oneEvent(id); ev.Outcome != runner.OutcomeSkippedStopped {
			t.Fatalf("global stop: %+v", ev)
		}
		if out := h.logs.String(); !strings.Contains(out, `"stop_level":"global"`) || !strings.Contains(out, `"reason":"operator_stop"`) {
			t.Fatalf("global stop not logged: %s", out)
		}

		h.putStop(store.Stop{ID: store.GlobalStopID, Level: store.StopGlobal, Reason: "incident", Force: true})
		if code, c, d := h.postCode(uid, app+"/entities/task/records", `{"data":{"title":"二つ目"}}`); code != http.StatusForbidden || c != httpx.CodeAppStopped || d["level"] != store.StopGlobal {
			t.Fatalf("forced global stop: %d %s %v", code, c, d)
		}
		// the creator's screen shows the app as stopped
		if code, out := h.do(uid, http.MethodGet, app+"/automations", "", nil); code != http.StatusOK || out["stopped"] == nil {
			t.Fatalf("automations: %d %v", code, out)
		}

		h.lift(store.GlobalStopID)
		h.post(uid, app+"/entities/task/records", `{"data":{"title":"三つ目"}}`)
		h.tick()
		if n := len(h.records(store.PrivateRecords(uid, id, "log"))); n != 1 {
			t.Fatalf("after lifting the global stop: %d logs", n)
		}
	})
}

// #346: a scheduled run of a stopped app is skipped_stopped and not run;
// a pause with a deadline ends by itself.
func TestScheduleStoppedApp(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		id, uid := "SCH-STOP"+h.sfx, "uid-sch-stop"+h.sfx
		_, row := h.morningApp(id, uid, "Asia/Tokyo")
		due := row.NextRunAt
		h.putStop(store.Stop{ID: store.AppStopID(id), Level: store.StopApp, AppID: id, Reason: "pause", Until: due.Add(time.Hour)})
		h.tickAt(due)
		evs := h.scheduleEvents(id)
		if len(evs) != 1 || evs[0].Outcome != runner.OutcomeSkippedStopped {
			t.Fatalf("scheduled run of a stopped app: %+v", evs)
		}
		if n := len(h.records(store.PrivateRecords(uid, id, "log"))); n != 0 {
			t.Fatalf("stopped schedule ran: %d logs", n)
		}
		// the next day the pause has ended
		next := h.oneSchedule(id).NextRunAt
		h.tickAt(next)
		if n := len(h.records(store.PrivateRecords(uid, id, "log"))); n != 1 {
			t.Fatalf("after the pause ended: %d logs", n)
		}
	})
}
