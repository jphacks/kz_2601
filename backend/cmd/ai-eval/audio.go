package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Audio recording (#420) expect keys, checked by checkAudio:
//
//	audio (bool)       some form binds an audioRecorder to an audio field
//	audioInList (bool) some list shows an audio field (item.trailing / subtitle),
//	                   i.e. saved recordings are playable from the rows
//
// Cases: internal/ai/eval/audio_cases.json (-cases).

// audioUses lists "screen:field" of recorders and "entity:field" of list rows
// that show an audio field.
func audioUses(a *dsl.App) (recorders, rows []string) {
	isAudio := func(entity, field string) bool {
		e := a.EntityByID(entity)
		if e == nil || field == "" {
			return false
		}
		f := e.FieldByID(field)
		return f != nil && f.Type == dsl.FieldAudio
	}
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				switch {
				case x.Type == dsl.CompAudioRecorder && isAudio(s.Entity, x.Field):
					recorders = append(recorders, s.ID+":"+x.Field)
				case x.Type == dsl.CompList && x.Item != nil:
					for _, f := range []string{x.Item.Trailing, x.Item.Subtitle} {
						if isAudio(x.Entity, f) {
							rows = append(rows, x.Entity+":"+f)
						}
					}
				}
			}
		}
	}
	return recorders, rows
}

func checkAudio(exp map[string]any, a *dsl.App) []string {
	recorders, rows := audioUses(a)
	var miss []string
	if want, ok := exp["audio"].(bool); ok && (len(recorders) > 0) != want {
		miss = append(miss, fmt.Sprintf("audio=%v", recorders))
	}
	if want, ok := exp["audioInList"].(bool); ok && (len(rows) > 0) != want {
		miss = append(miss, fmt.Sprintf("audioInList=%v", rows))
	}
	return miss
}
