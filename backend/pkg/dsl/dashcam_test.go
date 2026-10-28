package dsl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dashcamDoc(comps string) []byte {
	return []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home","entities":[],
"screens":[{"id":"home","title":"h","components":[` + comps + `]}]}`)
}

func TestDashcamValid(t *testing.T) {
	for _, comps := range []string{
		`{"type":"dashcam"}`,
		`{"type":"dashcam","title":"ドラレコ","keepMinutes":5,"maxStorageMB":1000,"impactG":3,"autoStart":["landscape","still","charging"],"audio":true}`,
	} {
		app, err := Parse(dashcamDoc(comps))
		if err != nil {
			t.Fatalf("valid dashcam rejected: %s: %v", comps, err)
		}
		if app.Screens[0].Components[0].Type != CompDashcam {
			t.Fatal("type not decoded")
		}
	}
}

// Keys round-trip through the Go struct (a missing field would drop them on save).
func TestDashcamRoundTrip(t *testing.T) {
	app, err := Parse(dashcamDoc(`{"type":"dashcam","keepMinutes":5,"maxStorageMB":1000,"impactG":3.5,"autoStart":["charging"],"audio":true}`))
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(app.Screens[0].Components[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"keepMinutes":5`, `"maxStorageMB":1000`, `"impactG":3.5`, `"autoStart":["charging"]`, `"audio":true`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("lost %s in %s", want, out)
		}
	}
}

func TestDashcamSchemaRanges(t *testing.T) {
	for _, comps := range []string{
		`{"type":"dashcam","keepMinutes":0}`,
		`{"type":"dashcam","keepMinutes":61}`,
		`{"type":"dashcam","maxStorageMB":100}`,
		`{"type":"dashcam","impactG":0.5}`,
		`{"type":"dashcam","autoStart":[]}`,
		`{"type":"dashcam","autoStart":["moving"]}`,
		`{"type":"dashcam","autoStart":["still","still"]}`,
		`{"type":"dashcam","entity":"x"}`,
	} {
		if _, err := Parse(dashcamDoc(comps)); err == nil {
			t.Errorf("should fail: %s", comps)
		}
	}
}

func TestDashcamSemanticErrors(t *testing.T) {
	app := Initial()
	app.Screens[0].Components = []Component{
		{Type: CompDashcam, AutoStart: []string{"moving", "still", "still"}},
		{Type: CompDashcam, Entity: "x"},
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		`dashcam autoStart "moving" must be landscape, still or charging`,
		`dashcam autoStart "still" is listed twice`,
		`dashcam keeps clips on the device; remove entity/field`,
		`only one dashcam per app`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

func TestDashcamExample(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "dashcam.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatalf("dashcam.json: %v", err)
	}
	c := app.Screens[0].Components[0]
	if c.Type != CompDashcam || len(c.AutoStart) != 2 {
		t.Fatalf("dashcam home should start on landscape + still: %+v", c)
	}
	if w := app.Lint(); len(w) != 0 {
		t.Errorf("lint: %v", w)
	}
}
