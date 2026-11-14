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
		{FieldDatetime, `{"op":"addMinutes","args":[{"ref":"arrive_at"},{"op":"neg","args":[{"ref":"travel_min"}]}]}`},
		{FieldDatetime, `{"op":"addMinutes","args":[{"ref":"day"},600]}`},
		{FieldDatetime, `{"op":"addMinutes","args":["2026-10-28T09:00:00+09:00",-45]}`},
		{FieldDatetime, `{"op":"addMinutes","args":[{"op":"addMinutes","args":[{"ref":"arrive_at"},10]},5]}`},
		{FieldBoolean, `{"op":"lt","args":[{"op":"addMinutes","args":[{"ref":"arrive_at"},-30]},{"op":"now","args":[]}]}`},
		{FieldBoolean, `{"op":"gt","args":[{"op":"addMinutes","args":[{"ref":"arrive_at"},-30]},"2026-10-28"]}`},
		{FieldNumber, `{"op":"daysBetween","args":[{"op":"addMinutes","args":[{"ref":"arrive_at"},-30]},{"op":"today","args":[]}]}`},
		// "HH:mm" chains and reads as a string / time
		{FieldString, `{"op":"addMinutes","args":[{"op":"addMinutes","args":[{"ref":"arrive"},10]},5]}`},
		{FieldNumber, `{"op":"hoursBetween","args":[{"op":"addMinutes","args":[{"ref":"arrive"},-30]},{"ref":"arrive"}]}`},
		{FieldBoolean, `{"op":"lt","args":[{"op":"addMinutes","args":["09:00",30]},{"ref":"arrive"}]}`},
		{FieldString, `{"op":"concat","args":[{"op":"addMinutes","args":["09:00",30]},"発"]}`},
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
		// a date / datetime gives a datetime: a string / date field would show only the date
		{FieldString, `{"op":"addMinutes","args":[{"ref":"arrive_at"},10]}`, `yields a datetime (addMinutes of a date / datetime) but the field type is string, which shows only the date; make it type datetime`},
		{FieldDate, `{"op":"addMinutes","args":[{"ref":"arrive_at"},10]}`, `but the field type is date, which shows only the date; make it type datetime`},
		{FieldDate, `{"op":"addMinutes","args":[{"ref":"day"},600]}`, `but the field type is date, which shows only the date`},
		{FieldString, `{"op":"addMinutes","args":["2026-10-28T09:00:00+09:00",-45]}`, `but the field type is string, which shows only the date`},
		{FieldString, `{"op":"coalesce","args":[{"op":"addMinutes","args":[{"ref":"arrive_at"},10]},""]}`, `but the field type is string, which shows only the date`},
		{FieldNumber, `{"op":"addMinutes","args":[{"ref":"arrive_at"},10]}`, `computed expression yields a datetime but the field type is number`},
		// a "HH:mm" time is not a date / number (these evaluated to nil)
		{FieldNumber, `{"op":"daysBetween","args":[{"op":"addMinutes","args":["09:00",10]},{"op":"today","args":[]}]}`, `daysBetween expects a date but got a "HH:mm" time`},
		{FieldNumber, `{"op":"add","args":[{"op":"addMinutes","args":[{"ref":"arrive"},10]},1]}`, `add expects a number but got a "HH:mm" time`},
		{FieldDatetime, `{"op":"addMinutes","args":[{"ref":"arrive"},10]}`, `computed expression yields a "HH:mm" time but the field type is datetime`},
		{FieldDate, `{"op":"addDays","args":[{"op":"addMinutes","args":["09:00",10]},1]}`, `addDays expects a date but got a "HH:mm" time`},
		{FieldBoolean, `{"op":"lt","args":[{"op":"addMinutes","args":["09:00",30]},{"ref":"day"}]}`, `lt compares a "HH:mm" time with a date`},
	} {
		app := tripApp(t, c.typ, c.expr)
		err := app.Validate()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want %q, got %v", c.expr, c.want, err)
		}
	}
}
