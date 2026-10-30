package dsl

import (
	"strings"
	"testing"
)

func audioApp(fieldType, components string) []byte {
	return []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"none"},"initialScreen":"home",
	"entities":[{"id":"memo","name":"ボイスメモ","fields":[
		{"id":"title","name":"タイトル","type":"string"},
		{"id":"voice","name":"録音","type":"` + fieldType + `"}
	]}],
	"screens":[
		{"id":"home","title":"メモ","components":[{"type":"list","entity":"memo","item":{"title":"title","trailing":"voice"},"onItemTap":{"type":"navigate","screen":"form"}}]},
		{"id":"form","title":"t","entity":"memo","components":[` + components + `]}]}`)
}

// #420: audioRecorder は保存される audio field に束縛し、maxDuration は 1..7200 秒。
func TestAudioRecorder(t *testing.T) {
	app, err := Parse(audioApp("audio", `
		{"type":"textField","field":"title"},
		{"type":"audioRecorder","field":"voice","label":"録音","maxDuration":3600}`))
	if err != nil {
		t.Fatalf("valid audio app: %v", err)
	}
	c := app.Screens[1].Components[1]
	if c.Type != CompAudioRecorder || c.MaxDuration != 3600 || c.Field != "voice" {
		t.Fatalf("decoded = %+v", c)
	}
	if f := app.EntityByID("memo").FieldByID("voice"); f.Type != FieldAudio {
		t.Fatalf("field type = %s", f.Type)
	}
	if _, err := Parse(audioApp("audio", `{"type":"audioRecorder","field":"voice"}`)); err != nil {
		t.Fatalf("maxDuration omitted: %v", err)
	}

	for name, tc := range map[string]struct{ ft, comps, want string }{
		"on a string field":    {"string", `{"type":"audioRecorder","field":"voice"}`, "must be a stored audio field"},
		"missing field":        {"audio", `{"type":"audioRecorder","field":"nope"}`, "not in entity"},
		"maxDuration too long": {"audio", `{"type":"audioRecorder","field":"voice","maxDuration":7201}`, ""},
		"maxDuration zero":     {"audio", `{"type":"audioRecorder","field":"voice","maxDuration":0}`, ""},
		"unknown key":          {"audio", `{"type":"audioRecorder","field":"voice","quality":"high"}`, ""},
		"videoField on audio":  {"audio", `{"type":"videoField","field":"voice"}`, "must be type video"},
	} {
		_, err := Parse(audioApp(tc.ft, tc.comps))
		if err == nil {
			t.Errorf("%s: want error", name)
		} else if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Without screen.entity there is no draft to record into.
func TestAudioRecorderNeedsFormEntity(t *testing.T) {
	raw := strings.Replace(string(audioApp("audio", `{"type":"textField","field":"title"}`)),
		`"components":[{"type":"list"`, `"components":[{"type":"audioRecorder","field":"voice"},{"type":"list"`, 1)
	if _, err := Parse([]byte(raw)); err == nil || !strings.Contains(err.Error(), "requires screen.entity") {
		t.Fatalf("want requires screen.entity, got %v", err)
	}
}
