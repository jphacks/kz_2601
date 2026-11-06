package appconfig

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// #347: only the creator reads GET /automations; a participant who
// imported the app and a stranger get 403. Each rule is listed with its
// description and "none" before it ever ran; an app without rules lists none.
func TestAutomationsCreatorOnly(t *testing.T) {
	t.Setenv("APP_ENV", "")
	storeEnvs(t, func(t *testing.T, e *env, _ store.Store, sfx string) {
		owner, friend, stranger := "auto-owner"+sfx, "auto-friend"+sfx, "auto-stranger"+sfx
		app := "/v1/apps/AUTO-OWN" + sfx
		e.putDSLAt(owner, app, []byte(morningRules), nil)

		r := e.do(owner, http.MethodGet, app+"/automations", nil, nil)
		if r.code != http.StatusOK || r.body["appId"] != "AUTO-OWN"+sfx {
			t.Fatalf("creator: %d %s", r.code, r.raw)
		}
		rules, _ := r.body["rules"].([]any)
		if len(rules) != 1 {
			t.Fatalf("rules: %s", r.raw)
		}
		rule := rules[0].(map[string]any)
		if rule["id"] != "morning" || rule["trigger"] != "schedule" || rule["result"] != "none" ||
			!strings.Contains(rule["description"].(string), "毎日 07:00") || rule["lastRunAt"] != nil {
			t.Fatalf("rule: %v", rule)
		}
		for _, k := range []string{"stopped", "notifyPaused", "quota"} {
			if _, ok := r.body[k]; ok {
				t.Fatalf("%s without a stop / run: %s", k, r.raw)
			}
		}

		// a participant of the creator's shared app
		shared := "/v1/apps/AUTO-SHARED" + sfx
		e.ownApp(owner, shared, sharedKakeibo(t))
		tok := e.do(owner, http.MethodPost, shared+"/share", nil, nil).body["token"]
		if r := e.do(friend, http.MethodPost, "/v1/shares/"+tok.(string)+"/import", nil, nil); r.code != http.StatusCreated || r.body["id"] != "AUTO-SHARED"+sfx || r.body["joined"] != true {
			t.Fatalf("join: %d %s", r.code, r.raw)
		}
		if r := e.do(friend, http.MethodGet, shared+"/automations", nil, nil); r.code != http.StatusForbidden {
			t.Fatalf("participant: %d %s", r.code, r.raw)
		}
		for _, uid := range []string{friend, stranger} {
			if r := e.do(uid, http.MethodGet, app+"/automations", nil, nil); r.code != http.StatusForbidden || strings.Contains(string(r.raw), owner) {
				t.Fatalf("%s: %d %s", uid, r.code, r.raw)
			}
		}
		if r := e.do("", http.MethodGet, app+"/automations", nil, nil); r.code != http.StatusUnauthorized {
			t.Fatalf("no auth: %d", r.code)
		}

		plain := "/v1/apps/AUTO-PLAIN" + sfx
		e.putDSLAt(owner, plain, withoutRules(t, []byte(morningRules)), nil)
		if r := e.do(owner, http.MethodGet, plain+"/automations", nil, nil); r.code != http.StatusOK || string(r.raw) == "" || len(r.body["rules"].([]any)) != 0 {
			t.Fatalf("no rules: %d %s", r.code, r.raw)
		}
	})
}

// #347: the runner's outcomes come out as the fixed words; the newer of the
// shared and the caller's own status wins; the operator's stops say only
// that they are in effect (and until when), never why, who or how many
// reports; today's executions come from the latest run, an earlier day's as 0.
func TestAutomationsResultsAndStops(t *testing.T) {
	t.Setenv("APP_ENV", "auto")
	storeEnvs(t, func(t *testing.T, e *env, st store.Store, sfx string) {
		ctx := t.Context()
		owner := "auto-res-owner" + sfx
		id := "AUTO-RES" + sfx
		app := "/v1/apps/" + id
		e.putDSLAt(owner, app, []byte(twoRules), nil)

		w := st.(store.RuleStatuses)
		now := store.Now()
		put := func(uid, rule, outcome, reason string, at time.Time, day string, used int) {
			t.Helper()
			s := store.RuleStatus{ID: store.RuleStatusID(id, uid, rule), AppID: id, RuleID: rule, At: at, Outcome: outcome,
				Reason: reason, QuotaDay: day, QuotaUsed: used, ExpireAt: at.Add(store.RuleStatusTTL)}
			if day != "" {
				s.QuotaLimit = 100
			}
			if outcome == "ok" {
				s.LastOKAt = at
			}
			if err := w.PutRuleStatus(ctx, "auto", s); err != nil {
				t.Fatal(err)
			}
		}
		today := now.In(ratelimit.JST).Format("2006-01-02")
		put("", "morning", "ok", "", now.Add(-2*time.Hour), today, 7)
		put(owner, "morning", "skipped_quota", "", now.Add(-time.Hour), today, 100) // newer: the caller's own
		put("", "on_log", "skipped_stopped", "operator_stop", now.Add(-3*time.Hour), "", 0)
		put("someone-else", "on_log", "ok", "", now, today, 50) // another user's private status is never read

		ops := st.(store.RunnerStops)
		for _, s := range []store.Stop{
			{ID: store.RuleStopID(id, "on_log"), Level: store.StopRule, AppID: id, RuleID: "on_log", By: store.StopByOperator, Reason: "secret-note-spam"},
			{ID: store.NotifyStopID(id), Level: store.StopNotify, AppID: id, By: store.StopByAuto, Reason: "reports", Until: now.Add(72 * time.Hour)},
		} {
			if err := ops.PutStop(ctx, "auto", s); err != nil {
				t.Fatal(err)
			}
		}

		r := e.do(owner, http.MethodGet, app+"/automations", nil, nil)
		if r.code != http.StatusOK {
			t.Fatalf("get: %d %s", r.code, r.raw)
		}
		for _, leak := range []string{"secret-note-spam", "reports", "operator_stop", "skipped_", "auto\"", "someone-else", `"by"`, "reporters"} {
			if strings.Contains(string(r.raw), leak) {
				t.Fatalf("response leaks %q: %s", leak, r.raw)
			}
		}
		rules := r.body["rules"].([]any)
		m, l := rules[0].(map[string]any), rules[1].(map[string]any)
		if m["id"] != "morning" || m["result"] != "limit" || m["lastRunAt"] == nil || m["lastOkAt"] == nil || m["stopped"] != nil {
			t.Fatalf("morning: %v", m)
		}
		if l["id"] != "on_log" || l["result"] != "stopped" || l["stopped"] == nil || l["lastOkAt"] != nil {
			t.Fatalf("on_log: %v", l)
		}
		if s, _ := l["stopped"].(map[string]any); s["until"] != nil {
			t.Fatalf("an operator stop without a deadline: %v", s)
		}
		if r.body["stopped"] != nil {
			t.Fatalf("app not stopped: %s", r.raw)
		}
		np, _ := r.body["notifyPaused"].(map[string]any)
		if np == nil || np["until"] == nil {
			t.Fatalf("notify pause: %s", r.raw)
		}
		q, _ := r.body["quota"].(map[string]any)
		if q == nil || q["used"] != float64(100) || q["limit"] != float64(100) || q["resetAt"] == nil {
			t.Fatalf("quota: %s", r.raw)
		}

		// a count from an earlier day reads as 0 used today
		put(owner, "morning", "ok", "", now, "2000-01-01", 99)
		r = e.do(owner, http.MethodGet, app+"/automations", nil, nil)
		if q, _ := r.body["quota"].(map[string]any); q == nil || q["used"] != float64(0) || q["limit"] != float64(100) {
			t.Fatalf("quota of an earlier day: %s", r.raw)
		}
		if m := r.body["rules"].([]any)[0].(map[string]any); m["result"] != "ok" {
			t.Fatalf("after an ok run: %v", m)
		}
	})
}

// twoRules: a private app with a daily rule and a record rule.
const twoRules = `{
  "schemaVersion": 1,
  "app": {"name": "朝", "voiceEntry": "floating"},
  "initialScreen": "home",
  "entities": [
    {"id": "log", "name": "記録", "fields": [
      {"id": "day", "name": "日", "type": "date"},
      {"id": "note", "name": "メモ", "type": "string"}
    ]},
    {"id": "memo", "name": "メモ帳", "fields": [{"id": "text", "name": "本文", "type": "string"}]}
  ],
  "screens": [
    {"id": "home", "title": "朝", "components": [{"type": "list", "entity": "log", "item": {"title": "note"}}]}
  ],
  "serverRules": [
    {"id": "morning", "on": {"every": "daily", "time": "07:00"},
     "do": [{"createRecord": {"entity": "log", "values": {"day": {"op": "today", "args": []}, "note": {"op": "concat", "args": ["おは", "よう"]}}}}]},
    {"id": "on_log", "on": {"record": "created", "entity": "memo"},
     "do": [{"notify": {"to": {"kind": "owner"}, "title": "記録", "body": "追加"}}]}
  ]
}`
