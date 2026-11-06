package store

import (
	"context"
	"testing"
	"time"
)

// #347: a rule status is overwritten by the next run, keeps the last ok
// time and the day's quota when the next run has none, and is gone after
// its expireAt. Private and shared statuses of one rule are apart.
func TestStoreRuleStatus(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		st := s.(interface {
			RuleStatuses
			RunnerActivity
		})
		env, app := "rs"+sfx, "RS"+sfx
		now := Now()
		shared, mine := RuleStatusID(app, "", "r1"), RuleStatusID(app, "u1", "r1")
		if shared == mine || RuleStatusID(app, "", "r1") != shared {
			t.Fatalf("ids: %s %s", shared, mine)
		}
		ok := RuleStatus{ID: shared, AppID: app, RuleID: "r1", At: now, Outcome: "ok", LastOKAt: now,
			QuotaDay: "2026-11-06", QuotaUsed: 3, QuotaLimit: 100, ExpireAt: now.Add(RuleStatusTTL)}
		if err := st.PutRuleStatus(ctx, env, ok); err != nil {
			t.Fatal(err)
		}
		later := now.Add(time.Minute)
		skip := RuleStatus{ID: shared, AppID: app, RuleID: "r1", At: later, Outcome: "skipped_stopped", Reason: "operator_stop", ExpireAt: later.Add(RuleStatusTTL)}
		if err := st.PutRuleStatus(ctx, env, skip); err != nil {
			t.Fatal(err)
		}
		gone := RuleStatus{ID: RuleStatusID(app, "", "old"), AppID: app, RuleID: "old", At: now, Outcome: "ok", ExpireAt: now.Add(-time.Second)}
		if err := st.PutRuleStatus(ctx, env, gone); err != nil {
			t.Fatal(err)
		}
		got, err := st.GetRuleStatuses(ctx, env, []string{shared, mine, gone.ID})
		if err != nil || len(got) != 1 {
			t.Fatalf("get: %+v %v", got, err)
		}
		g := got[shared]
		if g.Outcome != "skipped_stopped" || g.Reason != "operator_stop" || !g.At.Equal(later) || !g.LastOKAt.Equal(now) ||
			g.QuotaDay != "2026-11-06" || g.QuotaUsed != 3 || g.QuotaLimit != 100 || g.AppID != app || g.RuleID != "r1" {
			t.Fatalf("merged: %+v", g)
		}
		// the reason of the earlier run does not stay
		if err := st.PutRuleStatus(ctx, env, RuleStatus{ID: shared, AppID: app, RuleID: "r1", At: later, Outcome: "ok", ExpireAt: later.Add(RuleStatusTTL)}); err != nil {
			t.Fatal(err)
		}
		got, _ = st.GetRuleStatuses(ctx, env, []string{shared})
		if g := got[shared]; g.Reason != "" || g.Outcome != "ok" {
			t.Fatalf("reason kept: %+v", g)
		}
	})
}

// #347: the creator's view of stops holds only the active ones, with their
// level and deadline (no reason, no setter).
func TestStoreActiveStops(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		ops := s.(RunnerStops)
		env, app := "as"+sfx, "AS"+sfx
		now := Now()
		until := now.Add(time.Hour)
		for _, st := range []Stop{
			{ID: AppStopID(app), Level: StopApp, AppID: app, By: StopByOperator, Reason: "note"},
			{ID: NotifyStopID(app), Level: StopNotify, AppID: app, By: StopByAuto, Until: until},
			{ID: RuleStopID(app, "r"), Level: StopRule, AppID: app, RuleID: "r", By: StopByOperator, LiftedAt: now.Add(-time.Minute)},
		} {
			if err := ops.PutStop(ctx, env, st); err != nil {
				t.Fatal(err)
			}
		}
		got, err := s.ActiveStops(ctx, env, []string{AppStopID(app), NotifyStopID(app), RuleStopID(app, "r"), RuleStopID(app, "x")}, now)
		if err != nil || len(got) != 2 {
			t.Fatalf("active: %+v %v", got, err)
		}
		if g := got[AppStopID(app)]; g.Level != StopApp || !g.Until.IsZero() {
			t.Fatalf("app: %+v", g)
		}
		if g := got[NotifyStopID(app)]; g.Level != StopNotify || !g.Until.Equal(until) {
			t.Fatalf("notify: %+v", g)
		}
		if got, _ := s.ActiveStops(ctx, env, []string{NotifyStopID(app)}, until.Add(time.Second)); len(got) != 0 {
			t.Fatalf("after its deadline: %+v", got)
		}
	})
}
