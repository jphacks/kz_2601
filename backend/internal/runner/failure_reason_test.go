package runner_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
	"github.com/jphacks/kz_2601/backend/internal/runner/status"
)

// #347: a run that gave up shows the creator a fixed reason word (not the
// error code or its text); the next ok run clears it.
func TestRunnerFailureReasonForCreator(t *testing.T) {
	stores(t, options{}, func(t *testing.T, h *harness) {
		id, uid := "RUN-FAILREASON"+h.sfx, "uid-failreason"+h.sfx
		h.tokens[uid] = []string{"tok-failreason"}
		h.sender.Results = map[string]notify.Result{"tok-failreason": {Status: 503, Retryable: true}}
		app, _, _ := h.privateTodo(id, uid)
		q := h.r.Queue()
		for attempt := 1; attempt <= 4; attempt++ {
			h.tick()
			h.clock = h.clock.Add(q.Delay(attempt) + time.Second)
		}

		code, out := h.do(uid, http.MethodGet, app+"/automations", "", nil)
		rule := out["rules"].([]any)[0].(map[string]any)
		if code != http.StatusOK || rule["result"] != status.ResultFailed || rule["failureReason"] != status.FailureNotifyFailed {
			t.Fatalf("after giving up: %d %v", code, out)
		}
		raw, _ := json.Marshal(out)
		if s := string(raw); strings.Contains(s, "send_failed") || strings.Contains(s, "runner:") || strings.Contains(s, "tok-failreason") {
			t.Fatalf("internal error details leak: %s", s)
		}

		h.sender.Results = nil
		h.post(uid, app+"/entities/task/records", `{"data":{"title":"二つ目"}}`)
		h.tick()
		code, out = h.do(uid, http.MethodGet, app+"/automations", "", nil)
		rule = out["rules"].([]any)[0].(map[string]any)
		if _, has := rule["failureReason"]; code != http.StatusOK || rule["result"] != status.ResultOK || has {
			t.Fatalf("after an ok run: %d %v", code, out)
		}
	})
}
