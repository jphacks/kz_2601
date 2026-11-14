package runner_test

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/runner"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// #346 完了の条件 (stopped within a minute of the operation): a trigger
// already waiting in the outbox when the operator stops the app, the rule
// or everything is finished as skipped_stopped by the next tick (runner-tick
// runs every minute) and writes / sends nothing, then or after the lift.
func TestStopCatchesQueuedTriggers(t *testing.T) {
	cases := []struct {
		name string
		stop func(appID string) store.Stop
	}{
		{"app", func(id string) store.Stop {
			return store.Stop{ID: store.AppStopID(id), Level: store.StopApp, AppID: id, Reason: "test"}
		}},
		{"rule", func(id string) store.Stop {
			return store.Stop{ID: store.RuleStopID(id, "on_task"), Level: store.StopRule, AppID: id, RuleID: "on_task", Reason: "test"}
		}},
		{"global", func(string) store.Stop {
			return store.Stop{ID: store.GlobalStopID, Level: store.StopGlobal, Reason: "test"}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stores(t, options{}, func(t *testing.T, h *harness) {
				id, uid := "RUN-QSTOP-"+c.name+h.sfx, "uid-qstop-"+c.name+h.sfx
				h.tokens[uid] = []string{"tok-qstop"}
				h.privateTodo(id, uid) // queued, not run yet
				s := c.stop(id)
				h.putStop(s)
				h.tick()
				ev := h.oneEvent(id)
				if ev.Status != store.OutboxDone || ev.RuleOutcomes["on_task"] != runner.OutcomeSkippedStopped {
					t.Fatalf("queued trigger after the stop: %+v", ev)
				}
				h.lift(s.ID)
				h.tick()
				if n := len(h.records(store.PrivateRecords(uid, id, "log"))); n != 0 || len(h.sender.Pushes()) != 0 {
					t.Fatalf("queued trigger ran: %d logs, pushes %+v", n, h.sender.Pushes())
				}
			})
		})
	}
}
