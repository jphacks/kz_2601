package store

import (
	"context"
	"testing"
	"time"
)

// #346: every set and lift (operator or automatic) is kept in the stop log
// with who / why / force / until, newest first; a lift that finds no active
// stop logs nothing; force and operator round-trip on the stop.
func TestStoreStopLog(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		st := s.(RunnerStops)
		env, app, other := "stoplog"+sfx, "LOG"+sfx, "LOG2"+sfx
		now := Now().Truncate(time.Millisecond)
		until := now.Add(24 * time.Hour)
		set := Stop{ID: AppStopID(app), Level: StopApp, AppID: app, By: StopByOperator, Operator: "alice",
			Reason: "spam", Force: true, Until: until, CreatedAt: now}
		if err := st.PutStop(ctx, env, set); err != nil {
			t.Fatal(err)
		}
		got, err := st.GetStops(ctx, env, []string{set.ID})
		if g := got[set.ID]; err != nil || !g.Force || g.Operator != "alice" {
			t.Fatalf("stop: %+v %v", g, err)
		}
		auto := Stop{ID: NotifyStopID(other), Level: StopNotify, AppID: other, By: StopByAuto, Reason: "reports",
			Until: now.Add(time.Hour), CreatedAt: now.Add(time.Second)}
		if put, err := st.PutStopIfInactive(ctx, env, auto, now); !put || err != nil {
			t.Fatalf("auto: %v %v", put, err)
		}
		if put, _ := st.PutStopIfInactive(ctx, env, auto, now); put {
			t.Fatal("auto over an active stop")
		}
		if ok, err := st.LiftStop(ctx, env, set.ID, now.Add(2*time.Second), "bob"); !ok || err != nil {
			t.Fatalf("lift: %v %v", ok, err)
		}
		if ok, _ := st.LiftStop(ctx, env, set.ID, now.Add(3*time.Second), "bob"); ok {
			t.Fatal("lifted twice")
		}

		log, err := st.StopLog(ctx, env, "", 0)
		if err != nil || len(log) != 3 {
			t.Fatalf("log: %+v %v", log, err)
		}
		lift, a, first := log[0], log[1], log[2]
		if lift.Action != StopActionLift || lift.StopID != set.ID || lift.Operator != "bob" || lift.By != StopByOperator || !lift.At.Equal(now.Add(2*time.Second)) {
			t.Fatalf("lift entry: %+v", lift)
		}
		if a.Action != StopActionSet || a.By != StopByAuto || a.AppID != other || a.Operator != "" {
			t.Fatalf("auto entry: %+v", a)
		}
		if first.Action != StopActionSet || first.Operator != "alice" || first.Reason != "spam" || !first.Force || !first.Until.Equal(until) || !first.At.Equal(now) || first.ID == "" {
			t.Fatalf("set entry: %+v", first)
		}
		mine, err := st.StopLog(ctx, env, app, 1)
		if err != nil || len(mine) != 1 || mine[0].Action != StopActionLift {
			t.Fatalf("app log: %+v %v", mine, err)
		}
		if l, _ := st.StopLog(ctx, env, app, 0); len(l) != 2 {
			t.Fatalf("app log all: %+v", l)
		}
	})
}
