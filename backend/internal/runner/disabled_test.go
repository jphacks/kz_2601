package runner_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/runner"
	"github.com/jphacks/kz_2601/backend/internal/runner/status"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// patchDSL applies JSON Patch ops to uid's app (the AI edit / iOS switch path).
func (h *harness) patchDSL(uid, app, ops string) {
	h.t.Helper()
	_, cur := h.do(uid, http.MethodGet, app+"/dsl", "", nil)
	v, _ := cur["version"].(float64)
	hdr := map[string]string{"If-Match": `"` + strconv.Itoa(int(v)) + `"`}
	if code, out := h.do(uid, http.MethodPatch, app+"/dsl", `{"patch":`+ops+`}`, hdr); code != http.StatusCreated {
		h.t.Fatalf("patch dsl %s: %d %v", app, code, out)
	}
}

func (h *harness) automationRule(uid, app string) map[string]any {
	h.t.Helper()
	code, out := h.do(uid, http.MethodGet, app+"/automations", "", nil)
	if code != http.StatusOK {
		h.t.Fatalf("automations: %d %v", code, out)
	}
	return out["rules"].([]any)[0].(map[string]any)
}

// #347: the creator turns a rule off. A run queued before ends as
// skipped_disabled, writes and counts nothing; later writes queue nothing;
// the creator's screen says it is off; turning it on runs it again.
func TestRunnerRuleDisabled(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		id, uid := "RUN-OFF"+h.sfx, "uid-off"+h.sfx
		h.tokens[uid] = []string{"tok-off"}
		app, _, _ := h.privateTodo(id, uid) // queued while on
		h.patchDSL(uid, app, `[{"op":"add","path":"/serverRules/0/enabled","value":false}]`)
		h.tick()

		if got := h.outcomes(id); len(got) != 1 || got[0] != runner.OutcomeSkippedDisabled {
			t.Fatalf("outcomes: %v", got)
		}
		if n := len(h.records(store.PrivateRecords(uid, id, "log"))); n != 0 || len(h.sender.Pushes()) != 0 {
			t.Fatalf("a rule that is off ran: logs %d pushes %d", n, len(h.sender.Pushes()))
		}
		rule := h.automationRule(uid, app)
		if rule["enabled"] != false || rule["result"] != status.ResultDisabled || !strings.HasSuffix(rule["description"].(string), "(オフ)") {
			t.Fatalf("off: %v", rule)
		}
		if _, out := h.do(uid, http.MethodGet, app+"/automations", "", nil); out["quota"] != nil {
			t.Fatalf("a run that is off is not counted: %v", out["quota"])
		}

		h.post(uid, app+"/entities/task/records", `{"data":{"title":"二つ目"}}`)
		if n := len(h.events(id)); n != 1 {
			t.Fatalf("a write while off queued %d events, want the first one only", n)
		}

		h.patchDSL(uid, app, `[{"op":"remove","path":"/serverRules/0/enabled"}]`)
		h.post(uid, app+"/entities/task/records", `{"data":{"title":"三つ目"}}`)
		h.tick()
		if n := len(h.records(store.PrivateRecords(uid, id, "log"))); n != 1 {
			t.Fatalf("back on: logs %d", n)
		}
		if rule := h.automationRule(uid, app); rule["enabled"] != true || rule["result"] != status.ResultOK {
			t.Fatalf("on: %v", rule)
		}
	})
}

// #347: a time-triggered rule that is off has no schedule (no runs queued);
// turning it on builds it again.
func TestScheduleRuleDisabled(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		id, uid := "SCH-OFF"+h.sfx, "uid-sch-off"+h.sfx
		app, _ := h.morningApp(id, uid, "Asia/Tokyo")
		h.patchDSL(uid, app, `[{"op":"add","path":"/serverRules/0/enabled","value":false}]`)
		h.tick()
		if rows := h.schedules(id); len(rows) != 0 {
			t.Fatalf("schedules while off: %+v", rows)
		}
		h.patchDSL(uid, app, `[{"op":"replace","path":"/serverRules/0/enabled","value":true}]`)
		h.tick()
		h.oneSchedule(id)
	})
}

// #347: runs that gave up in a row show as consecutiveFailures (the chat's
// "fix it?" signal); an ok run resets it.
func TestAutomationsConsecutiveFailures(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		id, uid := "RUN-FAIL"+h.sfx, "uid-fail"+h.sfx
		app, _, _ := h.privateTodo(id, uid)
		h.tick()
		if rule := h.automationRule(uid, app); rule["consecutiveFailures"] != nil {
			t.Fatalf("after an ok run: %v", rule)
		}
		sid := store.RuleStatusID(id, uid, "on_task")
		for range 2 {
			h.clock = h.clock.Add(1e9)
			s := store.RuleStatus{ID: sid, AppID: id, RuleID: "on_task", At: h.clock, Outcome: "gave_up", Failed: true, ExpireAt: h.clock.Add(store.RuleStatusTTL)}
			if err := h.st.PutRuleStatus(t.Context(), h.env, s); err != nil {
				t.Fatal(err)
			}
		}
		if rule := h.automationRule(uid, app); rule["consecutiveFailures"] != float64(2) || rule["result"] != status.ResultFailed || rule["lastOkAt"] == nil {
			t.Fatalf("after two failures: %v", rule)
		}
		h.post(uid, app+"/entities/task/records", `{"data":{"title":"二つ目"}}`)
		h.tick()
		if rule := h.automationRule(uid, app); rule["consecutiveFailures"] != nil || rule["result"] != status.ResultOK {
			t.Fatalf("an ok run resets: %v", rule)
		}
	})
}
