package ai

import (
	"encoding/json"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// On-device first: a reminder the app's own notifications already give the
// creator (a fixed time, a record's date) must not become a server rule —
// that adds Pro (gatePaid 402) to a free app. init() rewrites the base
// prompt's "書かない" line and its private time example (server_rules_pass.go's
// init runs first: files initialize in name order). dropOnDeviceDuplicates
// removes what the model still writes for the exact time-trigger case.
//
// Condition notifications (notifications.when) are left out on purpose: the
// demo's Pro card (C1) asks for exactly that as a server rule.

// serverRulesOnDeviceLines: base line -> replacement.
var serverRulesOnDeviceLines = [][2]string{
	{
		`- 書かない: 画面の計算(stat・computed)で済む集計、app.notifications に既にある同じ時刻の通知(重ねない)。`,
		`- 書かない: 画面の計算(stat・computed)で済む集計。**この端末の通知で済むもの**: 現在の DSL の app.notifications が同じ用件をもう受け持っている時(決まった時刻の通知 = time・repeat、レコードの日付の通知 = entity・field・offsetMinutes。「毎日 21 時に記録を忘れないよう通知して」「予定の前の日に通知して」)は、作った人(owner)に同じ知らせを送る serverRules を作らない(重ねない。無料のアプリが Pro になる)。serverRules はこの端末だけではできない時に書く: ほかの人(参加者・書いた人・宛先)に届ける、ほかの人の記録をきっかけに届ける、記録を数える・書く・まとめる。`,
	},
	{
		`- 「毎朝 7 時に水分の記録を促す」(private):`,
		`- 「毎朝 7 時に水分の記録を促す」(private・app.notifications に 07:00 の通知が無い時だけ。あれば書かない):`,
	},
}

func init() {
	for _, l := range serverRulesOnDeviceLines {
		serverRulesPrompt = strings.Replace(serverRulesPrompt, l[0], l[1], 1)
	}
}

// dropOnDeviceDuplicates removes the rules added in this turn (id not in
// current) that only repeat a local notification added or changed in this
// turn (not in before as is) to the creator: a daily / weekly time trigger
// with no row, condition or read, whose actions are all notify to owner, at
// the same time (and weekday) as a time-based app.notifications entry. A
// notification the app already had is not this request, so a rule beside it
// (「21:05 に薬も」) is kept. before is nil on create (all new). It returns
// the kept rules and the dropped ids.
func dropOnDeviceDuplicates(current, before []byte, app *dsl.App, rules []map[string]any) ([]map[string]any, []string) {
	old := map[string]bool{}
	if cur, err := dsl.Parse(current); err == nil {
		for i := range cur.ServerRules {
			old[cur.ServerRules[i].ID] = true
		}
	}
	fresh := newNotifications(before, app.App.Notifications)
	drop := map[string]bool{}
	var ids []string
	for i := range app.ServerRules {
		r := &app.ServerRules[i]
		if !old[r.ID] && repeatsLocalNotification(fresh, r) {
			drop[r.ID] = true
			ids = append(ids, r.ID)
		}
	}
	if len(ids) == 0 {
		return rules, nil
	}
	kept := make([]map[string]any, 0, len(rules))
	for _, m := range rules {
		if id, _ := m["id"].(string); !drop[id] {
			kept = append(kept, m)
		}
	}
	return kept, ids
}

// newNotifications returns the entries of ns not in before as is: matched by
// id, else by time+repeat+weekday+title, and compared in full.
func newNotifications(before []byte, ns []dsl.Notification) []dsl.Notification {
	prev := map[string]string{}
	if b, err := dsl.Parse(before); err == nil {
		for _, n := range b.App.Notifications {
			prev[notificationKey(n)] = notificationJSON(n)
		}
	}
	var out []dsl.Notification
	for _, n := range ns {
		if j, ok := prev[notificationKey(n)]; !ok || j != notificationJSON(n) {
			out = append(out, n)
		}
	}
	return out
}

func notificationKey(n dsl.Notification) string {
	if n.ID != "" {
		return "id:" + n.ID
	}
	return "at:" + n.Time + "|" + n.Repeat + "|" + n.Weekday + "|" + n.Title
}

func notificationJSON(n dsl.Notification) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func repeatsLocalNotification(ns []dsl.Notification, r *dsl.ServerRule) bool {
	on := r.On
	if (on.Every != "daily" && on.Every != "weekly") || on.Each != "" || on.Record != "" ||
		r.If != nil || r.Lookup != nil || r.Similar != nil || r.Pick != nil || len(r.Do) == 0 {
		return false
	}
	for _, a := range r.Do {
		if a.Notify == nil || a.CreateRecord != nil || a.UpdateRecord != nil || a.Increment != nil ||
			a.Notify.To.Kind != "owner" || len(a.Notify.With) > 0 {
			return false
		}
	}
	for _, n := range ns {
		at := n.Time
		if at == "" {
			at = "09:00" // iOS NotificationPlanner.fixedTrigger's default
		}
		if n.Entity != "" || n.When != nil || n.Near != nil || at != on.Time || n.Repeat != on.Every {
			continue
		}
		if on.Every == "weekly" && n.Weekday != on.Weekday {
			continue
		}
		return true
	}
	return false
}

// withoutOnDeviceDuplicates applies dropOnDeviceDuplicates to a parsed pass
// result: it returns the DSL and app to keep (re-parsed when rules were
// dropped) and the dropped ids.
func withoutOnDeviceDuplicates(current, before, next []byte, app *dsl.App, rules []map[string]any) ([]byte, *dsl.App, []string) {
	kept, dropped := dropOnDeviceDuplicates(current, before, app, rules)
	if len(dropped) == 0 {
		return next, app, nil
	}
	b, err := withServerRules(next, kept)
	if err != nil {
		return next, app, nil
	}
	a, err := dsl.Parse(b)
	if err != nil {
		return next, app, nil
	}
	return b, a, dropped
}
