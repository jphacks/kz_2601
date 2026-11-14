package dsl

import (
	"strings"
	"testing"
)

const callDoc = `{"schemaVersion":1,"app":{"name":"部活の連絡","icon":"phone","voiceEntry":"floating"},"initialScreen":"home",
"entities":[{"id":"post","name":"連絡","scope":"shared","titleField":"body","fields":[{"id":"body","name":"本文","type":"string","required":true}]}],
"screens":[{"id":"home","title":"連絡","components":[
 {"type":"chatThread","area":"A1:F8","entity":"post","textField":"body"},
 {"type":"callButton","area":"A9:F10","label":"みんなで話す"},
 {"type":"callButton","area":"C11:F12","label":"ビデオで話す","video":true,"room":"video-1"}]}]}`

func TestCallButtonValid(t *testing.T) {
	app, err := Parse([]byte(callDoc))
	if err != nil {
		t.Fatal(err)
	}
	if probs := app.Lint(); len(probs) > 0 {
		t.Errorf("lint: %v", probs)
	}
	if c := app.CallButton(""); c == nil || c.Video {
		t.Errorf("the app's room: %+v", c)
	}
	if c := app.CallButton("video-1"); c == nil || !c.Video {
		t.Errorf("room video-1: %+v", c)
	}
	if app.CallButton("other") != nil {
		t.Error("a room no callButton names")
	}
}

func TestCallButtonOnlyInSharedApps(t *testing.T) {
	_, err := Parse([]byte(strings.Replace(callDoc, `"scope":"shared",`, "", 1)))
	if err == nil || !strings.Contains(err.Error(), "callButton works only in an app shared with other people") {
		t.Fatalf("private app: %v", err)
	}
	if _, err := Parse([]byte(strings.Replace(callDoc, `"scope":"shared"`, `"scope":"public"`, 1))); err != nil {
		t.Errorf("public app: %v", err)
	}
}

func TestCallButtonRoom(t *testing.T) {
	app, _ := Parse([]byte(callDoc))
	app.Screens[0].Components[2].Room = "Bad Room"
	if err := app.Validate(); err == nil || !strings.Contains(err.Error(), "callButton room") {
		t.Fatalf("bad room: %v", err)
	}
	if !ValidCallRoom("") || !ValidCallRoom("team-a_1") || ValidCallRoom("-x") || ValidCallRoom(strings.Repeat("a", 33)) {
		t.Error("ValidCallRoom")
	}
}

func TestCallButtonLintSameRoom(t *testing.T) {
	app, _ := Parse([]byte(strings.Replace(callDoc, `,"room":"video-1"`, "", 1)))
	probs := strings.Join(app.Lint(), "\n")
	if !strings.Contains(probs, "two callButtons join the same room") {
		t.Errorf("lint = %s", probs)
	}
}
