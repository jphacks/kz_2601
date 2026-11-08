package dsl

import (
	"strings"
	"testing"
)

// tripApp adds a trip entity whose computed field depart (of type typ) is expr.
func tripApp(t *testing.T, typ FieldType, expr string) App {
	t.Helper()
	app := shoppingApp(t)
	app.Entities = append(app.Entities, Entity{
		ID: "trip", Name: "予定", Fields: []Field{
			{ID: "arrive", Name: "着きたい時刻", Type: FieldString},
			{ID: "arrive_at", Name: "着く日時", Type: FieldDatetime},
			{ID: "day", Name: "日", Type: FieldDate},
			{ID: "travel_min", Name: "移動時間", Type: FieldInteger, Unit: "分"},
			{ID: "ok", Name: "済", Type: FieldBoolean},
			{ID: "depart", Name: "出発", Type: typ, Computed: mustExpr(t, expr)},
		},
	})
	return app
}

func TestAddMinutesValid(t *testing.T) {
	for _, c := range []struct {
		typ  FieldType
		expr string
	}{
		{FieldString, `{"op":"addMinutes","args":[{"ref":"arrive"},{"op":"neg","args":[{"op":"coalesce","args":[{"ref":"travel_min"},0]}]}]}`},
		{FieldString, `{"op":"addMinutes","args":["09:00",-30]}`},
		{FieldDate, `{"op":"addMinutes","args":[{"ref":"arrive_at"},{"op":"neg","args":[{"ref":"travel_min"}]}]}`},
		{FieldDate, `{"op":"addMinutes","args":[{"ref":"day"},600]}`},
		{FieldBoolean, `{"op":"lt","args":[{"op":"addMinutes","args":[{"ref":"arrive_at"},-30]},{"op":"now","args":[]}]}`},
	} {
		app := tripApp(t, c.typ, c.expr)
		if err := app.Validate(); err != nil {
			t.Errorf("%s: %v", c.expr, err)
		}
	}
}

func TestAddMinutesInvalid(t *testing.T) {
	for _, c := range []struct {
		typ  FieldType
		expr string
		want string
	}{
		{FieldString, `{"op":"addMinutes","args":[{"ref":"travel_min"},10]}`, `addMinutes expects a date but got a number (pass a "HH:mm" time string`},
		{FieldString, `{"op":"addMinutes","args":[{"ref":"ok"},10]}`, `addMinutes expects a date but got a boolean`},
		{FieldString, `{"op":"addMinutes","args":[{"ref":"arrive"},{"ref":"arrive"}]}`, `addMinutes expects a number but got a string`},
		{FieldString, `{"op":"addMinutes","args":[{"ref":"arrive"}]}`, `op addMinutes takes exactly 2 args`},
		// a datetime stays a date: it does not fit a string field
		{FieldString, `{"op":"addMinutes","args":[{"ref":"arrive_at"},10]}`, `computed expression yields a date but the field type is string`},
		{FieldNumber, `{"op":"addMinutes","args":[{"ref":"arrive_at"},10]}`, `computed expression yields a date but the field type is number`},
	} {
		app := tripApp(t, c.typ, c.expr)
		err := app.Validate()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want %q, got %v", c.expr, c.want, err)
		}
	}
}
