package store

import (
	"context"
	"testing"
	"time"
)

// #346: a stop round-trips; the automatic pause does not replace an
// active stop; a lifted stop keeps its document and is inactive; a stop
// with a deadline ends by itself; reports are read by time.
func TestStoreStops(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		st := s.(RunnerStops)
		env, app := "stop"+sfx, "STOP"+sfx
		now := Now()
		op := Stop{ID: AppStopID(app), Level: StopApp, AppID: app, By: StopByOperator, Reason: "spam", CreatedAt: now}
		if err := st.PutStop(ctx, env, op); err != nil {
			t.Fatal(err)
		}
		auto := Stop{ID: NotifyStopID(app), Level: StopNotify, AppID: app, By: StopByAuto, Until: now.Add(time.Hour), CreatedAt: now}
		if put, err := st.PutStopIfInactive(ctx, env, auto, now); !put || err != nil {
			t.Fatalf("auto put: %v %v", put, err)
		}
		later := auto
		later.Until = now.Add(48 * time.Hour)
		if put, err := st.PutStopIfInactive(ctx, env, later, now); put || err != nil {
			t.Fatalf("auto put over an active stop: %v %v", put, err)
		}
		rule := RuleStopID(app, "r1")
		got, err := st.GetStops(ctx, env, []string{op.ID, auto.ID, rule})
		if err != nil || len(got) != 2 {
			t.Fatalf("get: %+v %v", got, err)
		}
		if g := got[op.ID]; g.Reason != "spam" || g.By != StopByOperator || !g.Until.IsZero() || !g.Active(now.Add(1000*time.Hour)) {
			t.Fatalf("operator stop: %+v", g)
		}
		if g := got[auto.ID]; !g.Until.Equal(auto.Until) || !g.Active(now) || g.Active(now.Add(2*time.Hour)) || !g.Ended(now.Add(2*time.Hour)).Equal(auto.Until) {
			t.Fatalf("auto stop: %+v", g)
		}

		at := now.Add(time.Minute)
		if ok, err := st.LiftStop(ctx, env, op.ID, at); !ok || err != nil {
			t.Fatalf("lift: %v %v", ok, err)
		}
		if ok, err := st.LiftStop(ctx, env, op.ID, at); ok || err != nil {
			t.Fatalf("lift twice: %v %v", ok, err)
		}
		if ok, err := st.LiftStop(ctx, env, rule, at); ok || err != nil {
			t.Fatalf("lift missing: %v %v", ok, err)
		}
		all, err := st.ListStops(ctx, env)
		if err != nil || len(all) != 2 {
			t.Fatalf("list: %+v %v", all, err)
		}
		for _, g := range all {
			if g.ID == op.ID && (g.Active(at) || !g.LiftedAt.Equal(at) || !g.Ended(at).Equal(at)) {
				t.Fatalf("lifted: %+v", g)
			}
		}
		// lifted: the automatic pause may write it again
		re := Stop{ID: op.ID, Level: StopApp, AppID: app, By: StopByAuto, CreatedAt: at}
		if put, err := st.PutStopIfInactive(ctx, env, re, at); !put || err != nil {
			t.Fatalf("put over a lifted stop: %v %v", put, err)
		}

		// reports by time
		old := Report{ID: "rpt_old" + sfx, ReporterUID: "a" + sfx, Kind: ReportApp, TargetID: app, Reason: "spam", Status: "open", CreatedAt: now.Add(-25 * time.Hour)}
		fresh := Report{ID: "rpt_new" + sfx, ReporterUID: "b" + sfx, Kind: ReportRecord, TargetID: app + "/e/r", Reason: "spam", Status: "open", CreatedAt: now}
		for _, r := range []Report{old, fresh} {
			if _, err := s.AddReport(ctx, r, "d"+r.ID, 0); err != nil {
				t.Fatal(err)
			}
		}
		reps, err := st.ReportsSince(ctx, now.Add(-24*time.Hour), 0)
		if err != nil {
			t.Fatal(err)
		}
		found := map[string]bool{}
		for _, r := range reps {
			found[r.ID] = true
		}
		if !found[fresh.ID] || found[old.ID] {
			t.Fatalf("reports since: %+v", reps)
		}
		if ReportedAppID(fresh) != app || ReportedAppID(old) != app || ReportedAppID(Report{Kind: ReportListing, TargetID: "x"}) != "" {
			t.Fatal("ReportedAppID")
		}
	})
}
