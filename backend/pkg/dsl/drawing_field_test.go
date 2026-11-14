package dsl

import (
	"strings"
	"testing"
)

func drawingApp(fieldType, components string) []byte {
	return []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"none"},"initialScreen":"home",
	"entities":[{"id":"sketch","name":"スケッチ","fields":[
		{"id":"title","name":"タイトル","type":"string"},
		{"id":"art","name":"絵","type":"` + fieldType + `"}
	]}],
	"screens":[
		{"id":"home","title":"スケッチ","components":[{"type":"list","entity":"sketch","item":{"title":"title","image":"art"},"onItemTap":{"type":"navigate","screen":"form"}}]},
		{"id":"form","title":"t","entity":"sketch","components":[` + components + `]}]}`)
}

// drawingField は保存される photo field に束縛する(絵は写真と同じ画像で入る)。
func TestDrawingField(t *testing.T) {
	app, err := Parse(drawingApp("photo", `
		{"type":"textField","field":"title"},
		{"type":"drawingField","field":"art","label":"手書き","area":"A2:F6"},
		{"type":"image","field":"art"}`))
	if err != nil {
		t.Fatalf("valid drawing app: %v", err)
	}
	c := app.Screens[1].Components[1]
	if c.Type != CompDrawingField || c.Field != "art" || c.Label != "手書き" {
		t.Fatalf("decoded = %+v", c)
	}
	if _, err := Parse(drawingApp("photo", `{"type":"drawingField","field":"art"}`)); err != nil {
		t.Fatalf("label omitted: %v", err)
	}

	for name, tc := range map[string]struct{ ft, comps, want string }{
		"on a string field": {"string", `{"type":"drawingField","field":"art"}`, "must be a stored photo field"},
		"on a video field":  {"video", `{"type":"drawingField","field":"art"}`, "must be a stored photo field"},
		"missing field":     {"photo", `{"type":"drawingField","field":"nope"}`, "not in entity"},
		"no field":          {"photo", `{"type":"drawingField"}`, ""},
		"unknown key":       {"photo", `{"type":"drawingField","field":"art","color":"red"}`, ""},
	} {
		_, err := Parse(drawingApp(tc.ft, tc.comps))
		if err == nil {
			t.Errorf("%s: want error", name)
		} else if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// screen.entity が無ければ描いた絵を入れる下書きが無い。
func TestDrawingFieldNeedsFormEntity(t *testing.T) {
	raw := strings.Replace(string(drawingApp("photo", `{"type":"textField","field":"title"}`)),
		`"components":[{"type":"list"`, `"components":[{"type":"drawingField","field":"art"},{"type":"list"`, 1)
	if _, err := Parse([]byte(raw)); err == nil || !strings.Contains(err.Error(), "requires screen.entity") {
		t.Fatalf("want requires screen.entity, got %v", err)
	}
}
