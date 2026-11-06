package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// #342: a schedule row round-trips; a newer version is not rolled back; the
// same timing keeps nextRunAt; claiming queues a run once even when the
// row is claimed again at the same due time; delete respects the version;
// a commit stores its events; a retry keeps its charge.
func TestStoreSchedules(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		env, app := "sc"+sfx, "SC"+sfx
		due := time.Date(2026, 11, 7, 22, 2, 0, 0, time.UTC)
		row := Schedule{ID: ScheduleKey(app, "u1", "morning"), AppID: app, UID: "u1", Scope: "private", RuleID: "morning",
			Every: "daily", Time: "07:00", TimeZone: "Asia/Tokyo", Version: 3, Status: ScheduleActive, NextRunAt: due}
		if put, err := s.PutSchedule(ctx, env, row); !put || err != nil {
			t.Fatalf("put: %v %v", put, err)
		}
		older := row
		older.Version, older.Time = 2, "08:00"
		if put, err := s.PutSchedule(ctx, env, older); put || err != nil {
			t.Fatalf("older version put: %v %v", put, err)
		}
		same := row
		same.Version, same.NextRunAt = 4, due.Add(time.Hour)
		if _, err := s.PutSchedule(ctx, env, same); err != nil {
			t.Fatal(err)
		}
		rows, err := s.ListSchedules(ctx, env, app)
		if err != nil || len(rows) != 1 || rows[0].Version != 4 || !rows[0].NextRunAt.Equal(due) || rows[0].Time != "07:00" || rows[0].UID != "u1" {
			t.Fatalf("rows: %+v %v", rows, err)
		}

		// claim twice at the same due time (the row does not move): one event
		stay := func(sc Schedule) (time.Time, *OutboxEvent) {
			return sc.NextRunAt, &OutboxEvent{EventID: sc.ID + "_run", Env: env, Kind: OutboxSchedule, AppID: sc.AppID, UID: sc.UID,
				Scope: sc.Scope, RuleIDs: []string{sc.RuleID}, Version: sc.Version, DueAt: sc.NextRunAt, CreatedAt: due}
		}
		if evs, err := s.ClaimSchedules(ctx, env, due, 10, stay); err != nil || len(evs) != 1 {
			t.Fatalf("first claim: %+v %v", evs, err)
		}
		if evs, err := s.ClaimSchedules(ctx, env, due, 10, stay); err != nil || len(evs) != 0 {
			t.Fatalf("second claim: %+v %v", evs, err)
		}
		if evs, err := s.ClaimSchedules(ctx, env, due.Add(-time.Second), 10, stay); err != nil || len(evs) != 0 {
			t.Fatalf("claim before due: %+v %v", evs, err)
		}
		all, err := s.(interface {
			OutboxEvents(context.Context, string, int) ([]OutboxEvent, error)
		}).OutboxEvents(ctx, env, 0)
		if err != nil || len(all) != 1 || !all[0].DueAt.Equal(due) || all[0].Kind != OutboxSchedule {
			t.Fatalf("outbox: %+v %v", all, err)
		}

		// a retry keeps the charge; a later retry without one keeps it too
		q := s.(OutboxQueue)
		claimed, err := q.ClaimOutbox(ctx, env, due, time.Minute, 10)
		if err != nil || len(claimed) != 1 {
			t.Fatalf("claim outbox: %+v %v", claimed, err)
		}
		charge := json.RawMessage(`{"Parts":[{"Weight":1}]}`)
		if err := q.RetryOutbox(ctx, env, claimed[0].EventID, 1, due, "x", charge); err != nil {
			t.Fatal(err)
		}
		claimed, _ = q.ClaimOutbox(ctx, env, due, time.Minute, 10)
		if err := q.RetryOutbox(ctx, env, claimed[0].EventID, 2, due, "x", nil); err != nil {
			t.Fatal(err)
		}
		claimed, _ = q.ClaimOutbox(ctx, env, due, time.Minute, 10)
		if len(claimed) != 1 || string(claimed[0].Charge) != string(charge) {
			t.Fatalf("charge: %+v", claimed)
		}

		// moving on: the next run, the row's lastDueAt
		next := due.Add(24 * time.Hour)
		move := func(sc Schedule) (time.Time, *OutboxEvent) { return next, nil }
		if _, err := s.ClaimSchedules(ctx, env, due, 10, move); err != nil {
			t.Fatal(err)
		}
		rows, _ = s.ListSchedules(ctx, env, app)
		if len(rows) != 1 || !rows[0].NextRunAt.Equal(next) || !rows[0].LastDueAt.Equal(due) {
			t.Fatalf("after moving: %+v", rows)
		}

		if del, err := s.DeleteSchedule(ctx, env, row.ID, 3); del || err != nil {
			t.Fatalf("delete with an older version: %v %v", del, err)
		}
		if del, err := s.DeleteSchedule(ctx, env, row.ID, 4); !del || err != nil {
			t.Fatalf("delete: %v %v", del, err)
		}

		// runner app state
		key := TenantKey(app, "")
		if err := s.SetAppTimeZone(ctx, env, key, "America/New_York", 5); err != nil {
			t.Fatal(err)
		}
		if err := s.SetAppTimeZone(ctx, env, key, "Asia/Tokyo", 4); err != nil {
			t.Fatal(err)
		}
		opened := due.Truncate(time.Microsecond)
		if err := s.TouchAppOpened(ctx, env, key, opened); err != nil {
			t.Fatal(err)
		}
		a, err := s.RunnerApp(ctx, env, key)
		if err != nil || a.TimeZone != "America/New_York" || a.TimeZoneVersion != 5 || !a.LastOpenedAt.Equal(opened) {
			t.Fatalf("runner app: %+v %v", a, err)
		}
		if a, err := s.RunnerApp(ctx, env, "missing"+sfx); err != nil || !a.LastOpenedAt.IsZero() {
			t.Fatalf("missing runner app: %+v %v", a, err)
		}
	})
}

// #342: Commit.Events are stored with the version, or not at all.
func TestStoreCommitEvents(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		env, app, uid := "ce"+sfx, "CE"+sfx, "ce-user"+sfx
		if _, _, err := s.EnsureApp(ctx, uid, app, json.RawMessage(`{"a":1}`), "x"); err != nil {
			t.Fatal(err)
		}
		ev := OutboxEvent{Env: env, Kind: OutboxRulesChanged, AppID: app, UID: uid, TimeZone: "Asia/Tokyo"}
		v, err := s.CommitVersion(ctx, uid, app, Commit{BaseVersion: 1, DSL: json.RawMessage(`{"a":2}`), Events: []OutboxEvent{ev}})
		if err != nil {
			t.Fatal(err)
		}
		bad := OutboxEvent{Env: env, Kind: OutboxRulesChanged, AppID: app} // no uid
		if _, err := s.CommitVersion(ctx, uid, app, Commit{BaseVersion: 2, DSL: json.RawMessage(`{"a":3}`), Events: []OutboxEvent{bad}}); err == nil {
			t.Fatal("a bad event was accepted")
		}
		if a, _ := s.GetApp(ctx, uid, app); a.CurrentVersion != 2 {
			t.Fatalf("the failed commit was stored: %+v", a)
		}
		all, err := s.(interface {
			OutboxEvents(context.Context, string, int) ([]OutboxEvent, error)
		}).OutboxEvents(ctx, env, 0)
		if err != nil || len(all) != 1 || all[0].Version != v.Version || all[0].Kind != OutboxRulesChanged || all[0].TimeZone != "Asia/Tokyo" {
			t.Fatalf("events: %+v %v", all, err)
		}
	})
}

// #342: keys of different (appId, uid, ruleId) never collide, even when the
// parts joined with "_" read the same.
func TestStoreScheduleKeysDoNotCollide(t *testing.T) {
	if ScheduleKey("a_b", "", "c") == ScheduleKey("a", "", "b_c") ||
		ScheduleKey("a", "b_c", "d") == ScheduleKey("a_b", "c", "d") ||
		ScheduleKey("a", "", "b") == ScheduleKey("a", "b", "") ||
		TenantKey("a_b", "") == TenantKey("a", "b") {
		t.Fatal("keys collide")
	}
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		env := "sk" + sfx
		next := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
		x := Schedule{ID: ScheduleKey("a_b"+sfx, "", "c"), AppID: "a_b" + sfx, Scope: "shared", RuleID: "c",
			Every: "daily", Time: "07:00", TimeZone: "Asia/Tokyo", Version: 1, NextRunAt: next}
		y := Schedule{ID: ScheduleKey("a"+sfx, "", "b_c"), AppID: "a" + sfx, Scope: "shared", RuleID: "b_c",
			Every: "daily", Time: "08:00", TimeZone: "Asia/Tokyo", Version: 1, NextRunAt: next}
		for _, row := range []Schedule{x, y} {
			if put, err := s.PutSchedule(ctx, env, row); !put || err != nil {
				t.Fatalf("put %s: %v %v", row.RuleID, put, err)
			}
		}
		for _, want := range []Schedule{x, y} {
			rows, err := s.ListSchedules(ctx, env, want.AppID)
			if err != nil || len(rows) != 1 || rows[0].ID != want.ID || rows[0].RuleID != want.RuleID || rows[0].Time != want.Time {
				t.Fatalf("rows of %s: %+v %v", want.AppID, rows, err)
			}
		}
		if del, err := s.DeleteSchedule(ctx, env, x.ID, 1); !del || err != nil {
			t.Fatalf("delete: %v %v", del, err)
		}
		if rows, _ := s.ListSchedules(ctx, env, y.AppID); len(rows) != 1 {
			t.Fatalf("deleting one removed the other: %+v", rows)
		}
	})
}
