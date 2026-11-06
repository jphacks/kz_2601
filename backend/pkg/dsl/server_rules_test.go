package dsl

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func serverRulesRaw(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/server_rules.json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func serverRulesApp(t *testing.T) *App {
	t.Helper()
	a, err := Parse(serverRulesRaw(t))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func expr(t *testing.T, s string) *Expr {
	t.Helper()
	var e Expr
	if err := json.Unmarshal([]byte(s), &e); err != nil {
		t.Fatal(err)
	}
	return &e
}

// #340: the #269 examples (出欠 → 予定を作った人、新着メッセージ → 参加者全員)
// parse, validate and round-trip without losing a key.
func TestServerRulesParseAndRoundTrip(t *testing.T) {
	raw := serverRulesRaw(t)
	a := serverRulesApp(t)
	if len(a.ServerRules) != 4 {
		t.Fatalf("rules = %d", len(a.ServerRules))
	}
	out, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	var want, got any
	_ = json.Unmarshal(raw, &want)
	_ = json.Unmarshal(out, &got)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("round-trip mismatch\n got: %s", out)
	}
	if _, err := Parse(out); err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
}

// Existing documents have no serverRules and must marshal exactly as before.
func TestServerRulesOmittedWhenAbsent(t *testing.T) {
	out, err := json.Marshal(Initial())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "serverRules") {
		t.Fatalf("serverRules leaked into a document without rules: %s", out)
	}
}

func TestServerRulesAccepts(t *testing.T) {
	cases := []struct {
		name string
		edit func(a *App)
	}{
		{"no condition", func(a *App) { a.ServerRules[0].If = nil }},
		{"daily", func(a *App) { a.ServerRules[3].On = RuleTrigger{Every: "daily", Time: "21:30"} }},
		{"monthly", func(a *App) { a.ServerRules[3].On = RuleTrigger{Every: "monthly", Time: "08:00", Day: 28} }},
		{"updated without fields", func(a *App) {
			a.ServerRules[2].On.Fields = nil
			a.ServerRules[0].Do = a.ServerRules[0].Do[:1] // the increment would fire it (chain)
		}},
		{"increment by a field", func(a *App) {
			a.ServerRules[0].Do[1].Increment.By = expr(t, `{"op":"neg","args":[1]}`)
		}},
		{"update trigger record", func(a *App) {
			a.ServerRules[1].Do = append(a.ServerRules[1].Do, RuleAction{UpdateRecord: &RuleUpdateRecord{Values: map[string]Expr{"body": *expr(t, `{"op":"concat","args":[{"ref":"body"},"!"]}`)}}})
		}},
		{"update ref target", func(a *App) {
			a.ServerRules[0].Do = append(a.ServerRules[0].Do, RuleAction{UpdateRecord: &RuleUpdateRecord{Ref: "event", Values: map[string]Expr{"date": *expr(t, `"2026-12-01"`)}}})
		}},
		{"lookup through ref", func(a *App) {
			a.ServerRules[0].If = expr(t, `{"op":"neq","args":[{"ref":"event.status"},"中止"]}`)
		}},
		{"author on shared", func(a *App) { a.ServerRules[1].Do[0].Notify.To = RuleRecipient{Kind: RecipientAuthor} }},
		{"public owner", func(a *App) {
			a.Entities[2].Scope = ScopePublic
			a.ServerRules[1].Do[0].Notify.To = RuleRecipient{Kind: RecipientOwner}
		}},
		{"public authorOf", func(a *App) {
			a.Entities[0].Scope = ScopePublic
			a.Entities[1].Scope = ScopePublic
			a.ServerRules[2].Do[0].Notify.To = RuleRecipient{Kind: RecipientOwner}
		}},
		{"private owner", func(a *App) {
			a.Entities = []Entity{{ID: "diary", Name: "日記", Fields: []Field{{ID: "mood", Name: "気分", Type: FieldInteger}}}}
			a.Screens[0].Components = []Component{{Type: CompText, Text: "日記"}}
			a.ServerRules = []ServerRule{{ID: "low", On: RuleTrigger{Record: TriggerCreated, Entity: "diary"},
				If: expr(t, `{"op":"lt","args":[{"ref":"mood"},2]}`),
				Do: []RuleAction{{Notify: &RuleNotify{To: RuleRecipient{Kind: RecipientOwner}, Title: "ひとこと", Body: "気分が {mood} の日でした"}}}}}
		}},
		{"time inside text", func(a *App) { a.ServerRules[3].Do[0].Notify.Body = "9:00 から 10:30 まで" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := serverRulesApp(t)
			c.edit(a)
			if err := a.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// The forbidden declarations of #336 / #339 are rejected by Validate.
func TestServerRulesRejects(t *testing.T) {
	notify := func(a *App) *RuleNotify { return a.ServerRules[1].Do[0].Notify }
	cases := []struct {
		name string
		edit func(a *App)
		want string
	}{
		// trigger
		{"unknown entity", func(a *App) { a.ServerRules[0].On.Entity = "ghost" }, `entity "ghost" does not exist`},
		{"unknown trigger field", func(a *App) { a.ServerRules[2].On.Fields = []string{"ghost"} }, `field "ghost" is not a field`},
		{"fields on created", func(a *App) { a.ServerRules[0].On.Fields = []string{"answer"} }, "fields is only for record updated"},
		{"record and time", func(a *App) { a.ServerRules[0].On.Every = "daily" }, "not both"},
		{"no trigger", func(a *App) { a.ServerRules[0].On = RuleTrigger{} }, "needs record + entity"},
		{"bad time", func(a *App) { a.ServerRules[3].On.Time = "25:00" }, "must be HH:MM"},
		{"weekly without weekday", func(a *App) { a.ServerRules[3].On.Weekday = "" }, "needs weekday"},
		{"monthly day 31", func(a *App) { a.ServerRules[3].On = RuleTrigger{Every: "monthly", Time: "09:00", Day: 31} }, "day 1-28"},
		{"hourly", func(a *App) { a.ServerRules[3].On.Every = "hourly" }, "daily, weekly or monthly"},
		{"deleted", func(a *App) { a.ServerRules[0].On.Record = "deleted" }, "created or updated"},
		// condition
		{"agg in condition", func(a *App) {
			a.ServerRules[0].If = expr(t, `{"op":"gt","args":[{"agg":"count","entity":"rsvps"},3]}`)
		}, "agg is not available in server rules"},
		{"condition not boolean", func(a *App) { a.ServerRules[0].If = expr(t, `{"ref":"answer"}`) }, "must be true/false"},
		{"unknown field in condition", func(a *App) { a.ServerRules[0].If = expr(t, `{"ref":"ghost"}`) }, `ref "ghost" is not a field`},
		{"prev on created", func(a *App) {
			a.ServerRules[0].If = expr(t, `{"op":"eq","args":[{"ref":"$prev.answer"},"no"]}`)
		}, "only for record updated triggers"},
		{"ref on time trigger", func(a *App) { a.ServerRules[3].If = expr(t, `{"op":"eq","args":[{"ref":"title"},"x"]}`) }, "is not allowed here"},
		{"computed in condition", func(a *App) {
			a.Entities[1].Fields = append(a.Entities[1].Fields, Field{ID: "yes", Name: "参加", Type: FieldBoolean, Computed: expr(t, `{"op":"eq","args":[{"ref":"answer"},"yes"]}`)})
			a.ServerRules[0].If = expr(t, `{"ref":"yes"}`)
		}, "calculated on the device"},
		// recipients
		{"public members", func(a *App) { a.Entities[2].Scope = ScopePublic }, `kind "members" is not allowed on public data`},
		{"public author", func(a *App) {
			a.Entities[2].Scope = ScopePublic
			notify(a).To = RuleRecipient{Kind: RecipientAuthor}
		}, `kind "author" is not allowed on public data`},
		{"private members", func(a *App) {
			for i := range a.Entities {
				a.Entities[i].Scope = ""
			}
		}, `kind "members" is not allowed on private data`},
		{"unknown recipient", func(a *App) { notify(a).To = RuleRecipient{Kind: "everyone"} }, "must be owner, author, authorOf or members"},
		{"authorOf non-ref", func(a *App) { a.ServerRules[0].Do[0].Notify.To.Field = "answer" }, "must be a type ref field"},
		{"authorOf on time trigger", func(a *App) {
			a.ServerRules[3].Do[0].Notify.To = RuleRecipient{Kind: RecipientAuthorOf, Field: "event"}
		}, "needs a record trigger"},
		{"field on members", func(a *App) { notify(a).To.Field = "body" }, "field is only for kind authorOf"},
		// text
		{"title too long", func(a *App) { notify(a).Title = strings.Repeat("あ", 31) }, "too long (31 > 30"},
		{"body too long", func(a *App) { notify(a).Body = strings.Repeat("あ", 81) }, "too long (81 > 80"},
		{"empty body", func(a *App) { notify(a).Body = " " }, "is empty"},
		{"url", func(a *App) { notify(a).Body = "詳しくは https://example.com へ" }, "URL or domain"},
		{"bare domain", func(a *App) { notify(a).Body = "example.jp を見て" }, "URL or domain"},
		{"fullwidth domain", func(a *App) { notify(a).Body = "ｅｘａｍｐｌｅ．ｃｏｍ" }, "URL or domain"},
		{"email", func(a *App) { notify(a).Body = "連絡は a@b へ" }, "email address"},
		{"phone", func(a *App) { notify(a).Body = "090-1234-5678 に電話" }, "phone number"},
		{"fullwidth phone", func(a *App) { notify(a).Body = "０９０１２３４５６７８" }, "phone number"},
		{"official", func(a *App) { notify(a).Title = "運営からのお知らせ" }, "look official"},
		{"service name", func(a *App) { notify(a).Title = "Tailor It より" }, "look official"},
		{"control char", func(a *App) { notify(a).Body = "改行\nあり" }, "control or invisible"},
		{"bidi char", func(a *App) { notify(a).Body = "向き\u202eを変える" }, "control or invisible"},
		{"zero width", func(a *App) { notify(a).Body = "見え\u200bない" }, "control or invisible"},
		{"unknown placeholder", func(a *App) { notify(a).Body = "{ghost} が来ました" }, "{ghost} is not a field"},
		{"unmatched brace", func(a *App) { notify(a).Body = "{body が来ました" }, "unmatched"},
		{"placeholder on time trigger", func(a *App) { a.ServerRules[3].Do[0].Notify.Body = "{title}" }, "time trigger has no record"},
		{"placeholder ref field", func(a *App) { a.ServerRules[0].Do[0].Notify.Body = "{event}" }, "cannot be inserted"},
		{"placeholder hidden", func(a *App) {
			a.Entities[2].Fields = append(a.Entities[2].Fields, Field{ID: "secret", Name: "答え", Type: FieldString, Hidden: true})
			notify(a).Body = "{secret}"
		}, "hidden"},
		{"placeholder location", func(a *App) {
			a.Entities[2].Fields = append(a.Entities[2].Fields, Field{ID: "where", Name: "場所", Type: FieldLocation})
			notify(a).Body = "{where}"
		}, "cannot be inserted"},
		// writes
		{"write unknown entity", func(a *App) { a.ServerRules[2].Do[1].CreateRecord.Entity = "ghost" }, `entity "ghost" does not exist`},
		{"write unknown field", func(a *App) {
			a.ServerRules[2].Do[1].CreateRecord.Values["ghost"] = *expr(t, `"x"`)
		}, `"ghost" is not a field of entity "log"`},
		{"missing required", func(a *App) { a.ServerRules[2].Do[1].CreateRecord.Values = map[string]Expr{} }, `required field "note"`},
		{"value type", func(a *App) {
			a.ServerRules[2].Do[1].CreateRecord.Values["note"] = *expr(t, `{"op":"eq","args":[1,1]}`)
		}, "is string but the value is a boolean"},
		{"shared rule writes private", func(a *App) { a.Entities[3].Scope = "" }, "same side"},
		{"write photo", func(a *App) {
			a.Entities[3].Fields = append(a.Entities[3].Fields, Field{ID: "pic", Name: "写真", Type: FieldPhoto})
			a.ServerRules[2].Do[1].CreateRecord.Values["pic"] = *expr(t, `"x"`)
		}, "cannot write a photo field"},
		{"update on time trigger", func(a *App) {
			a.ServerRules[3].Do = append(a.ServerRules[3].Do, RuleAction{UpdateRecord: &RuleUpdateRecord{Values: map[string]Expr{"title": *expr(t, `"x"`)}}})
		}, "needs a record trigger"},
		{"increment non-number", func(a *App) { a.ServerRules[0].Do[1].Increment.Field = "title" }, "needs an integer / number / currency"},
		{"increment non-ref", func(a *App) { a.ServerRules[0].Do[1].Increment.Ref = "answer" }, "must be a type ref field"},
		{"increment on time trigger", func(a *App) {
			a.ServerRules[3].Do = append(a.ServerRules[3].Do, RuleAction{Increment: &RuleIncrement{Ref: "event", Field: "yes_count"}})
		}, "needs a record trigger"},
		{"two actions in one", func(a *App) {
			a.ServerRules[0].Do[0].Increment = &RuleIncrement{Ref: "event", Field: "yes_count"}
		}, "exactly one of"},
		{"empty do", func(a *App) { a.ServerRules[0].Do = nil }, "at least one action"},
		// chains
		{"chain via create", func(a *App) {
			a.ServerRules = append(a.ServerRules, ServerRule{ID: "on_log", On: RuleTrigger{Record: TriggerCreated, Entity: "log"},
				Do: []RuleAction{{Notify: &RuleNotify{To: RuleRecipient{Kind: RecipientOwner}, Title: "記録", Body: "記録が増えました"}}}})
		}, "no chains"},
		{"chain via increment", func(a *App) { a.ServerRules[2].On.Fields = []string{"status", "yes_count"} }, "no chains"},
		{"self chain", func(a *App) {
			a.ServerRules[2].Do = append(a.ServerRules[2].Do, RuleAction{UpdateRecord: &RuleUpdateRecord{Values: map[string]Expr{"status": *expr(t, `"確定"`)}}})
		}, "no chains"},
		// limits and app kinds
		{"too many rules", func(a *App) {
			for len(a.ServerRules) <= MaxServerRulesPro {
				r := a.ServerRules[1]
				r.ID = "r" + strings.Repeat("x", len(a.ServerRules))
				a.ServerRules = append(a.ServerRules, r)
			}
		}, "too many rules"},
		{"too many actions", func(a *App) {
			for len(a.ServerRules[1].Do) <= MaxRuleActionsPro {
				a.ServerRules[1].Do = append(a.ServerRules[1].Do, a.ServerRules[1].Do[0])
			}
		}, "too many actions"},
		{"too many scheduled", func(a *App) {
			for i := 0; i < MaxScheduledRulesPro; i++ {
				r := a.ServerRules[3]
				r.ID = "s" + strings.Repeat("x", i)
				a.ServerRules = append(a.ServerRules, r)
			}
		}, "too many time-triggered rules"},
		{"duplicate id", func(a *App) { a.ServerRules[1].ID = a.ServerRules[0].ID }, "duplicate rule id"},
		{"bad id", func(a *App) { a.ServerRules[1].ID = "Bad Id" }, "not a valid id"},
		{"matching app", func(a *App) {
			a.Entities[2].Match = MatchProfile
		}, "matching apps"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := serverRulesApp(t)
			c.edit(a)
			err := a.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
}

// Words outside the vocabulary (external sends, arbitrary recipients, delete
// triggers, unknown keys) never get past the closed JSON Schema.
func TestServerRulesSchemaRejects(t *testing.T) {
	cases := []struct{ name, from, to string }{
		{"webhook action", `{"increment": {"ref": "event", "field": "yes_count"}}`, `{"webhook": {"url": "https://example.com"}}`},
		{"openUrl action", `{"increment": {"ref": "event", "field": "yes_count"}}`, `{"openUrl": "https://example.com"}`},
		{"recipient uid", `{"kind": "members"}`, `{"kind": "members", "uid": "abc"}`},
		{"recipient email", `{"kind": "members"}`, `{"kind": "email", "field": "a@b.c"}`},
		{"delete trigger", `"record": "created", "entity": "messages"`, `"record": "deleted", "entity": "messages"`},
		{"hourly", `"every": "weekly"`, `"every": "hourly"`},
		{"unknown rule key", `"id": "morning",`, `"id": "morning", "priority": 1,`},
		{"two actions in one object", `{"increment": {"ref": "event", "field": "yes_count"}}`, `{"increment": {"ref": "event", "field": "yes_count"}, "notify": {"to": {"kind": "owner"}, "title": "a", "body": "b"}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := string(serverRulesRaw(t))
			if !strings.Contains(raw, c.from) {
				t.Fatalf("testdata has no %s", c.from)
			}
			if _, err := Parse([]byte(strings.Replace(raw, c.from, c.to, 1))); err == nil {
				t.Fatal("Parse accepted it")
			}
		})
	}
}

func TestServerRuleQuota(t *testing.T) {
	a := serverRulesApp(t)
	if p := a.ServerRuleQuotaProblems(true); len(p) != 0 {
		t.Fatalf("pro: %v", p)
	}
	p := a.ServerRuleQuotaProblems(false)
	if len(p) != 1 || !strings.Contains(p[0], "4 rules exceed the plan limit of 3") {
		t.Fatalf("free: %v", p)
	}
}

func TestDescribeServerRule(t *testing.T) {
	a := serverRulesApp(t)
	want := []string{
		"「出欠」が作られた時(条件に合えば)、「予定」を作った人に通知・「予定」の「参加の数」を数える",
		"「メッセージ」が作られた時、参加者全員に通知",
		"「予定」の「状態」が変わった時(条件に合えば)、参加者全員に通知・「変更の記録」に記録を足す",
		"毎週月曜 09:00、アプリの作成者に通知",
	}
	for i := range a.ServerRules {
		if got := a.DescribeServerRule(&a.ServerRules[i]); got != want[i] {
			t.Errorf("rule %d: %s", i, got)
		}
	}
}
