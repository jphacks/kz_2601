package dsl

import (
	"encoding/json"
	"strings"
	"testing"
)

// addressedWith is the addressed test app with rules in place of its own.
func addressedWith(t *testing.T, rules string) []byte {
	t.Helper()
	var a map[string]any
	if err := json.Unmarshal(readsRaw(t, "server_rules_addressed.json"), &a); err != nil {
		t.Fatal(err)
	}
	var rs any
	if err := json.Unmarshal([]byte(rules), &rs); err != nil {
		t.Fatalf("rules: %v", err)
	}
	a["serverRules"] = rs
	out, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

const (
	onNote    = `"on": {"record": "created", "entity": "notes"}`
	onMessage = `"on": {"record": "created", "entity": "messages"}`
	daily     = `"on": {"every": "daily", "time": "08:00"}`
	eachNote  = `"on": {"every": "daily", "time": "08:00", "each": "notes"}`
	countMsgs = `{"agg": "count", "entity": "messages"}`
	lookup    = `{"ref": "about.body"}`
)

func notifyOwner(with string) string {
	return `{"notify": {"to": {"kind": "owner"}, "title": "件数", "body": "{n}", "with": {"n": ` + with + `}}}`
}

// An addressed entity is not a target of each / agg / a ref lookup, wherever
// the expression is written.
func TestAddressedRuleReads(t *testing.T) {
	const addressedMsg = `entity "messages" is addressed`
	cases := []struct{ name, rule, want string }{
		{"each", `{"id": "r", "on": {"every": "daily", "time": "08:00", "each": "messages"},
			"do": [{"notify": {"to": {"kind": "owner"}, "title": "t", "body": "b"}}]}`, `on.each: ` + addressedMsg},
		{"agg in if", `{"id": "r", ` + daily + `, "if": {"op": "gt", "args": [` + countMsgs + `, 0]},
			"do": [{"notify": {"to": {"kind": "owner"}, "title": "t", "body": "b"}}]}`, `agg: ` + addressedMsg},
		{"agg in with", `{"id": "r", ` + daily + `, "do": [` + notifyOwner(countMsgs) + `]}`, `agg: ` + addressedMsg},
		{"agg in values", `{"id": "r", ` + daily + `, "do": [{"createRecord": {"entity": "notes", "values":
			{"text": {"op": "toString", "args": [` + countMsgs + `]}}}}]}`, `agg: ` + addressedMsg},
		{"agg on the row of each", `{"id": "r", ` + eachNote + `, "do": [{"updateRecord": {"values":
			{"text": {"op": "toString", "args": [` + countMsgs + `]}}}}]}`, `agg: ` + addressedMsg},
		{"agg in a filter value", `{"id": "r", ` + daily + `, "do": [` + notifyOwner(`{"agg": "count", "entity": "notes",
			"filter": {"field": "text", "op": "eq", "value": {"op": "toString", "args": [`+countMsgs+`]}}}`) + `]}`, `agg: ` + addressedMsg},
		{"lookup in if", `{"id": "r", ` + onNote + `, "if": {"op": "eq", "args": [` + lookup + `, "x"]},
			"do": [{"notify": {"to": {"kind": "owner"}, "title": "t", "body": "b"}}]}`, `ref "about.body": ` + addressedMsg},
		{"lookup in values", `{"id": "r", ` + onNote + `, "do": [{"updateRecord": {"values": {"text": ` + lookup + `}}}]}`, `ref "about.body": ` + addressedMsg},
		{"lookup in with", `{"id": "r", ` + onNote + `, "do": [` + notifyOwner(`{"op": "len", "args": [`+lookup+`]}`) + `]}`, `ref "about.body": ` + addressedMsg},
		{"lookup in a filter value", `{"id": "r", ` + onNote + `, "do": [` + notifyOwner(`{"agg": "count", "entity": "notes",
			"filter": {"field": "text", "op": "eq", "value": `+lookup+`}}`) + `]}`, `ref "about.body": ` + addressedMsg},
		{"lookup on the row of each", `{"id": "r", ` + eachNote + `, "do": [{"updateRecord": {"values": {"text": ` + lookup + `}}}]}`, `ref "about.body": ` + addressedMsg},
		{"recipients from each", `{"id": "r", ` + eachNote + `, "do": [{"updateRecord": {"ref": "about", "values": {"to": {"ref": "text"}}}}]}`, `a rule cannot change them`},
		{"notify owner", `{"id": "r", ` + onMessage + `, "do": [{"notify": {"to": {"kind": "owner"}, "title": "t", "body": "{body}"}}]}`, `notify kind "author" or "recipients" only`},
		{"notify members", `{"id": "r", ` + onMessage + `, "do": [{"notify": {"to": {"kind": "members"}, "title": "t", "body": "b"}}]}`, `notify kind "author" or "recipients" only`},
		{"recipients of another entity", `{"id": "r", ` + onNote + `, "do": [{"notify": {"to": {"kind": "recipients", "field": "about"}, "title": "t", "body": "b"}}]}`, `entity "notes" has no addressedTo`},
		{"recipients: wrong field", `{"id": "r", ` + onMessage + `, "do": [{"notify": {"to": {"kind": "recipients", "field": "body"}, "title": "t", "body": "b"}}]}`, `must be the addressedTo`},
		{"recipients without a field", `{"id": "r", ` + onMessage + `, "do": [{"notify": {"to": {"kind": "recipients"}, "title": "t", "body": "b"}}]}`, `must be the addressedTo`},
		{"recipients on a time trigger", `{"id": "r", ` + daily + `, "do": [{"notify": {"to": {"kind": "recipients", "field": "to"}, "title": "t", "body": "b"}}]}`, `kind recipients needs a record trigger`},
		{"field on owner", `{"id": "r", ` + onNote + `, "do": [{"notify": {"to": {"kind": "owner", "field": "about"}, "title": "t", "body": "b"}}]}`, `field is only for kind authorOf or recipients`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(addressedWith(t, `[`+c.rule+`]`))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
}

// What stays allowed: the record itself on its own rule, ids, and notifying
// the author of a ref target (no value of the record is read).
func TestAddressedRuleAllowed(t *testing.T) {
	a := readsApp(t, "server_rules_addressed.json")
	if got := a.DescribeServerRule(&a.ServerRules[0]); !strings.Contains(got, "宛先の人に通知") {
		t.Fatalf("describe: %s", got)
	}
	for name, rule := range map[string]string{
		"row fields":     `{"id": "r", ` + onMessage + `, "do": [{"notify": {"to": {"kind": "author"}, "title": "t", "body": "{body}", "with": {"n": {"op": "len", "args": [{"ref": "body"}]}, "m": ` + `{"agg": "count", "entity": "notes"}` + `}}}]}`,
		"copy the ref":   `{"id": "r", ` + onNote + `, "do": [{"updateRecord": {"values": {"text": {"ref": "about"}}}}]}`,
		"author of ref":  `{"id": "r", ` + onNote + `, "do": [{"notify": {"to": {"kind": "authorOf", "field": "about"}, "title": "t", "body": "{text}"}}]}`,
		"other entities": `{"id": "r", ` + eachNote + `, "do": [{"updateRecord": {"values": {"text": {"op": "toString", "args": [{"agg": "count", "entity": "notes"}]}}}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			rule := strings.Replace(rule, `"body": "{body}", "with": {"n": {"op": "len", "args": [{"ref": "body"}]}, "m"`, `"body": "{n} {m}", "with": {"n": {"op": "len", "args": [{"ref": "body"}]}, "m"`, 1)
			if _, err := Parse(addressedWith(t, `[`+rule+`]`)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// The run-time table is the one Validate uses, for a rule of any version.
func TestAddressedRuleProblems(t *testing.T) {
	a := readsApp(t, "server_rules_addressed.json")
	if got := a.AddressedRuleProblems(&a.ServerRules[0]); len(got) != 0 {
		t.Fatalf("valid rule: %v", got)
	}
	rule := &ServerRule{ID: "r", On: RuleTrigger{Every: "daily", Time: "08:00", Each: "messages"}}
	if got := a.AddressedRuleProblems(rule); len(got) != 1 || !strings.Contains(got[0], "on.each") {
		t.Fatalf("each: %v", got)
	}
	if got := (&App{}).AddressedRuleProblems(rule); got != nil {
		t.Fatalf("no addressed entity: %v", got)
	}
}
