package runner_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
)

// warnPushes are the creator's daily-limit pushes (the rule's own is titled "追加").
func (h *harness) warnPushes() []notify.Push {
	var out []notify.Push
	for _, p := range h.sender.Pushes() {
		if p.Title != "追加" {
			out = append(out, p)
		}
	}
	return out
}

// #345: the creator gets one push at 80% and one at 100% of the app's daily
// executions, never more on the same day.
func TestRunnerQuotaWarnings(t *testing.T) {
	stores(t, options{limits: func(l *quota.Limits) { l.AppExecPro = 5 }}, func(t *testing.T, h *harness) {
		id, uid := "RUN-WARN"+h.sfx, "uid-warn"+h.sfx
		h.tokens[uid] = []string{"tok-warn"}
		app, _, _ := h.privateTodo(id, uid)
		h.tick()
		var levels []int
		for range 6 { // runs 2..7: 4 = 80%, 6 = stopped
			h.post(uid, app+"/entities/task/records", `{"data":{"title":"次"}}`)
			h.tick()
			levels = append(levels, len(h.warnPushes()))
		}
		if want := []int{0, 0, 1, 1, 2, 2}; !slices.Equal(levels, want) {
			t.Fatalf("warning pushes after each run: %v, want %v", levels, want)
		}
		ws := h.warnPushes()
		if ws[0].Token != "tok-warn" || ws[0].AppID != id || ws[0].AppName != "やること" ||
			ws[0].Body != "『やること』の自動の動きが、今日の上限の 80% に達しました" {
			t.Fatalf("80%% push: %+v", ws[0])
		}
		if ws[1].Body != "『やること』の自動の動きは、今日の上限に達したため、今日は止まっています。明日また動きます" || ws[0].CollapseID == ws[1].CollapseID {
			t.Fatalf("100%% push: %+v", ws[1])
		}
		for _, p := range ws {
			if strings.Contains(p.Title+p.Body, id) || strings.Contains(p.Title+p.Body, uid) {
				t.Fatalf("internal id in a push: %+v", p)
			}
		}
		if out := h.logs.String(); strings.Count(out, `"event":"runner.quota_warn"`) != 2 || strings.Contains(out, uid) {
			t.Fatalf("logs: %s", out)
		}
	})
}

// #345: under a notify pause nothing is sent; the warning is still sent
// once the pause ends the same day.
func TestRunnerQuotaWarningNotifyPaused(t *testing.T) {
	stores(t, options{limits: func(l *quota.Limits) { l.AppExecPro = 5 }}, func(t *testing.T, h *harness) {
		id, uid := "RUN-WARNP"+h.sfx, "uid-warnp"+h.sfx
		h.tokens[uid] = []string{"tok-warnp"}
		h.notifyPaused = true
		app, _, _ := h.privateTodo(id, uid)
		for range 3 {
			h.post(uid, app+"/entities/task/records", `{"data":{"title":"次"}}`)
		}
		h.tick() // 4 runs: 80%
		if n := len(h.sender.Pushes()); n != 0 {
			t.Fatalf("pushes while paused: %d", n)
		}
		h.notifyPaused = false
		h.post(uid, app+"/entities/task/records", `{"data":{"title":"再開"}}`)
		h.tick()
		if ws := h.warnPushes(); len(ws) != 1 || !strings.Contains(ws[0].Body, "80%") {
			t.Fatalf("after the pause: %+v", ws)
		}
	})
}

// failingWarn fails the daily-limit push only (retryable, the worst case).
type failingWarn struct{ notify.FakeSender }

func (f *failingWarn) Send(ctx context.Context, p notify.Push) (notify.Result, error) {
	r, err := f.FakeSender.Send(ctx, p)
	if p.Title != "追加" {
		return notify.Result{Status: 503, Retryable: true}, notify.ErrFakeRejected
	}
	return r, err
}

// #345: a failed warning push does not fail (or retry) the run.
func TestRunnerQuotaWarningSendFailure(t *testing.T) {
	s := &failingWarn{}
	stores(t, options{limits: func(l *quota.Limits) { l.AppExecPro = 1 }, sender: s}, func(t *testing.T, h *harness) {
		id, uid := "RUN-WARNF"+h.sfx, "uid-warnf"+h.sfx
		h.tokens[uid] = []string{"tok-warnf"}
		h.privateTodo(id, uid)
		res := h.tick()
		if res.Done != 1 || res.Retried != 0 {
			t.Fatalf("tick: %+v", res)
		}
		if ev := h.oneEvent(id); ev.Outcome != quota.OutcomeOK {
			t.Fatalf("event: %+v", ev)
		}
		var warns int
		for _, p := range s.Pushes() {
			if p.Title != "追加" && p.AppID == id { // the sender is shared by both subtests
				warns++
			}
		}
		if warns != 1 {
			t.Fatalf("warning attempts: %d", warns)
		}
	})
}
