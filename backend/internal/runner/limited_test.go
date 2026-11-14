package runner_test

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/runner/status"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// #345: when the app's daily writes cut a run, the creator sees "limit",
// not "ok" (the run itself still finishes ok).
func TestRunnerAppWritesLimitShowsLimit(t *testing.T) {
	stores(t, options{limits: func(l *quota.Limits) { l.AppWritesPro = 1 }}, func(t *testing.T, h *harness) {
		id, uid := "RUN-WLIM"+h.sfx, "uid-wlim"+h.sfx
		app, _, _ := h.privateTodo(id, uid)
		h.tick()
		if ev := h.oneEvent(id); ev.Outcome != quota.OutcomeOK {
			t.Fatalf("event: %+v", ev)
		}
		if rule := h.automationRule(uid, app); rule["result"] != status.ResultLimit || rule["lastOkAt"] == nil {
			t.Fatalf("rule: %v", rule)
		}
	})
}

// #345: the app's daily notification recipients cut a run: "limit".
func TestRunnerAppNotifyLimitShowsLimit(t *testing.T) {
	stores(t, options{limits: func(l *quota.Limits) { l.AppNotifyPro = 1 }}, func(t *testing.T, h *harness) {
		id, uid := "RUN-NLIM"+h.sfx, "uid-nlim"+h.sfx
		h.tokens[uid] = []string{"tok-nlim"}
		app, _, _ := h.privateTodo(id, uid)
		h.tick()
		if rule := h.automationRule(uid, app); rule["result"] != status.ResultOK {
			t.Fatalf("first run: %v", rule)
		}
		h.post(uid, app+"/entities/task/records", `{"data":{"title":"二つ目"}}`)
		h.tick()
		if n := len(h.sender.Pushes()); n != 1 {
			t.Fatalf("pushes: %d", n)
		}
		if rule := h.automationRule(uid, app); rule["result"] != status.ResultLimit {
			t.Fatalf("second run: %v", rule)
		}
		if n := len(h.records(store.PrivateRecords(uid, id, "log"))); n != 2 {
			t.Fatalf("writes still go on: %d", n)
		}
	})
}
