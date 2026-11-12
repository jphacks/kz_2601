package appconfig

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func loadExample(t *testing.T, name string) *dsl.App {
	t.Helper()
	raw, err := os.ReadFile("../../../docs/dsl/examples/" + name)
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// #1268: a shared app with musicPlayer opened on Android (which declares it
// as fallback only, #1510) keeps the musicPlayer, so Android's own stand-in
// (search by title in a music app, #879) has the title to work with.
func TestSubstituteKeepsMusicPlayerForAndroidFallback(t *testing.T) {
	app := loadExample(t, "drive_music.json")
	before := mustJSON(t, app)
	android := map[string]bool{"vstack": true, "text": true, "list": true} // no musicPlayer
	got := substituteUndeclared(app, android, fetchSubstitutes)
	if after := mustJSON(t, got); after != before {
		t.Fatalf("musicPlayer app changed for a client without musicPlayer:\n%s\n->\n%s", before, after)
	}
	if c := got.Screens[0].Components[0]; c.Type != dsl.CompMusicPlayer || c.Title != "ドライブ BGM" {
		t.Fatalf("component = %+v, want the musicPlayer with its title", c)
	}
}

func TestSubstituteNilDeclaredReturnsSameApp(t *testing.T) {
	app := loadExample(t, "drive_music.json")
	if got := substituteUndeclared(app, nil, fetchSubstitutes); got != app {
		t.Fatal("no declaration must leave the app as is")
	}
}

// The engine replaces undeclared types that have a stand-in (nested too),
// keeps id / area, and never touches the stored app.
func TestSubstituteReplacesUndeclaredKeepingPlace(t *testing.T) {
	app := &dsl.App{Screens: []dsl.Screen{{ID: "home", Components: []dsl.Component{
		{Type: dsl.CompMapView, ID: "map", Area: "A1:F4"},
		{Type: dsl.CompVStack, ID: "box", Area: "A5:F8", Children: []dsl.Component{
			{Type: dsl.CompMapView, ID: "inner"},
			{Type: dsl.CompMusicPlayer, ID: "player", Title: "BGM"},
		}},
	}}}}
	stored := mustJSON(t, app)
	table := map[dsl.ComponentType]substituteFunc{
		dsl.CompMapView: func(dsl.Component) *dsl.Component {
			return &dsl.Component{Type: dsl.CompList, ID: "x", Area: "B1"}
		},
		dsl.CompMusicPlayer: keepForClientFallback,
	}
	got := substituteUndeclared(app, map[string]bool{"vstack": true, "list": true}, table)

	want := []dsl.Component{
		{Type: dsl.CompList, ID: "map", Area: "A1:F4"},
		{Type: dsl.CompVStack, ID: "box", Area: "A5:F8", Children: []dsl.Component{
			{Type: dsl.CompList, ID: "inner"},
			{Type: dsl.CompMusicPlayer, ID: "player", Title: "BGM"},
		}},
	}
	if !reflect.DeepEqual(got.Screens[0].Components, want) {
		t.Fatalf("got %s\nwant %s", mustJSON(t, got.Screens[0].Components), mustJSON(t, want))
	}
	if mustJSON(t, app) != stored {
		t.Fatal("stored app was modified")
	}
	all := map[string]bool{"vstack": true, "mapView": true, "musicPlayer": true}
	if mustJSON(t, substituteUndeclared(app, all, table)) != stored {
		t.Fatal("declared types must stay")
	}
}

func TestFetchSubstitutesKeysAreSchemaTypes(t *testing.T) {
	schema := string(dsl.SchemaJSON())
	for typ := range fetchSubstitutes {
		if !strings.Contains(schema, `"`+string(typ)+`"`) {
			t.Errorf("fetchSubstitutes has %q, not in dsl.schema.json", typ)
		}
	}
}
