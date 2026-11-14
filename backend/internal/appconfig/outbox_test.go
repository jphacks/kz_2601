package appconfig

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// circleRules is the #340 example: shared events / rsvps / messages with
// record rules (出欠 → 予定を作った人、新着メッセージ → 参加者全員、状態の変更).
func circleRules(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../pkg/dsl/testdata/server_rules.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// withoutRules drops serverRules from raw.
func withoutRules(t *testing.T, raw []byte) []byte {
	t.Helper()
	var a map[string]any
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatal(err)
	}
	delete(a, "serverRules")
	out, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// privateRules has private records with a created rule (reads n and pic)
// and a single settings entity with an updated rule.
const privateRules = `{
  "schemaVersion": 1,
  "app": {"name": "メモ", "voiceEntry": "floating"},
  "initialScreen": "home",
  "entities": [
    {"id": "memo", "name": "メモ", "fields": [
      {"id": "n", "name": "数", "type": "integer"},
      {"id": "pic", "name": "写真", "type": "photo"},
      {"id": "note", "name": "メモ", "type": "string"}
    ]},
    {"id": "settings", "name": "設定", "single": true, "fields": [
      {"id": "goal", "name": "目標", "type": "integer"},
      {"id": "label", "name": "名前", "type": "string"}
    ]}
  ],
  "screens": [
    {"id": "home", "title": "メモ", "components": [{"type": "list", "entity": "memo", "item": {"title": "note"}}]}
  ],
  "serverRules": [
    {"id": "big_memo", "on": {"record": "created", "entity": "memo"},
     "if": {"op": "and", "args": [{"op": "gt", "args": [{"ref": "n"}, 10]}, {"op": "neq", "args": [{"ref": "pic"}, ""]}]},
     "do": [{"notify": {"to": {"kind": "owner"}, "title": "メモ", "body": "{n} 件"}}]},
    {"id": "goal_changed", "on": {"record": "updated", "entity": "settings", "fields": ["goal"]},
     "do": [{"notify": {"to": {"kind": "owner"}, "title": "目標", "body": "{goal} になりました"}}]}
  ]
}`

type outboxReader interface {
	OutboxEvents(ctx context.Context, env string, limit int) ([]store.OutboxEvent, error)
}

// outboxFor returns env's record-trigger events of appID.
func outboxFor(t *testing.T, st store.Store, env, appID string) []store.OutboxEvent {
	t.Helper()
	return outboxKinds(t, st, env, appID, store.OutboxRecordCreated, store.OutboxRecordUpdated)
}

// outboxKinds returns env's events of appID of the given kinds.
func outboxKinds(t *testing.T, st store.Store, env, appID string, kinds ...string) []store.OutboxEvent {
	t.Helper()
	all, err := st.(outboxReader).OutboxEvents(t.Context(), env, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []store.OutboxEvent
	for _, ev := range all {
		if ev.AppID == appID && slices.Contains(kinds, ev.Kind) {
			out = append(out, ev)
		}
	}
	return out
}

func (e *env) post(uid, path, body string) string {
	e.t.Helper()
	r := e.do(uid, http.MethodPost, path, body, nil)
	if r.code != http.StatusCreated && r.code != http.StatusOK {
		e.t.Fatalf("post %s by %s: %d %s", path, uid, r.code, r.raw)
	}
	return r.body["id"].(string)
}

func (e *env) patch(uid, path, body string) {
	e.t.Helper()
	if r := e.do(uid, http.MethodPatch, path, body, nil); r.code != http.StatusOK {
		e.t.Fatalf("patch %s by %s: %d %s", path, uid, r.code, r.raw)
	}
}

// circleApp makes owner's shared circle app (dsl) and lets a participant join.
func circleApp(t *testing.T, e *env, id, owner, member string, dsl []byte) string {
	t.Helper()
	app := "/v1/apps/" + id
	e.ownApp(owner, app, dsl)
	tok := e.share(owner, app)
	if r := e.do(member, http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil); r.code != http.StatusCreated {
		t.Fatalf("join: %d %s", r.code, r.raw)
	}
	return app
}

// #341: a write whose rule matches queues one event with the creator's
// published version, the matching rule ids and only the fields they read.
func TestOutboxSharedRules(t *testing.T) {
	t.Setenv(runnerOutboxEnv, "1")
	t.Setenv("APP_ENV", "")
	storeEnvs(t, func(t *testing.T, e *env, st store.Store, sfx string) {
		id := "OUTBOX-CIRCLE" + sfx
		owner, mom := "owner"+sfx, "mom"+sfx
		app := circleApp(t, e, id, owner, mom, circleRules(t))
		pub, err := st.GetPublicApp(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}

		// no created rule on events: nothing queued
		evID := e.post(owner, app+"/entities/events/records", `{"data":{"title":"練習","status":"予定","date":"2026-11-10"}}`)
		if evs := outboxFor(t, st, outboxLocalEnv, id); len(evs) != 0 {
			t.Fatalf("events created without a rule: %+v", evs)
		}

		rsvp := e.post(mom, app+"/entities/rsvps/records", `{"data":{"event":"`+evID+`","answer":"yes"}}`)
		evs := outboxFor(t, st, outboxLocalEnv, id)
		if len(evs) != 1 {
			t.Fatalf("rsvp: want 1 event, got %+v", evs)
		}
		ev := evs[0]
		if ev.Kind != store.OutboxRecordCreated || ev.Entity != "rsvps" || ev.RecordID != rsvp || ev.Scope != "shared" || ev.UID != "" ||
			ev.Version != pub.Version || !reflect.DeepEqual(ev.RuleIDs, []string{"notify_rsvp"}) ||
			!reflect.DeepEqual(ev.Data, map[string]any{"answer": "yes", "event": evID}) || ev.Prev != nil ||
			ev.ActorUID != mom || ev.ActorAnonymous || ev.Author != mom || ev.Status != store.OutboxPending {
			t.Fatalf("rsvp event: %+v (published version %d)", ev, pub.Version)
		}

		// fields: only a status change queues status_changed
		recPath := app + "/entities/events/records/" + evID
		e.patch(owner, recPath, `{"data":{"title":"練習2"}}`)
		e.patch(owner, recPath, `{"data":{"status":"予定"}}`) // same value: not a change
		if n := len(outboxFor(t, st, outboxLocalEnv, id)); n != 1 {
			t.Fatalf("update of other fields queued: %d events", n)
		}
		e.patch(owner, recPath, `{"data":{"status":"中止"}}`)
		evs = outboxFor(t, st, outboxLocalEnv, id)
		var upd *store.OutboxEvent
		for i := range evs {
			if evs[i].Kind == store.OutboxRecordUpdated {
				upd = &evs[i]
			}
		}
		if len(evs) != 2 || upd == nil || !reflect.DeepEqual(upd.RuleIDs, []string{"status_changed"}) || !reflect.DeepEqual(upd.ChangedFields, []string{"status"}) ||
			upd.Data["status"] != "中止" || upd.Prev["status"] != "予定" || upd.Data["title"] != "練習2" || upd.Data["date"] != nil || upd.Version != pub.Version {
			t.Fatalf("status event: %+v", upd)
		}
	})
}

// #341: off by default, and a DSL with no rule queues nothing.
func TestOutboxOffOrNoRules(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Run("flag off", func(t *testing.T) {
		t.Setenv(runnerOutboxEnv, "")
		storeEnvs(t, func(t *testing.T, e *env, st store.Store, sfx string) {
			id := "OUTBOX-OFF" + sfx
			app := circleApp(t, e, id, "owner"+sfx, "mom"+sfx, circleRules(t))
			e.post("mom"+sfx, app+"/entities/messages/records", `{"data":{"body":"こんにちは"}}`)
			if evs := outboxFor(t, st, outboxLocalEnv, id); len(evs) != 0 {
				t.Fatalf("flag off queued: %+v", evs)
			}
		})
	})
	t.Run("no rules", func(t *testing.T) {
		t.Setenv(runnerOutboxEnv, "1")
		storeEnvs(t, func(t *testing.T, e *env, st store.Store, sfx string) {
			id := "OUTBOX-NORULES" + sfx
			app := circleApp(t, e, id, "owner"+sfx, "mom"+sfx, withoutRules(t, circleRules(t)))
			e.post("mom"+sfx, app+"/entities/messages/records", `{"data":{"body":"こんにちは"}}`)
			if evs := outboxFor(t, st, outboxLocalEnv, id); len(evs) != 0 {
				t.Fatalf("no rules queued: %+v", evs)
			}
		})
	})
}

// #337: rules a participant writes into their own copy never fire on the
// shared entities; only the creator's published DSL counts.
func TestOutboxIgnoresParticipantCopy(t *testing.T) {
	t.Setenv(runnerOutboxEnv, "1")
	t.Setenv("APP_ENV", "")
	storeEnvs(t, func(t *testing.T, e *env, st store.Store, sfx string) {
		id := "OUTBOX-COPY" + sfx
		owner, mom := "owner"+sfx, "mom"+sfx
		app := circleApp(t, e, id, owner, mom, withoutRules(t, circleRules(t)))
		e.editCopy(mom, app, circleRules(t))
		e.post(mom, app+"/entities/messages/records", `{"data":{"body":"こんにちは"}}`)
		e.post(owner, app+"/entities/messages/records", `{"data":{"body":"やあ"}}`)
		if evs := outboxFor(t, st, outboxLocalEnv, id); len(evs) != 0 {
			t.Fatalf("participant's copy fired rules: %+v", evs)
		}
	})
}

// #337: APP_ENV names the queue, so stg events never show up under prod.
func TestOutboxEnvSeparation(t *testing.T) {
	t.Setenv(runnerOutboxEnv, "1")
	t.Setenv("APP_ENV", "stg")
	storeEnvs(t, func(t *testing.T, e *env, st store.Store, sfx string) {
		id := "OUTBOX-ENV" + sfx
		app := circleApp(t, e, id, "owner"+sfx, "mom"+sfx, circleRules(t))
		e.post("mom"+sfx, app+"/entities/messages/records", `{"data":{"body":"こんにちは"}}`)
		if evs := outboxFor(t, st, "stg", id); len(evs) != 1 || !reflect.DeepEqual(evs[0].RuleIDs, []string{"new_message"}) {
			t.Fatalf("stg queue: %+v", evs)
		}
		for _, other := range []string{"prod", outboxLocalEnv} {
			if evs := outboxFor(t, st, other, id); len(evs) != 0 {
				t.Fatalf("stg event under %s: %+v", other, evs)
			}
		}
	})
}

// #341: private records follow the user's own DSL and version; heavy values
// (photo) stay out of the event; a single entity's overwrite is an update.
func TestOutboxPrivateRules(t *testing.T) {
	t.Setenv(runnerOutboxEnv, "1")
	t.Setenv("APP_ENV", "")
	storeEnvs(t, func(t *testing.T, e *env, st store.Store, sfx string) {
		id := "OUTBOX-PRIVATE" + sfx
		app := "/v1/apps/" + id
		u := "anon-u" + sfx // dev uids starting with anon are anonymous accounts
		e.ownApp(u, app, []byte(privateRules))
		cur, err := st.GetApp(t.Context(), u, id)
		if err != nil {
			t.Fatal(err)
		}
		rec := e.post(u, app+"/entities/memo/records", `{"data":{"n":12,"pic":"https://example.com/p.jpg","note":"x"}}`)
		evs := outboxFor(t, st, outboxLocalEnv, id)
		if len(evs) != 1 {
			t.Fatalf("memo: %+v", evs)
		}
		ev := evs[0]
		if ev.Scope != "private" || ev.UID != u || ev.RecordID != rec || ev.Version != cur.CurrentVersion || !reflect.DeepEqual(ev.RuleIDs, []string{"big_memo"}) ||
			len(ev.Data) != 1 || ev.Data["n"] == nil || !ev.ActorAnonymous || ev.Author != "" {
			t.Fatalf("memo event: %+v (version %d)", ev, cur.CurrentVersion)
		}

		sets := app + "/entities/settings/records"
		e.post(u, sets, `{"data":{"goal":5,"label":"a"}}`) // created: no created rule on settings
		e.post(u, sets, `{"data":{"goal":5,"label":"b"}}`) // goal unchanged
		e.post(u, sets, `{"data":{"goal":8,"label":"b"}}`)
		evs = outboxFor(t, st, outboxLocalEnv, id)
		if len(evs) != 2 {
			t.Fatalf("settings: %+v", evs)
		}
		var upd store.OutboxEvent
		for _, ev := range evs {
			if ev.Kind == store.OutboxRecordUpdated {
				upd = ev
			}
		}
		if !reflect.DeepEqual(upd.RuleIDs, []string{"goal_changed"}) || !reflect.DeepEqual(upd.ChangedFields, []string{"goal"}) || len(upd.Data) != 1 || len(upd.Prev) != 1 {
			t.Fatalf("settings event: %+v", upd)
		}
	})
}
