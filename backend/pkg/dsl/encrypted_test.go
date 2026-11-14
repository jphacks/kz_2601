package dsl

import (
	"strings"
	"testing"
)

// encryptedDoc: a private diary with an encrypted body. extra is spliced into
// the home screen's components, rules into serverRules.
func encryptedDoc(field, scope, extra, rules string) string {
	if scope != "" {
		scope = `"scope":"` + scope + `",`
	}
	return `{"schemaVersion":1,"app":{"name":"日記","voiceEntry":"none"},"initialScreen":"home",
"entities":[{"id":"diary",` + scope + `"name":"日記","titleField":"title","fields":[
 {"id":"title","name":"題","type":"string"},` + field + `,
 {"id":"mood","name":"気分","type":"integer"}]}],
"screens":[
 {"id":"home","title":"一覧","components":[
  {"type":"list","entity":"diary","item":{"title":"title","subtitle":"body"},"onItemTap":{"type":"navigate","screen":"form"}},
  {"type":"button","label":"書く","action":{"type":"navigate","screen":"form"}}` + extra + `]},
 {"id":"form","title":"日記","entity":"diary","components":[
  {"type":"textField","field":"title"},{"type":"textField","field":"body"}],
  "toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"diary"}}]}]` + rules + `}`
}

const encBody = `{"id":"body","name":"本文","type":"text","encrypted":true}`

func TestEncryptedFieldValid(t *testing.T) {
	app, err := Parse([]byte(encryptedDoc(encBody, "", "", "")))
	if err != nil {
		t.Fatal(err)
	}
	if !app.HasEncryptedFields() || !app.Entities[0].FieldByID("body").Encrypted {
		t.Fatal("encrypted not decoded")
	}
	// the list shows the decrypted body on the device: fine
	if _, err := Parse([]byte(encryptedDoc(`{"id":"body","name":"本文","type":"string","encrypted":true}`, "", "", ""))); err != nil {
		t.Fatal(err)
	}
}

func TestEncryptedFieldRejected(t *testing.T) {
	for name, tc := range map[string]struct{ field, scope, extra, rules, want string }{
		"number":   {`{"id":"body","name":"本文","type":"integer","encrypted":true}`, "", "", "", "only for string / text"},
		"computed": {`{"id":"body","name":"本文","type":"string","encrypted":true,"computed":{"ref":"title"}}`, "", "", "", "computed or hidden"},
		"hidden":   {`{"id":"body","name":"本文","type":"string","encrypted":true,"hidden":true}`, "", "", "", "computed or hidden"},
		"default":  {`{"id":"body","name":"本文","type":"text","encrypted":true,"defaultValue":"x"}`, "", "", "", "cannot have a defaultValue"},
		"shared":   {encBody, "shared", "", "", "only for apps whose data is private"},
		"public":   {encBody, "public", "", "", "only for apps whose data is private"},
		"filter": {encBody, "", `,{"type":"stat","label":"件数","entity":"diary","aggregate":"count","filter":{"field":"body","op":"eq","value":"x"}}`, "",
			`filter.field "body" is encrypted`},
		"sort": {encBody, "", `,{"type":"list","entity":"diary","item":{"title":"title"},"sort":{"field":"body","direction":"asc"}}`, "",
			`sort.field "body" is encrypted`},
		"chart groupBy": {encBody, "", `,{"type":"chart","entity":"diary","kind":"bar","groupBy":"body","aggregate":"count"}`, "",
			`groupBy "body" is encrypted`},
		"expr agg": {encBody, "", `,{"type":"stat","label":"最新","value":{"agg":"latest","entity":"diary","field":"body"}}`, "",
			"agg reads encrypted field diary.body"},
		"computed reads": {encBody + `,{"id":"len","name":"長さ","type":"integer","computed":{"op":"len","args":[{"ref":"body"}]}}`, "", "", "",
			"reads encrypted field diary.body"},
		"server rule": {encBody, "", "", `,"serverRules":[{"id":"r","on":{"record":"created","entity":"diary"},"do":[{"notify":{"to":{"kind":"owner"},"title":"日記","body":"{body}"}}]}]`,
			"server rule reads encrypted field diary.body"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(encryptedDoc(tc.field, tc.scope, tc.extra, tc.rules)))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

// The online AI never gets the value: aiButton input / {{field}} and the
// aiChat conversation are refused; inputEntity needs another readable field.
func TestEncryptedFieldNotForAI(t *testing.T) {
	app := &App{Entities: []Entity{{ID: "diary", Fields: []Field{{ID: "body", Type: FieldText, Encrypted: true}, {ID: "title", Type: FieldString}}}}}
	ents := map[string]*Entity{"diary": &app.Entities[0]}
	var probs []string
	add := func(f string, a ...any) { probs = append(probs, f) }
	form := ents["diary"]
	checkEncryptedComponent("a", form, &Component{Type: CompAiButton, Input: "body"}, ents, add)
	checkEncryptedComponent("b", form, &Component{Type: CompAiButton, Input: "title", Prompt: "{{ body }} を読む"}, ents, add)
	checkEncryptedComponent("c", nil, &Component{Type: CompAiChat, Entity: "diary", TextField: "body"}, ents, add)
	checkEncryptedComponent("d", nil, &Component{Type: CompNearbyShare, Entity: "diary"}, ents, add)
	checkEncryptedComponent("ok", form, &Component{Type: CompAiButton, Input: "title", Prompt: "{{title}}"}, ents, add)
	if len(probs) != 4 {
		t.Fatalf("problems = %v", probs)
	}
	var chat []string
	only := &Entity{ID: "secret", Fields: []Field{{ID: "body", Type: FieldText, Encrypted: true}}}
	checkAIChatInput("x", &Component{InputEntity: "secret"}, map[string]*Entity{"secret": only}, func(f string, a ...any) { chat = append(chat, f) })
	if len(chat) != 1 {
		t.Fatalf("inputEntity with only encrypted fields must be refused: %v", chat)
	}
}

func TestEncryptedNotificationLint(t *testing.T) {
	app := &App{App: AppMeta{Notifications: []Notification{{ID: "n", Entity: "diary", TitleField: "body"}}},
		Entities: []Entity{{ID: "diary", Fields: []Field{{ID: "body", Type: FieldText, Encrypted: true}}}}}
	l := &linter{app: app, entities: map[string]*Entity{"diary": &app.Entities[0]}}
	l.encryptedNotifications()
	if len(l.out) != 1 || !strings.Contains(l.out[0], `titleField "body" is encrypted`) {
		t.Fatalf("lint = %v", l.out)
	}
}
