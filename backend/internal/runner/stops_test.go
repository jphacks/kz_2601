package runner_test

import (
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/runner"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

func (h *harness) putStop(s store.Stop) {
	h.t.Helper()
	if s.By == "" {
		s.By = store.StopByOperator
	}
	if err := h.st.PutStop(h.t.Context(), h.env, s); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) lift(id string) {
	h.t.Helper()
	if ok, err := h.st.LiftStop(h.t.Context(), h.env, id, h.clock); !ok || err != nil {
		h.t.Fatalf("lift %s: %v %v", id, ok, err)
	}
}

func (h *harness) stop(id string) store.Stop {
	h.t.Helper()
	got, err := h.st.GetStops(h.t.Context(), h.env, []string{id})
	if err != nil {
		h.t.Fatal(err)
	}
	return got[id]
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func (h *harness) outcomes(appID string) []string {
	h.t.Helper()
	var out []string
	for _, ev := range h.events(appID) {
		out = append(out, ev.Outcome)
	}
	return out
}

// #346: the operator's app and rule stops finish triggers as skipped_stopped
// (reason operator_stop, stop_level app / rule); what came in while stopped
// does not run after the stop is lifted; the creator's own actions (saving
// the DSL again, posting again) do not lift it.
func TestRunnerOperatorStops(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		uid := "uid-opstop" + h.sfx
		h.tokens[uid] = []string{"tok-opstop"}

		id := "RUN-OPSTOP-APP" + h.sfx
		h.putStop(store.Stop{ID: store.AppStopID(id), Level: store.StopApp, AppID: id, Reason: "test"})
		app, _, _ := h.privateTodo(id, uid)
		h.tick()
		ev := h.oneEvent(id)
		if ev.Status != store.OutboxDone || ev.Outcome != runner.OutcomeSkippedStopped || ev.RuleOutcomes["on_task"] != runner.OutcomeSkippedStopped {
			t.Fatalf("app stop: %+v", ev)
		}
		if out := h.logs.String(); !strings.Contains(out, `"reason":"operator_stop"`) || !strings.Contains(out, `"stop_level":"app"`) {
			t.Fatalf("stop not logged: %s", out)
		}
		// the creator saves again and posts again: still stopped
		h.putDSL(uid, app, []byte(todoRules))
		h.post(uid, app+"/entities/task/records", `{"data":{"title":"二つ目"}}`)
		h.tick()
		if got := h.outcomes(id); len(got) != 2 || got[0] != runner.OutcomeSkippedStopped || got[1] != runner.OutcomeSkippedStopped {
			t.Fatalf("after the creator's save: %v", got)
		}
		// lifted: nothing that came in while stopped runs; a new trigger does
		h.lift(store.AppStopID(id))
		h.tick()
		if n := len(h.records(store.PrivateRecords(uid, id, "log"))); n != 0 || len(h.sender.Pushes()) != 0 {
			t.Fatalf("ran after lift: %d logs, pushes %+v", n, h.sender.Pushes())
		}
		h.post(uid, app+"/entities/task/records", `{"data":{"title":"三つ目"}}`)
		h.tick()
		if n := len(h.records(store.PrivateRecords(uid, id, "log"))); n != 1 || len(h.sender.Pushes()) != 1 {
			t.Fatalf("after lift: %d logs, pushes %+v", n, h.sender.Pushes())
		}

		// rule stop
		rid := "RUN-OPSTOP-RULE" + h.sfx
		h.putStop(store.Stop{ID: store.RuleStopID(rid, "on_task"), Level: store.StopRule, AppID: rid, RuleID: "on_task"})
		rapp, _, _ := h.privateTodo(rid, uid)
		h.tick()
		if ev := h.oneEvent(rid); ev.Outcome != runner.OutcomeSkippedStopped || ev.RuleOutcomes["on_task"] != runner.OutcomeSkippedStopped {
			t.Fatalf("rule stop: %+v", ev)
		}
		if out := h.logs.String(); !strings.Contains(out, `"stop_level":"rule"`) {
			t.Fatalf("rule stop not logged: %s", out)
		}
		h.lift(store.RuleStopID(rid, "on_task"))
		h.post(uid, rapp+"/entities/task/records", `{"data":{"title":"後"}}`)
		h.tick()
		if got := strings.Join(sorted(h.outcomes(rid)), ","); got != quota.OutcomeOK+","+runner.OutcomeSkippedStopped {
			t.Fatalf("after rule lift: %v", got)
		}
		if n := len(h.records(store.PrivateRecords(uid, rid, "log"))); n != 1 {
			t.Fatalf("rule lift: %d logs", n)
		}
	})
}

// #346: the stop ids of different (appId, ruleId) pairs never collide, and
// the store the API handlers get cannot write a stop (the creator has no
// way to lift the operator's).
func TestStopIDsAndNoCreatorPath(t *testing.T) {
	if store.RuleStopID("a_b", "c") == store.RuleStopID("a", "b_c") {
		t.Fatal("rule stop ids collide")
	}
	if store.AppStopID("x") == store.NotifyStopID("x") {
		t.Fatal("app and notify stop ids collide")
	}
	api := reflect.TypeFor[store.Store]()
	for _, m := range []string{"PutStop", "PutStopIfInactive", "LiftStop", "GetStops", "ListStops"} {
		if _, ok := api.MethodByName(m); ok {
			t.Errorf("store.Store exposes %s to the API handlers", m)
		}
	}
}

// #339 / #346: RUNNER_NOTIFY_PAUSED and an app's notify pause let the rule
// run and write but send nothing, and nothing is sent after the pause.
func TestRunnerNotifyPause(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		uid := "uid-npause" + h.sfx
		h.tokens[uid] = []string{"tok-npause"}

		id := "RUN-NPAUSE-ENV" + h.sfx
		h.notifyPaused = true
		_, project, _ := h.privateTodo(id, uid)
		h.tick()
		if ev := h.oneEvent(id); ev.Outcome != quota.OutcomeOK {
			t.Fatalf("env notify pause: %+v", ev)
		}
		if n := len(h.records(store.PrivateRecords(uid, id, "log"))); n != 1 {
			t.Fatalf("writes must run: %d logs", n)
		}
		if c := num(h.record(store.PrivateRecords(uid, id, "project"), project).Data["count"]); c != 1 {
			t.Fatalf("count: %v", c)
		}
		if p := h.sender.Pushes(); len(p) != 0 {
			t.Fatalf("pushes while paused: %+v", p)
		}
		if out := h.logs.String(); !strings.Contains(out, `"notify_paused":"env"`) {
			t.Fatalf("notify pause not logged: %s", out)
		}
		h.notifyPaused = false
		h.tick()
		if p := h.sender.Pushes(); len(p) != 0 {
			t.Fatalf("sent after the pause: %+v", p)
		}

		aid := "RUN-NPAUSE-APP" + h.sfx
		h.tick() // move the clock to now
		h.putStop(store.Stop{ID: store.NotifyStopID(aid), Level: store.StopNotify, AppID: aid, Until: h.clock.Add(time.Minute)})
		app, _, _ := h.privateTodo(aid, uid)
		h.tick()
		if n := len(h.records(store.PrivateRecords(uid, aid, "log"))); n != 1 || len(h.sender.Pushes()) != 0 {
			t.Fatalf("app notify pause: %d logs, pushes %+v", n, h.sender.Pushes())
		}
		if out := h.logs.String(); !strings.Contains(out, `"notify_paused":"app"`) {
			t.Fatalf("app notify pause not logged: %s", out)
		}
		// the pause has a deadline: after it, new triggers notify again
		h.clock = h.clock.Add(2 * time.Minute)
		h.post(uid, app+"/entities/task/records", `{"data":{"title":"期限の後"}}`)
		h.tick()
		if p := h.sender.Pushes(); len(p) != 1 || p[0].Body != "期限の後" {
			t.Fatalf("after the deadline: %+v", p)
		}
	})
}

// #339: reports from 3 different people within 24 hours pause the app's
// notifications (not more: the app stays published); 2 people, or one
// person reporting 3 things, do not. A lifted pause is not set again by the
// same reports.
func TestRunnerAutoNotifyPause(t *testing.T) {
	stores(t, options{pro: fakePro{}, age: oldAccounts{}}, func(t *testing.T, h *harness) {
		owner, mom := "uid-ap-owner"+h.sfx, "uid-ap-mom"+h.sfx
		r1, r2, r3 := "uid-ap-r1"+h.sfx, "uid-ap-r2"+h.sfx, "uid-ap-r3"+h.sfx
		h.pro[owner] = true
		h.tokens[owner] = []string{"tok-ap-owner"}
		id := "RUN-AUTOPAUSE" + h.sfx
		app := h.circle(id, owner, circleRules(t), mom)
		event := h.post(owner, app+"/entities/events/records", `{"data":{"title":"練習"}}`)
		other := h.post(owner, app+"/entities/events/records", `{"data":{"title":"試合"}}`)
		report := func(uid, body string) {
			t.Helper()
			if code, out := h.do(uid, http.MethodPost, "/v1/reports", body, nil); code != http.StatusCreated {
				t.Fatalf("report: %d %v", code, out)
			}
		}
		appReport := `{"kind":"app","appId":"` + id + `","reason":"spam"}`
		recReport := func(rec string) string {
			return `{"kind":"record","appId":"` + id + `","entityId":"events","recordId":"` + rec + `","reason":"spam"}`
		}
		paused := func() bool { return h.stop(store.NotifyStopID(id)).Active(h.clock) }

		// one person, three reports
		report(r1, appReport)
		report(r1, recReport(event))
		report(r1, recReport(other))
		if res := h.tick(); res.AutoPaused != 0 || paused() {
			t.Fatalf("one reporter paused: %+v", res)
		}
		// two people
		report(r2, appReport)
		if res := h.tick(); res.AutoPaused != 0 || paused() {
			t.Fatalf("two reporters paused: %+v", res)
		}
		// three people
		report(r3, recReport(event))
		if res := h.tick(); res.AutoPaused != 1 || !paused() {
			t.Fatalf("three reporters did not pause: %+v", res)
		}
		s := h.stop(store.NotifyStopID(id))
		if s.By != store.StopByAuto || s.Level != store.StopNotify || !s.Until.After(h.clock) || s.Until.Sub(h.clock) > runner.DefaultAutoPauseFor {
			t.Fatalf("auto stop: %+v", s)
		}
		if res := h.tick(); res.AutoPaused != 0 {
			t.Fatalf("paused twice: %+v", res)
		}
		// only notifications: the app stays published, writes run
		if p, err := h.st.GetPublicApp(t.Context(), id); err != nil || p.Hidden {
			t.Fatalf("auto pause hid the app: %+v %v", p, err)
		}
		h.post(mom, app+"/entities/rsvps/records", `{"data":{"event":"`+event+`","answer":"yes"}}`)
		h.tick()
		if c := num(h.record(store.PublicRecords(id, "events"), event).Data["yes_count"]); c != 1 {
			t.Fatalf("yes_count: %v", c)
		}
		if p := h.sender.Pushes(); len(p) != 0 {
			t.Fatalf("pushes while paused: %+v", p)
		}
		// the operator lifts it: the same reports do not pause it again
		h.lift(store.NotifyStopID(id))
		if res := h.tick(); res.AutoPaused != 0 || paused() {
			t.Fatalf("re-paused on old reports: %+v", res)
		}
	})
}
