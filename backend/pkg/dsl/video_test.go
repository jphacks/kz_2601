package dsl

import (
	"strings"
	"testing"
)

func videoApp(fieldType, components string) []byte {
	return []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"none"},"initialScreen":"form",
	"entities":[{"id":"clip","name":"動画","fields":[
		{"id":"name","name":"種目","type":"string"},
		{"id":"video","name":"動画","type":"` + fieldType + `"}
	]}],
	"screens":[{"id":"form","title":"t","entity":"clip","components":[` + components + `]}]}`)
}

// #206: videoField / videoPlayer は video field に束縛し、maxDuration は秒。
func TestVideoComponents(t *testing.T) {
	app, err := Parse(videoApp("video", `
		{"type":"videoField","field":"video","label":"撮る","source":"both","maxDuration":30},
		{"type":"videoPlayer","field":"video","height":"medium"},
		{"type":"list","entity":"clip","item":{"title":"name","image":"video"}}`))
	if err != nil {
		t.Fatalf("valid video app: %v", err)
	}
	c := app.Screens[0].Components[0]
	if c.Type != CompVideoField || c.MaxDuration != 30 || c.Source != "both" {
		t.Fatalf("decoded = %+v", c)
	}
	if f := app.EntityByID("clip").FieldByID("video"); f.Type != FieldVideo {
		t.Fatalf("field type = %s", f.Type)
	}

	for name, tc := range map[string]struct{ ft, comps, want string }{
		"videoField on photo":    {"photo", `{"type":"videoField","field":"video"}`, "must be type video"},
		"videoPlayer on photo":   {"photo", `{"type":"videoPlayer","field":"video"}`, "must be type video"},
		"photoField on video":    {"video", `{"type":"photoField","field":"video"}`, "must be type photo"},
		"maxDuration too long":   {"video", `{"type":"videoField","field":"video","maxDuration":601}`, ""},
		"unknown videoField key": {"video", `{"type":"videoField","field":"video","quality":"high"}`, ""},
	} {
		_, err := Parse(videoApp(tc.ft, tc.comps))
		if err == nil {
			t.Errorf("%s: want error", name)
		} else if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
