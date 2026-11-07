package appconfig

import (
	"strings"
	"testing"
)

// text {{fieldId}} is a record value: the web page shows a dash, never the
// placeholder (records are not shown, #253).
func TestShareViewTextPlaceholder(t *testing.T) {
	raw := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"form",
"entities":[{"id":"memo","name":"m","fields":[{"id":"body","name":"b","type":"text"},{"id":"kw","name":"k","type":"string"}]}],
"screens":[{"id":"form","title":"f","entity":"memo","components":[{"type":"text","text":"エラー: {{body}}","highlight":{"field":"kw"}}]}]}`)
	got, err := renderSharePage(buildWebPage("", raw, nil))
	if err != nil {
		t.Fatal(err)
	}
	if s := string(got); strings.Contains(s, "{{body}}") || !strings.Contains(s, "エラー: —") {
		t.Errorf("placeholder not replaced:\n%s", s)
	}
}

// Without screen.entity the text is shown as written (as before).
func TestShareViewTextLiteralWithoutEntity(t *testing.T) {
	raw := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home",
"entities":[],"screens":[{"id":"home","title":"h","components":[{"type":"text","text":"{{name}} さん"}]}]}`)
	got, err := renderSharePage(buildWebPage("", raw, nil))
	if err != nil {
		t.Fatal(err)
	}
	if s := string(got); !strings.Contains(s, "{{name}} さん") {
		t.Errorf("literal text changed:\n%s", s)
	}
}

// Unknown, photo and hidden fields stay literal on the web too.
func TestShareViewTextLiteralFields(t *testing.T) {
	raw := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"form",
"entities":[{"id":"memo","name":"m","fields":[{"id":"body","name":"b","type":"text"},{"id":"pic","name":"p","type":"photo"},{"id":"secret","name":"s","type":"string","hidden":true}]}],
"screens":[{"id":"form","title":"f","entity":"memo","components":[{"type":"text","text":"{{body}}|{{zzz}}|{{pic}}|{{secret}}"}]}]}`)
	got, err := renderSharePage(buildWebPage("", raw, nil))
	if err != nil {
		t.Fatal(err)
	}
	if s := string(got); !strings.Contains(s, "—|{{zzz}}|{{pic}}|{{secret}}") {
		t.Errorf("only the visible scalar field is replaced:\n%s", s)
	}
}
