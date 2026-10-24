package dsl

import (
	"strings"
	"testing"
)

func progressBarDoc(extra string) []byte {
	return []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"none"},"initialScreen":"home","entities":[],
	"screens":[{"id":"home","title":"t","components":[{"type":"progressBar","label":"達成率","value":0.5` + extra + `}]}]}`)
}

// #140: tone tells whether 100% is good (goal) or bad (budget).
func TestProgressBarTone(t *testing.T) {
	for _, tone := range []string{"", `,"tone":"positive"`, `,"tone":"negative"`} {
		app, err := Parse(progressBarDoc(tone))
		if err != nil {
			t.Fatalf("tone %q: %v", tone, err)
		}
		if tone != "" && app.Screens[0].Components[0].Tone == "" {
			t.Fatalf("tone %q not decoded", tone)
		}
	}
	_, err := Parse(progressBarDoc(`,"tone":"good"`))
	if err == nil || !strings.Contains(err.Error(), "tone") {
		t.Fatalf("want tone error, got %v", err)
	}
}
