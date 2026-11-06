package dsl

import (
	"strings"
	"testing"
)

// rolesShiftApp (#272 #273): admins confirm and delete shifts, one shift per slot,
// staff is a participant; one answer per person.
const rolesShiftApp = `{
  "schemaVersion": 1,
  "app": {"name": "シフト", "voiceEntry": "none"},
  "initialScreen": "home",
  "entities": [
    {"id": "shifts", "name": "シフト", "scope": "shared", "deleteBy": "admins", "unique": ["day", "slot"], "fields": [
      {"id": "day", "name": "日", "type": "date", "required": true},
      {"id": "slot", "name": "時間", "type": "enum", "options": ["朝", "昼", "夜"], "required": true},
      {"id": "staff", "name": "担当", "type": "member"},
      {"id": "confirmed", "name": "確定", "type": "boolean", "editableBy": "admins"}
    ]},
    {"id": "answers", "name": "回答", "scope": "shared", "unique": ["$author"], "deleteBy": "authorOrAdmins", "fields": [
      {"id": "choice", "name": "回答", "type": "string"}
    ]}
  ],
  "screens": [
    {"id": "home", "title": "シフト", "components": [
      {"type": "list", "entity": "shifts", "item": {"title": "day", "subtitle": "staff"}, "onItemTap": {"type": "navigate", "screen": "shift_form"}},
      {"type": "list", "entity": "answers", "item": {"title": "choice", "subtitle": "$author"}},
      {"type": "button", "label": "確定する", "visibleTo": "admins", "action": {"type": "navigate", "screen": "shift_form"}}
    ]},
    {"id": "shift_form", "title": "シフト", "entity": "shifts", "toolbar": [
      {"label": "保存", "action": {"type": "saveRecord", "entity": "shifts"}}
    ], "components": [
      {"type": "datePicker", "field": "day", "label": "日"},
      {"type": "picker", "field": "slot", "label": "時間"},
      {"type": "picker", "field": "staff", "label": "担当"},
      {"type": "toggle", "field": "confirmed", "label": "確定"}
    ]}
  ]
}`

func TestDataRulesValid(t *testing.T) {
	a, err := Parse([]byte(rolesShiftApp))
	if err != nil {
		t.Fatal(err)
	}
	if !a.UsesRoles() {
		t.Fatal("UsesRoles")
	}
	if e := a.Entities[0]; e.EffectiveDeleteBy() != DeleteByAdmins || !e.HasAdminOnlyFields() || !e.Fields[3].AdminOnly() {
		t.Fatalf("accessors: %+v", e)
	}
	if e := a.Entities[1]; e.HasAdminOnlyFields() || e.EffectiveDeleteBy() != DeleteByAuthorOrAdmins {
		t.Fatalf("answers: %+v", e)
	}
}

func TestDataRulesProblems(t *testing.T) {
	cases := []struct {
		name, from, to, want string
	}{
		{"editableBy on private", `"scope": "shared", "deleteBy": "admins"`, `"unique": ["day"]`, `editableBy admins needs scope shared or public`},
		{"member on public", `"scope": "shared", "deleteBy": "admins"`, `"scope": "public"`, `type member (a participant of the shared data) needs the entity scope "shared"`},
		{"member default", `"type": "member"}`, `"type": "member", "defaultValue": "u1"}`, `a member field cannot have defaultValue`},
		{"member not picker", `{"type": "picker", "field": "staff", "label": "担当"}`, `{"type": "textField", "field": "staff", "label": "担当"}`, `is type member; use a picker`},
		{"unique unknown", `["day", "slot"]`, `["day", "nope"]`, `unique: field "nope" does not exist`},
		{"unique twice", `["day", "slot"]`, `["day", "day"]`, `unique: items at 0 and 1 are equal`},
		{"unique too many", `["day", "slot"]`, `["day", "slot", "staff", "confirmed"]`, `unique: maxItems`},
		{"unique author private", `"scope": "shared", "unique": ["$author"]`, `"unique": ["$author"]`, `"$author" (one per person) needs scope shared or public`},
		{"deleteBy private", `"scope": "shared", "unique": ["$author"], "deleteBy": "authorOrAdmins"`, `"deleteBy": "authorOrAdmins"`, `deleteBy needs scope shared or public`},
		{"visibleTo private app", `"scope": "shared", "deleteBy": "admins", "unique": ["day", "slot"]`, `"unique": ["day", "slot"]`, `visibleTo admins needs a shared or public entity`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := rolesShiftApp
			if !strings.Contains(raw, c.from) {
				t.Fatalf("fixture lacks %q", c.from)
			}
			raw = strings.Replace(raw, c.from, c.to, 1)
			if c.name == "visibleTo private app" {
				// make the whole app private
				raw = strings.Replace(raw, `"scope": "shared", "unique": ["$author"], "deleteBy": "authorOrAdmins"`, `"unique": ["choice"]`, 1)
				raw = strings.Replace(raw, `, "editableBy": "admins"`, ``, 1)
				raw = strings.Replace(raw, `"type": "member"`, `"type": "string"`, 1)
				raw = strings.Replace(raw, `{"type": "picker", "field": "staff", "label": "担当"}`, `{"type": "textField", "field": "staff", "label": "担当"}`, 1)
				raw = strings.Replace(raw, `"subtitle": "$author"`, `"subtitle": "choice"`, 1)
			}
			_, err := Parse([]byte(raw))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
}

func TestDataRulesSchemaRejectsUnknownValues(t *testing.T) {
	for _, bad := range []struct{ from, to string }{
		{`"deleteBy": "admins"`, `"deleteBy": "everyone"`},
		{`"editableBy": "admins"`, `"editableBy": "owner"`},
		{`"visibleTo": "admins"`, `"visibleTo": "members"`},
	} {
		if _, err := Parse([]byte(strings.Replace(rolesShiftApp, bad.from, bad.to, 1))); err == nil {
			t.Fatalf("%s must be rejected", bad.to)
		}
	}
}
