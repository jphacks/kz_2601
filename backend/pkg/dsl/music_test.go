package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func musicApp(cs ...Component) *App {
	app := Initial()
	app.Screens[0].Components = cs
	return &app
}

func TestMusicPlayerValid(t *testing.T) {
	for _, c := range []Component{
		{Type: CompMusicPlayer},
		{Type: CompMusicPlayer, Title: "BGM", AllowRequests: true},
		{Type: CompMusicPlayer, AllowRequests: true, AutoAccept: true},
	} {
		if err := musicApp(c).Validate(); err != nil {
			t.Errorf("%+v rejected: %v", c, err)
		}
	}
}

func TestMusicPlayerSemanticErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		app  *App
		want string
	}{
		"autoAccept alone": {musicApp(Component{Type: CompMusicPlayer, AutoAccept: true}), "autoAccept needs allowRequests"},
		"entity":           {musicApp(Component{Type: CompMusicPlayer, Entity: "x"}), "remove entity/field"},
		"two players": {musicApp(Component{Type: CompMusicPlayer},
			Component{Type: CompSection, Children: []Component{{Type: CompMusicPlayer}}}), "musicPlayer appears 2 times"},
	} {
		err := tc.app.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want %q, got %v", name, tc.want, err)
		}
	}
}

// The schema is closed: unknown keys on musicPlayer never reach Validate.
func TestMusicPlayerSchemaClosed(t *testing.T) {
	raw := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home","entities":[],
"screens":[{"id":"home","title":"h","components":[{"type":"musicPlayer","service":"spotify"}]}]}`)
	if _, err := Parse(raw); err == nil {
		t.Fatal("musicPlayer with an unknown key should fail schema validation")
	}
}

func TestMusicPlayerExample(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "drive_music.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatalf("drive_music.json: %v", err)
	}
	c := app.Screens[0].Components[0]
	if c.Type != CompMusicPlayer || !c.AllowRequests {
		t.Fatalf("drive_music home should be a musicPlayer with allowRequests: %+v", c)
	}
	if w := app.Lint(); len(w) > 0 {
		t.Errorf("lint: %v", w)
	}
}
