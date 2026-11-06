package appconfig

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// morningRules: a private app with one daily rule at 07:00.
const morningRules = `{
  "schemaVersion": 1,
  "app": {"name": "朝", "voiceEntry": "floating"},
  "initialScreen": "home",
  "entities": [
    {"id": "log", "name": "記録", "fields": [
      {"id": "day", "name": "日", "type": "date"},
      {"id": "note", "name": "メモ", "type": "string"}
    ]}
  ],
  "screens": [
    {"id": "home", "title": "朝", "components": [{"type": "list", "entity": "log", "item": {"title": "note"}}]}
  ],
  "serverRules": [
    {"id": "morning", "on": {"every": "daily", "time": "07:00"},
     "do": [{"createRecord": {"entity": "log", "values": {"day": {"op": "today", "args": []}, "note": {"op": "concat", "args": ["おは", "よう"]}}}}]}
  ]
}`

// putDSLAt saves dsl over appPath's current version with hdr.
func (e *env) putDSLAt(uid, appPath string, dsl []byte, hdr map[string]string) int {
	e.t.Helper()
	cur := e.do(uid, http.MethodGet, appPath+"/dsl", nil, nil)
	v, _ := cur.body["version"].(float64)
	h := map[string]string{"If-Match": `"` + strconv.Itoa(int(v)) + `"`}
	for k, x := range hdr {
		h[k] = x
	}
	r := e.do(uid, http.MethodPut, appPath+"/dsl", putBody(dsl), h)
	if r.code != http.StatusCreated {
		e.t.Fatalf("put %s: %d %s", appPath, r.code, r.raw)
	}
	return int(r.body["version"].(float64))
}

// #342: saving a DSL with time-triggered rules queues one rules_changed in
// the version's transaction, with the version and the creator's time zone;
// a DSL that never had one queues nothing; removing them queues one more.
func TestRulesChangedOutbox(t *testing.T) {
	t.Setenv(runnerOutboxEnv, "1")
	t.Setenv("APP_ENV", "")
	storeEnvs(t, func(t *testing.T, e *env, st store.Store, sfx string) {
		id, uid := "SCHED-RC"+sfx, "sched-owner"+sfx
		app := "/v1/apps/" + id
		v := e.putDSLAt(uid, app, []byte(morningRules), map[string]string{TimeZoneHeader: "America/New_York"})
		evs := outboxKinds(t, st, outboxLocalEnv, id, store.OutboxRulesChanged)
		if len(evs) != 1 || evs[0].Version != v || evs[0].UID != uid || evs[0].TimeZone != "America/New_York" {
			t.Fatalf("rules_changed: %+v (version %d)", evs, v)
		}

		// no time zone (or a bad one): Asia/Tokyo
		v = e.putDSLAt(uid, app, []byte(morningRules), map[string]string{TimeZoneHeader: "Mars/Olympus"})
		evs = outboxKinds(t, st, outboxLocalEnv, id, store.OutboxRulesChanged)
		if len(evs) != 2 || evs[1].Version != v && evs[0].Version != v {
			t.Fatalf("second save: %+v", evs)
		}
		for _, ev := range evs {
			if ev.Version == v && ev.TimeZone != "Asia/Tokyo" {
				t.Fatalf("default zone: %+v", ev)
			}
		}

		// removing the rule queues one (the runner deletes the schedule)
		e.putDSLAt(uid, app, withoutRules(t, []byte(morningRules)), nil)
		if n := len(outboxKinds(t, st, outboxLocalEnv, id, store.OutboxRulesChanged)); n != 3 {
			t.Fatalf("after removing the rule: %d rules_changed", n)
		}
		// a DSL that has none and had none queues nothing
		e.putDSLAt(uid, app, withoutRules(t, []byte(morningRules)), nil)
		if n := len(outboxKinds(t, st, outboxLocalEnv, id, store.OutboxRulesChanged)); n != 3 {
			t.Fatalf("no rules before or after: %d rules_changed", n)
		}
	})
}

// #342: with RUNNER_OUTBOX off nothing is queued and nothing is recorded
// when the app is opened (the commit costs what it did before).
func TestRulesChangedOutboxOff(t *testing.T) {
	t.Setenv(runnerOutboxEnv, "")
	t.Setenv("APP_ENV", "")
	e, st := newEnvWith(t)
	id, uid := "SCHED-OFF", "sched-off"
	e.putDSLAt(uid, "/v1/apps/"+id, []byte(morningRules), map[string]string{TimeZoneHeader: "America/New_York"})
	all, err := st.OutboxEvents(t.Context(), outboxLocalEnv, 0)
	if err != nil || len(all) != 0 {
		t.Fatalf("outbox off: %+v %v", all, err)
	}
	if a, _ := st.RunnerApp(t.Context(), outboxLocalEnv, store.TenantKey(id, uid)); !a.LastOpenedAt.IsZero() {
		t.Fatalf("activity written with the outbox off: %+v", a)
	}
}

// #342: GET /dsl records lastOpenedAt at most once a day; a private app is
// keyed by the user, an app with shared data by the appId.
func TestAppActivity(t *testing.T) {
	t.Setenv(runnerOutboxEnv, "1")
	t.Setenv("APP_ENV", "")
	storeEnvs(t, func(t *testing.T, e *env, st store.Store, sfx string) {
		id, uid := "SCHED-ACT"+sfx, "act-user"+sfx
		app := "/v1/apps/" + id
		before := time.Now().Add(-time.Second)
		e.putDSLAt(uid, app, []byte(morningRules), nil) // GET /dsl first
		a, err := st.RunnerApp(t.Context(), outboxLocalEnv, store.TenantKey(id, uid))
		if err != nil || a.LastOpenedAt.Before(before) {
			t.Fatalf("lastOpenedAt after opening: %+v %v", a, err)
		}
		// a second open the same day writes nothing
		old := time.Now().Add(-48 * time.Hour).UTC().Truncate(time.Microsecond)
		if err := st.TouchAppOpened(t.Context(), outboxLocalEnv, store.TenantKey(id, uid), old); err != nil {
			t.Fatal(err)
		}
		e.do(uid, http.MethodGet, app+"/dsl", nil, nil)
		if a, _ := st.RunnerApp(t.Context(), outboxLocalEnv, store.TenantKey(id, uid)); !a.LastOpenedAt.Equal(old) {
			t.Fatalf("written twice in a day: %v (want %v)", a.LastOpenedAt, old)
		}

		// shared data: keyed by the appId (a participant opening counts)
		cid, owner, mom := "SCHED-ACT-C"+sfx, "act-owner"+sfx, "act-mom"+sfx
		capp := circleApp(t, e, cid, owner, mom, circleRules(t))
		e.do(mom, http.MethodGet, capp+"/dsl", nil, nil)
		if a, _ := st.RunnerApp(t.Context(), outboxLocalEnv, store.TenantKey(cid, "")); a.LastOpenedAt.IsZero() {
			t.Fatalf("shared app activity: %+v", a)
		}
		if a, _ := st.RunnerApp(t.Context(), outboxLocalEnv, store.TenantKey(cid, mom)); !a.LastOpenedAt.IsZero() {
			t.Fatalf("shared app keyed by the user: %+v", a)
		}
		// a participant's copy queues no rules_changed (only the creator's DSL counts)
		for _, ev := range outboxKinds(t, st, outboxLocalEnv, cid, store.OutboxRulesChanged) {
			if ev.UID != owner {
				t.Fatalf("participant queued rules_changed: %+v", ev)
			}
		}
	})
}

// #342: a private record trigger carries the writer's (= creator's) zone.
func TestRecordEventTimeZone(t *testing.T) {
	t.Setenv(runnerOutboxEnv, "1")
	t.Setenv("APP_ENV", "")
	e, st := newEnvWith(t)
	id, uid := "SCHED-TZ", "tz-user"
	app := "/v1/apps/" + id
	e.putDSLAt(uid, app, []byte(privateRules), nil)
	r := e.do(uid, http.MethodPost, app+"/entities/memo/records", `{"data":{"n":11,"pic":"https://example.com/a.jpg","note":"a"}}`,
		map[string]string{TimeZoneHeader: "Europe/Paris"})
	if r.code != http.StatusCreated {
		t.Fatalf("post: %d %s", r.code, r.raw)
	}
	evs := outboxFor(t, st, outboxLocalEnv, id)
	if len(evs) != 1 || evs[0].TimeZone != "Europe/Paris" {
		t.Fatalf("record event: %+v", evs)
	}
}
