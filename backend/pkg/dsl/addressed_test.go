package dsl

import (
	"strings"
	"testing"
)

// dmApp (#271): group messages read only by the sender and the recipients,
// with a read mark, an inbox and a sent list; a rule tells the sender when
// the message is answered.
const dmApp = `{
  "schemaVersion": 1,
  "app": {"name": "連絡", "voiceEntry": "none"},
  "initialScreen": "inbox",
  "entities": [
    {"id": "messages", "name": "メッセージ", "scope": "shared", "addressedTo": "to", "marks": [{"id": "read", "icon": "eye", "label": "既読"}], "fields": [
      {"id": "to", "name": "宛先", "type": "members", "required": true},
      {"id": "body", "name": "本文", "type": "text", "required": true},
      {"id": "answer", "name": "返事", "type": "string"}
    ]},
    {"id": "notes", "name": "メモ", "scope": "shared", "fields": [
      {"id": "about", "name": "どれ", "type": "ref", "refEntity": "messages"},
      {"id": "count", "name": "件数", "type": "integer"}
    ]}
  ],
  "screens": [
    {"id": "inbox", "title": "受信箱", "toolbar": [{"label": "送る", "action": {"type": "navigate", "screen": "compose"}}], "components": [
      {"type": "list", "entity": "messages", "filter": {"field": "$author", "op": "neq", "value": {"me": true}}, "item": {"title": "body", "subtitle": "$author", "mark": "read"}},
      {"type": "list", "entity": "messages", "filter": {"field": "$author", "op": "eq", "value": {"me": true}}, "item": {"title": "body", "subtitle": "to"}}
    ]},
    {"id": "compose", "title": "送る", "entity": "messages", "toolbar": [
      {"label": "送信", "action": {"type": "saveRecord", "entity": "messages"}}
    ], "components": [
      {"type": "picker", "field": "to", "label": "宛先"},
      {"type": "textEditor", "field": "body"}
    ]}
  ],
  "serverRules": [
    {"id": "answered", "on": {"record": "updated", "entity": "messages", "fields": ["answer"]},
     "do": [{"notify": {"to": {"kind": "author"}, "title": "返事が来ました", "body": "{answer}"}}]}
  ]
}`

func TestAddressedValid(t *testing.T) {
	a, err := Parse([]byte(dmApp))
	if err != nil {
		t.Fatal(err)
	}
	e := a.EntityByID("messages")
	if !a.UsesAddressed() || !e.IsAddressed() || a.EntityByID("notes").IsAddressed() {
		t.Fatal("IsAddressed / UsesAddressed")
	}
	got := e.Recipients(map[string]any{"to": []any{"b", "c", "b", ""}})
	if strings.Join(got, ",") != "b,c" {
		t.Fatalf("Recipients: %v", got)
	}
	if one := (&Entity{AddressedTo: "to"}).Recipients(map[string]any{"to": "b"}); len(one) != 1 || one[0] != "b" {
		t.Fatalf("member recipient: %v", one)
	}
	// a single member field works too
	if _, err := Parse([]byte(strings.Replace(dmApp, `"type": "members", "required": true`, `"type": "member", "required": true`, 1))); err != nil {
		t.Fatalf("member recipients: %v", err)
	}
}

func TestAddressedProblems(t *testing.T) {
	rule := `"do": [{"notify": {"to": {"kind": "author"}, "title": "返事が来ました", "body": "{answer}"}}]`
	cases := []struct{ name, from, to, want string }{
		{"public", `"scope": "shared", "addressedTo"`, `"scope": "public", "addressedTo"`, `addressed records need scope "shared"`},
		{"unknown field", `"addressedTo": "to"`, `"addressedTo": "nope"`, `addressedTo: field "nope" does not exist`},
		{"string field", `"addressedTo": "to"`, `"addressedTo": "body"`, `field "body" is type text; the recipients are a type member`},
		{"edit members", `"addressedTo": "to",`, `"addressedTo": "to", "edit": "members",`, `edit "members" would let people change records`},
		{"single", `"addressedTo": "to",`, `"addressedTo": "to", "single": true,`, `a single entity cannot be addressed`},
		{"unique without author", `"addressedTo": "to",`, `"addressedTo": "to", "unique": ["body"],`, `unique on an addressed entity must include "$author"`},
		{"admin recipients", `"type": "members", "required": true`, `"type": "members", "required": true, "editableBy": "admins"`, `has editableBy; the sender chooses`},
		{"members default", `"type": "members", "required": true`, `"type": "members", "defaultValue": "u1"`, `a members field cannot have defaultValue`},
		{"members not picker", `{"type": "picker", "field": "to", "label": "宛先"}`, `{"type": "textField", "field": "to", "label": "宛先"}`, `is type members; use a picker`},
		{"notify members", `{"kind": "author"}`, `{"kind": "members"}`, `is addressed (only its author and recipients read it); notify kind "author" only`},
		{"copy out", rule, `"do": [{"createRecord": {"entity": "notes", "values": {"count": 1}}}]`, `a rule on it cannot copy it into other records`},
		{"create addressed", `{"record": "updated", "entity": "messages", "fields": ["answer"]}`, `{"record": "created", "entity": "notes"}`, ``},
		{"change recipients", rule, `"do": [{"updateRecord": {"values": {"to": "x"}}}]`, `a rule cannot change them`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(dmApp, c.from) {
				t.Fatalf("fixture lacks %q", c.from)
			}
			raw := strings.Replace(dmApp, c.from, c.to, 1)
			if c.name == "create addressed" {
				raw = strings.Replace(raw, rule, `"do": [{"createRecord": {"entity": "messages", "values": {"body": "x"}}}]`, 1)
				c.want = `entity "messages" is addressed; a rule cannot send it`
			}
			_, err := Parse([]byte(raw))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
}
