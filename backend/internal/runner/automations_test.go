package runner_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/runner"
	"github.com/jphacks/kz_2601/backend/internal/runner/queue"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/runner/status"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// #347: every outcome and reason the runner writes has a word for the
// creator (none falls through to "other").
func TestEveryOutcomeHasACreatorWord(t *testing.T) {
	for _, o := range []string{quota.OutcomeOK, quota.OutcomeSkippedQuota, quota.OutcomeSkippedGlobal, quota.OutcomeSkippedPaused,
		quota.OutcomeSkippedPlan, runner.OutcomeSkippedStale, runner.OutcomeSkippedLate, runner.OutcomeSkippedInactive, queue.OutcomeGaveUp} {
		if got := status.Result(o, "", false); got == status.ResultOther || got == status.ResultNone {
			t.Errorf("outcome %s -> %s", o, got)
		}
	}
	want := map[string]string{
		runner.ReasonOperatorStop: status.ResultStopped, runner.ReasonAppHidden: status.ResultStopped,
		runner.ReasonRuleRemoved: status.ResultChanged, runner.ReasonVersionMissing: status.ResultChanged,
		runner.ReasonAppDeleted: status.ResultUnavailable, runner.ReasonCreatorDeleted: status.ResultUnavailable,
		runner.ReasonNotCreator: status.ResultUnavailable, runner.ReasonBadEvent: status.ResultUnavailable, runner.ReasonAddressed: status.ResultUnavailable,
	}
	for reason, w := range want {
		if got := status.Result(runner.OutcomeSkippedStopped, reason, false); got != w {
			t.Errorf("reason %s -> %s, want %s", reason, got, w)
		}
	}
	if got := status.Result(quota.OutcomeOK, runner.ReasonAppLimit, false); got != status.ResultLimit {
		t.Errorf("ok + %s -> %s", runner.ReasonAppLimit, got)
	}
}

// #347 完了の条件: after a run the creator sees the rule's result, when it
// ran and the app's executions today; after the operator stops the rule the
// creator sees "stopped" and that the stop is in effect, not its note.
func TestRunnerStatusForCreator(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		id, uid := "RUN-STATUS"+h.sfx, "uid-status"+h.sfx
		app, _, _ := h.privateTodo(id, uid)
		h.tick()

		code, out := h.do(uid, http.MethodGet, app+"/automations", "", nil)
		if code != http.StatusOK {
			t.Fatalf("get: %d %v", code, out)
		}
		rule := out["rules"].([]any)[0].(map[string]any)
		if rule["id"] != "on_task" || rule["result"] != status.ResultOK || rule["lastRunAt"] == nil || rule["lastOkAt"] == nil || rule["trigger"] != "record" {
			t.Fatalf("after a run: %v", out)
		}
		q, _ := out["quota"].(map[string]any)
		if q == nil || q["used"] != float64(1) || q["limit"] != float64(quota.DefaultLimits.AppExecPro) {
			t.Fatalf("quota: %v", out)
		}
		if code, _ := h.do("uid-someone"+h.sfx, http.MethodGet, app+"/automations", "", nil); code != http.StatusForbidden {
			t.Fatalf("someone else: %d", code)
		}

		h.putStop(store.Stop{ID: store.RuleStopID(id, "on_task"), Level: store.StopRule, AppID: id, RuleID: "on_task", Reason: "operator-note-x"})
		h.post(uid, app+"/entities/task/records", `{"data":{"title":"二つ目"}}`)
		h.tick()
		code, out = h.do(uid, http.MethodGet, app+"/automations", "", nil)
		rule = out["rules"].([]any)[0].(map[string]any)
		if code != http.StatusOK || rule["result"] != status.ResultStopped || rule["stopped"] == nil || rule["lastOkAt"] == nil {
			t.Fatalf("after the stop: %d %v", code, out)
		}
		if q := out["quota"].(map[string]any); q["used"] != float64(1) {
			t.Fatalf("a stopped run is not counted: %v", q)
		}
		raw, _ := json.Marshal(out)
		if s := string(raw); strings.Contains(s, "operator-note-x") || strings.Contains(s, "operator_stop") || strings.Contains(s, uid) {
			t.Fatalf("stop details leak: %s", s)
		}
	})
}
