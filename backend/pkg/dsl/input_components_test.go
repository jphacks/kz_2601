package dsl

import (
	"encoding/json"
	"strings"
	"testing"
)

// inputApp builds a one-entity form screen holding the given components.
func inputApp(components string) []byte {
	return []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"none"},"initialScreen":"form",
	"entities":[{"id":"log","name":"記録","fields":[
		{"id":"score","name":"点","type":"integer"},
		{"id":"weight","name":"重さ","type":"number"},
		{"id":"note","name":"メモ","type":"text"},
		{"id":"pin","name":"PIN","type":"string"},
		{"id":"wake","name":"起床","type":"string"},
		{"id":"at","name":"日時","type":"datetime"},
		{"id":"color","name":"色","type":"string"},
		{"id":"mood","name":"気分","type":"enum","options":["良","普","悪"]},
		{"id":"done","name":"完了","type":"boolean"}
	]}],
	"screens":[{"id":"form","title":"t","entity":"log","components":[` + components + `]}]}`)
}

func TestInputComponentsDecodeAndValidate(t *testing.T) {
	raw := inputApp(`
		{"type":"slider","id":"s","field":"score","label":"点","min":0,"max":10,"step":1},
		{"type":"stepper","field":"score","min":0,"max":20},
		{"type":"stepper","field":"weight","step":0.5},
		{"type":"textEditor","field":"note","placeholder":"自由に","minLines":5},
		{"type":"secureField","field":"pin","placeholder":"****"},
		{"type":"timePicker","field":"wake","label":"起床"},
		{"type":"timePicker","field":"at","label":"日時"},
		{"type":"colorPicker","field":"color","label":"色"},
		{"type":"rating","field":"score","max":5},
		{"type":"rating","field":"weight"},
		{"type":"picker","field":"mood","style":"segmented"}`)
	app, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cs := app.Screens[0].Components
	if cs[0].Type != CompSlider || cs[0].Min == nil || *cs[0].Min != 0 || *cs[0].Max != 10 || *cs[0].Step != 1 {
		t.Fatalf("slider props lost: %+v", cs[0])
	}
	if cs[3].MinLines != 5 {
		t.Fatalf("textEditor minLines lost: %+v", cs[3])
	}
	// explicit min:0 must survive marshal (pointer, not omitempty-zero)
	out, _ := json.Marshal(cs[0])
	if !strings.Contains(string(out), `"min":0`) {
		t.Fatalf("min:0 dropped on marshal: %s", out)
	}
}

func TestInputComponentsSemanticErrors(t *testing.T) {
	cases := map[string]struct {
		comp string
		want string
	}{
		"slider on text":      {`{"type":"slider","field":"note","min":0,"max":1}`, "must be one of"},
		"slider min>=max":     {`{"type":"slider","field":"score","min":5,"max":5}`, "must be < max"},
		"slider step<=0":      {`{"type":"slider","field":"score","min":0,"max":5,"step":0}`, "step must be > 0"},
		"stepper bad range":   {`{"type":"stepper","field":"score","min":3,"max":1}`, "must be < max"},
		"rating on enum":      {`{"type":"rating","field":"mood"}`, "must be one of"},
		"rating max too big":  {`{"type":"rating","field":"score","max":11}`, "1..10"},
		"textEditor on int":   {`{"type":"textEditor","field":"score"}`, "must be one of"},
		"secureField on bool": {`{"type":"secureField","field":"done"}`, "must be one of"},
		"timePicker on int":   {`{"type":"timePicker","field":"score"}`, "must be one of"},
		"colorPicker on int":  {`{"type":"colorPicker","field":"score"}`, "must be one of"},
		"missing field":       {`{"type":"slider","field":"nope","min":0,"max":1}`, "not in entity"},
		"unknown prop":        {`{"type":"slider","field":"score","min":0,"max":1,"thumb":"big"}`, "thumb"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(inputApp(tc.comp))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestInputComponentsRequireFormScreen(t *testing.T) {
	raw := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"none"},"initialScreen":"home","entities":[],
	"screens":[{"id":"home","title":"t","components":[{"type":"rating","field":"score"}]}]}`)
	_, err := Parse(raw)
	if err == nil || !strings.Contains(err.Error(), "requires screen.entity") {
		t.Fatalf("expected screen.entity error, got %v", err)
	}
}
