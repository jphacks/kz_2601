package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckAudio(t *testing.T) {
	memo := &dsl.App{
		Entities: []dsl.Entity{{ID: "memo", Fields: []dsl.Field{
			{ID: "title", Type: dsl.FieldString},
			{ID: "voice", Type: dsl.FieldAudio},
		}}},
		Screens: []dsl.Screen{
			{ID: "home", Components: []dsl.Component{{Type: dsl.CompList, Entity: "memo", Item: &dsl.ListItem{Title: "title", Trailing: "voice"}}}},
			{ID: "form", Entity: "memo", Components: []dsl.Component{{Type: dsl.CompVStack, Children: []dsl.Component{
				{Type: dsl.CompTextField, Field: "title"},
				{Type: dsl.CompAudioRecorder, Field: "voice"},
			}}}},
		},
	}
	recs, rows := audioUses(memo)
	if len(recs) != 1 || recs[0] != "form:voice" || len(rows) != 1 || rows[0] != "memo:voice" {
		t.Fatalf("audioUses = %v %v", recs, rows)
	}
	if miss := checkAudio(map[string]any{"audio": true, "audioInList": true}, memo); miss != nil {
		t.Fatalf("memo: %v", miss)
	}
	if miss := checkAudio(map[string]any{"audio": false}, memo); len(miss) != 1 {
		t.Fatalf("audio=false should miss: %v", miss)
	}
	if miss := checkAudio(map[string]any{}, memo); miss != nil {
		t.Fatalf("no expect key: %v", miss)
	}

	// a recorder on a string field (or a list of text) does not count
	memo.Entities[0].Fields[1].Type = dsl.FieldString
	if miss := checkAudio(map[string]any{"audio": true, "audioInList": true}, memo); len(miss) != 2 {
		t.Fatalf("string field should miss both: %v", miss)
	}
}
