package dsl

import (
	"strings"
	"testing"
)

// #172: streak / doneInDays read the checked-day history of a daily toggle.
func TestDailyHistoryOps(t *testing.T) {
	app := loadExample(t, "medicine") // declares streak + last7 computed fields
	wantValid(t, app)
	if got := DailyToggleFields(&app); !got["medicine"]["taken_on"] {
		t.Fatalf("DailyToggleFields = %v", got)
	}

	setComputed := func(a *App, expr string) {
		for i := range a.Entities[0].Fields {
			if a.Entities[0].Fields[i].ID == "streak" {
				a.Entities[0].Fields[i].Computed = mustExpr(t, expr)
			}
		}
	}
	ok := []string{
		`{"op":"streak","args":[{"ref":"taken_on"}]}`,
		`{"op":"doneInDays","args":[{"ref":"taken_on"},30]}`,
		`{"op":"sub","args":[7,{"op":"doneInDays","args":[{"ref":"taken_on"},7]}]}`,
	}
	for _, e := range ok {
		app := loadExample(t, "medicine")
		setComputed(&app, e)
		wantValid(t, app)
	}

	bad := []struct{ name, expr, want string }{
		{"value not ref", `{"op":"streak","args":["2026-10-24"]}`, `streak needs {"ref": <daily check field>}`},
		{"not a date", `{"op":"streak","args":[{"ref":"name"}]}`, `streak needs the stored date field of a daily check`},
		{"n missing", `{"op":"doneInDays","args":[{"ref":"taken_on"}]}`, `op doneInDays takes exactly 2 args`},
		{"n zero", `{"op":"doneInDays","args":[{"ref":"taken_on"},0]}`, `integer literal 1..366`},
		{"n expr", `{"op":"doneInDays","args":[{"ref":"taken_on"},{"op":"add","args":[3,4]}]}`, `integer literal 1..366`},
		{"lookup", `{"op":"streak","args":[{"ref":"id"}]}`, `streak needs {"ref": <daily check field>}`},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			app := loadExample(t, "medicine")
			setComputed(&app, tc.expr)
			wantProblem(t, app, tc.want)
		})
	}

	// a date field that is not a resets daily toggle has no history
	t.Run("not a daily toggle", func(t *testing.T) {
		app := loadExample(t, "medicine")
		app.Entities[0].Fields = append(app.Entities[0].Fields, Field{ID: "started", Name: "開始日", Type: FieldDate})
		setComputed(&app, `{"op":"streak","args":[{"ref":"started"}]}`)
		wantProblem(t, app, `"started" is not a daily check`)
	})

	// stat.value has no row
	t.Run("stat value", func(t *testing.T) {
		app := withStat(t, loadExample(t, "medicine"), Component{Value: mustExpr(t, `{"op":"streak","args":[{"ref":"taken_on"}]}`)})
		wantProblem(t, app, `is not allowed here`)
	})
}

func TestSchemaHasDailyHistoryOps(t *testing.T) {
	s := string(SchemaJSON())
	for _, op := range []string{"streak", "doneInDays"} {
		if _, ok := Ops[op]; !ok {
			t.Errorf("Ops lacks %s", op)
		}
		if !strings.Contains(s, `"`+op+`"`) {
			t.Errorf("schema lacks %s", op)
		}
	}
}
