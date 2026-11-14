package dsl

import "testing"

func TestDescribeServerRuleEN(t *testing.T) {
	cases := map[string][]string{
		"server_rules.json": {
			`When "出欠" is created (if the conditions match): notify whoever created the "予定", count "参加の数" of the "予定"`,
			`When "メッセージ" is created: notify all members`,
			`When "状態" of "予定" changes (if the conditions match): notify all members, add a record to "変更の記録"`,
			`Every Monday at 09:00: notify the app's creator`,
		},
		"server_rules_addressed.json": {
			`When "メッセージ" is created: notify the recipients`,
		},
		"server_rules_lookup.json": {
			`Every day at 07:00, for each "履修登録" record, if no "名簿" record has "学籍番号" equal to "学籍番号": update that record`,
			`When "履修登録" is created, if no "名簿" record has "学籍番号" equal to "学籍番号": notify the app's creator`,
			`Every Sunday at 10:00, for each "店" record, if no "訪問" record has "店" equal to this record: notify the app's creator`,
		},
		"server_rules_pick.json": {
			`Every day at 08:00, pick 1 from "メニュー" at random (matching the conditions, not in "食べた記録"): add a record to "今日のおすすめ", notify all members`,
			`When "食べた記録" is created, pick 1 from "メニュー" oldest first (not in "食べた記録"): update that record, notify the writer`,
		},
		"server_rules_ranking.json": {
			`Every day at 21:00, for each "参加者" record: total "参加者", then update that record, notify the writer`,
		},
		"server_rules_reads.json": {
			`When "支出" is created (if the conditions match): total "支出", then notify the app's creator`,
			`Every Sunday at 20:00: total "支出", then add a record to "週のまとめ", notify the app's creator`,
			`Every day at 08:00, for each "やること" record (if the conditions match): notify the app's creator`,
		},
		"server_rules_similar.json": {
			`When "趣味" / "レベル" of "プロフィール" changes, find the most similar "プロフィール" record by "趣味" / "レベル" (2 or more in common): update that record, notify whoever created the similar "プロフィール"`,
			`Every day at 08:00, for each "プロフィール" record, find the most similar "プロフィール" record by "趣味" (1 or more in common): update that record`,
			`When "サークル" is created, find the most similar "プロフィール" record by "活動" (1 or more in common): notify whoever created the similar "プロフィール"`,
		},
	}
	for name, want := range cases {
		a := readsApp(t, name)
		if len(a.ServerRules) != len(want) {
			t.Fatalf("%s: %d rules, want %d", name, len(a.ServerRules), len(want))
		}
		for i := range a.ServerRules {
			if got := a.DescribeServerRuleIn(&a.ServerRules[i], "en"); got != want[i] {
				t.Errorf("%s rule %d:\n got %s\nwant %s", name, i, got, want[i])
			}
		}
	}
}

// Branches the fixtures do not reach: monthly, updated without fields,
// lookup exists / by id, pick newest, update / notify via ref, off.
func TestDescribeServerRuleENBranches(t *testing.T) {
	a := serverRulesApp(t)
	off := false
	cases := []struct {
		rule ServerRule
		want string
	}{
		{ServerRule{On: RuleTrigger{Every: "monthly", Day: 5, Time: "09:00"}, Do: []RuleAction{{CreateRecord: &RuleCreateRecord{Entity: "log"}}}},
			`On day 5 of every month at 09:00: add a record to "変更の記録"`},
		{ServerRule{On: RuleTrigger{Record: TriggerUpdated, Entity: "rsvps"}, Do: []RuleAction{{UpdateRecord: &RuleUpdateRecord{Ref: "event"}}, {Notify: &RuleNotify{To: RuleRecipient{Kind: RecipientAuthor}}}}},
			`When "出欠" is updated: update the "予定" record, notify the writer`},
		{ServerRule{On: RuleTrigger{Record: TriggerCreated, Entity: "rsvps"}, Lookup: &RuleLookup{Op: LookupExists, Entity: "events", Field: LookupID, Equals: "event"}, Do: []RuleAction{{Notify: &RuleNotify{To: RuleRecipient{Kind: RecipientOwner}}}}},
			`When "出欠" is created, if a "予定" record has an ID equal to "予定": notify the app's creator`},
		{ServerRule{On: RuleTrigger{Every: "daily", Time: "08:00"}, Pick: &RulePick{Entity: "events", Order: PickNewest}, Do: []RuleAction{{UpdateRecord: &RuleUpdateRecord{Ref: PickRef}}, {Notify: &RuleNotify{To: RuleRecipient{Kind: RecipientAuthorOf, Field: PickRef}}}}},
			`Every day at 08:00, pick 1 from "予定" newest first: update the picked "予定" record, notify whoever created the picked "予定"`},
		{ServerRule{On: RuleTrigger{Record: TriggerCreated, Entity: "messages"}, Do: []RuleAction{{Notify: &RuleNotify{To: RuleRecipient{Kind: RecipientMembers}}}}, Enabled: &off},
			`When "メッセージ" is created: notify all members` + DisabledNoteEN},
	}
	for i := range cases {
		if got := a.DescribeServerRuleIn(&cases[i].rule, "en"); got != cases[i].want {
			t.Errorf("case %d:\n got %s\nwant %s", i, got, cases[i].want)
		}
	}
}

func TestDescribeServerRuleInDefaultsToJapanese(t *testing.T) {
	a := serverRulesApp(t)
	for i := range a.ServerRules {
		r := &a.ServerRules[i]
		for _, lang := range []string{"", "ja", "fr"} {
			if got, want := a.DescribeServerRuleIn(r, lang), a.DescribeServerRule(r); got != want {
				t.Errorf("rule %d lang %q: %s, want %s", i, lang, got, want)
			}
		}
	}
}
