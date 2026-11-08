package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestServerRulesOnDevicePrompt(t *testing.T) {
	for _, l := range serverRulesOnDeviceLines {
		if strings.Contains(serverRulesPrompt, l[0]) || !strings.Contains(serverRulesPrompt, l[1]) {
			t.Fatalf("line not swapped: %s", l[0])
		}
	}
}

const reminderApp = `{"schemaVersion":1,"app":{"name":"家計簿","voiceEntry":"floating",
"notifications":[{"id":"remind","title":"記録","body":"今日の記録を忘れずに","repeat":"daily","time":"21:05"}]},
"initialScreen":"home",
"entities":[{"id":"expenses","name":"支出","titleField":"memo","fields":[
 {"id":"memo","name":"メモ","type":"string"},
 {"id":"amount","name":"金額","type":"currency","required":true}]}],
"screens":[{"id":"home","title":"支出","components":[{"type":"list","entity":"expenses","item":{"title":"memo"}}]}]}`

func reminderRule(time string) string {
	return `{"unchanged":false,"declined":"","serverRules":[{"id":"remind_srv","on":{"every":"daily","time":"` + time + `"},
"do":[{"notify":{"to":{"kind":"owner"},"title":"記録","body":"今日の記録を忘れずに"}}]}]}`
}

// C2 「毎日21時05分に記録を忘れないよう通知して」: the local notification is
// enough, so the same server reminder to the creator is dropped (no Pro).
func TestServerRulesPass_DropsRepeatOfLocalReminder(t *testing.T) {
	f := &fakeCaller{replies: []string{reminderApp, reminderRule("21:05")}}
	svc, _ := newSvc(f)
	svc.ServerRulesPass = true
	res, err := svc.Generate(context.Background(), Request{AppID: "sr-" + t.Name(), Transcript: "家計簿。毎日21時05分に記録を忘れないよう通知して"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := dsl.Parse(res.DSL)
	if err != nil {
		t.Fatal(err)
	}
	sm := res.Meta.ServerRules
	if len(a.ServerRules) != 0 || len(a.App.Notifications) != 1 || sm == nil || sm.Applied || sm.Rules != 0 ||
		len(sm.Dropped) != 1 || sm.Dropped[0] != "remind_srv" {
		t.Fatalf("rules=%d meta=%+v", len(a.ServerRules), sm)
	}
	if strings.Contains(res.Summary, "自動の動き") {
		t.Fatalf("summary must not mention a server rule: %q", res.Summary)
	}
}

func TestServerRulesPass_KeepsOtherTimeReminder(t *testing.T) {
	_, _, a := generateWithRules(t, "家計簿。毎日21時05分に通知して、朝7時にも促して", reminderApp, reminderRule("07:00"))
	if len(a.ServerRules) != 1 {
		t.Fatalf("rules=%d", len(a.ServerRules))
	}
}

func TestRepeatsLocalNotification(t *testing.T) {
	app := &dsl.App{}
	app.App.Notifications = []dsl.Notification{
		{ID: "d", Repeat: "daily", Time: "21:05"},
		{ID: "w", Repeat: "weekly", Time: "08:00", Weekday: "mon"},
		{ID: "r", Repeat: "once", Entity: "events", Field: "start", OffsetMinutes: -1440},
	}
	owner := []dsl.RuleAction{{Notify: &dsl.RuleNotify{To: dsl.RuleRecipient{Kind: "owner"}, Title: "t", Body: "b"}}}
	members := []dsl.RuleAction{{Notify: &dsl.RuleNotify{To: dsl.RuleRecipient{Kind: "members"}, Title: "t", Body: "b"}}}
	cases := []struct {
		name string
		r    dsl.ServerRule
		want bool
	}{
		{"same daily", dsl.ServerRule{On: dsl.RuleTrigger{Every: "daily", Time: "21:05"}, Do: owner}, true},
		{"same weekly", dsl.ServerRule{On: dsl.RuleTrigger{Every: "weekly", Time: "08:00", Weekday: "mon"}, Do: owner}, true},
		{"other weekday", dsl.ServerRule{On: dsl.RuleTrigger{Every: "weekly", Time: "08:00", Weekday: "tue"}, Do: owner}, false},
		{"other time", dsl.ServerRule{On: dsl.RuleTrigger{Every: "daily", Time: "07:00"}, Do: owner}, false},
		{"to members", dsl.ServerRule{On: dsl.RuleTrigger{Every: "daily", Time: "21:05"}, Do: members}, false},
		{"with a condition", dsl.ServerRule{On: dsl.RuleTrigger{Every: "daily", Time: "21:05"}, If: &dsl.Expr{}, Do: owner}, false},
		{"per record", dsl.ServerRule{On: dsl.RuleTrigger{Every: "daily", Time: "21:05", Each: "events"}, Do: owner}, false},
		{"also writes", dsl.ServerRule{On: dsl.RuleTrigger{Every: "daily", Time: "21:05"}, Do: append(owner, dsl.RuleAction{CreateRecord: &dsl.RuleCreateRecord{Entity: "x"}})}, false},
		{"record trigger", dsl.ServerRule{On: dsl.RuleTrigger{Record: "created", Entity: "events"}, Do: owner}, false},
	}
	for _, c := range cases {
		if got := repeatsLocalNotification(app, &c.r); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

// A rule already in the app (an edit turn) is never dropped.
func TestDropOnDeviceDuplicates_KeepsExistingRule(t *testing.T) {
	cur := []byte(strings.Replace(reminderApp, `"initialScreen"`, `"serverRules":[{"id":"remind_srv","on":{"every":"daily","time":"21:05"},"do":[{"notify":{"to":{"kind":"owner"},"title":"記録","body":"今日の記録を忘れずに"}}]}],"initialScreen"`, 1))
	a, err := dsl.Parse(cur)
	if err != nil {
		t.Fatal(err)
	}
	rules := []map[string]any{{"id": "remind_srv"}}
	kept, dropped := dropOnDeviceDuplicates(cur, a, rules)
	if len(kept) != 1 || dropped != nil {
		t.Fatalf("kept=%v dropped=%v", kept, dropped)
	}
}
