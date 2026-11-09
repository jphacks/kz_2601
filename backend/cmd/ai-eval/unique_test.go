package main

import (
	"encoding/json"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckUnique(t *testing.T) {
	booking := &dsl.App{Entities: []dsl.Entity{
		{ID: "slot"},
		{ID: "booking", Unique: []string{"slot"}},
	}}
	author := &dsl.App{Entities: []dsl.Entity{{ID: "vote", Unique: []string{"event", dsl.RecordAuthor}}}}
	none := &dsl.App{Entities: []dsl.Entity{{ID: "slot"}, {ID: "booking", Unique: []string{}}}}
	for _, tc := range []struct {
		name string
		exp  map[string]any
		app  *dsl.App
		miss int
	}{
		{"field unique wanted", map[string]any{"unique": true}, booking, 0},
		{"$author unique wanted", map[string]any{"unique": true}, author, 0},
		{"empty unique is none", map[string]any{"unique": true}, none, 1},
		{"no entities", map[string]any{"unique": true}, &dsl.App{}, 1},
		{"unwanted", map[string]any{"unique": false}, booking, 1},
		{"none wanted none", map[string]any{"unique": false}, none, 0},
		{"no key", map[string]any{}, booking, 0},
	} {
		if miss := checkUnique(tc.exp, tc.app); len(miss) != tc.miss {
			t.Errorf("%s: want %d misses, got %v", tc.name, tc.miss, miss)
		}
	}
}

// The parsed shape: entity.unique is a flat list of field ids (dsl.go).
func TestCheckUniqueParsed(t *testing.T) {
	var app dsl.App
	if err := json.Unmarshal([]byte(`{"entities":[{"id":"booking","unique":["slot","$author"]}]}`), &app); err != nil {
		t.Fatal(err)
	}
	if miss := checkUnique(map[string]any{"unique": true}, &app); miss != nil {
		t.Errorf("parsed unique: %v", miss)
	}
}
