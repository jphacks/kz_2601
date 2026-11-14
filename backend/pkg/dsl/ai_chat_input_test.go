package dsl

import (
	"encoding/json"
	"strings"
	"testing"
)

// inputChatApp: mealChatApp with the aiChat reading input (limit n) and a
// photo-only entity in place of strict (6 entities at most).
func inputChatApp(input string, limit int) App {
	app := mealChatApp(nil)
	app.Entities[len(app.Entities)-1] = Entity{ID: "album", Name: "写真", Fields: []Field{
		{ID: "pic", Name: "写真", Type: FieldPhoto},
		{ID: "key", Name: "答え", Type: FieldString, Hidden: true},
	}}
	app.Screens[0].Components[0].InputEntity = input
	app.Screens[0].Components[0].Limit = limit
	return app
}

func TestAIChatInputValidPasses(t *testing.T) {
	for _, tc := range []struct {
		input string
		limit int
	}{{"meal", 0}, {"meal", 20}, {"board", 5}, {"wall", 1}, {"prefs", 0}} {
		app := inputChatApp(tc.input, tc.limit)
		if err := app.Validate(); err != nil {
			t.Fatalf("%v: %v", tc, err)
		}
		raw, err := json.Marshal(app)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Parse(raw); err != nil {
			t.Fatalf("schema rejects aiChat inputEntity %v: %v", tc, err)
		}
	}
}

func TestAIChatInputSemanticErrors(t *testing.T) {
	for want, app := range map[string]App{
		`aiChat inputEntity "nope" does not exist`:                    inputChatApp("nope", 0),
		`aiChat inputEntity must differ from the conversation entity`: inputChatApp("turn", 0),
		`aiChat inputEntity "album" has no field the AI can read`:     inputChatApp("album", 0),
		`aiChat limit 21 must be 1..20`:                               inputChatApp("meal", 21),
		`aiChat limit -1 must be 1..20`:                               inputChatApp("meal", -1),
		`aiChat limit is the number of inputEntity records`:           inputChatApp("", 5),
	} {
		err := app.Validate()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q, got %v", want, err)
		}
	}
}

// The schema bounds limit too (the generator sees it before Validate).
func TestAIChatInputSchemaBoundsLimit(t *testing.T) {
	app := inputChatApp("meal", 0)
	raw, _ := json.Marshal(app)
	bad := strings.Replace(string(raw), `"inputEntity":"meal"`, `"inputEntity":"meal","limit":50`, 1)
	if _, err := Parse([]byte(bad)); err == nil {
		t.Fatal("schema must reject aiChat limit 50")
	}
}
