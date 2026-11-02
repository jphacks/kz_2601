package dsl

import (
	"strings"
	"testing"
)

// subsApp: a subscription list with a renewal date (#266).
func subsApp() App {
	app := memoApp()
	app.Entities = append(app.Entities, Entity{ID: "subs", Name: "s", Fields: []Field{
		{ID: "name", Name: "n", Type: FieldString},
		{ID: "renew_on", Name: "r", Type: FieldDate},
		{ID: "at", Name: "t", Type: FieldString},
	}})
	return app
}

func nextOcc(date Expr, unit Expr) *Expr {
	return &Expr{Kind: ExprOp, Op: "nextOccurrence", Args: []Expr{date, unit}}
}

func lit(v any) Expr { return Expr{Kind: ExprLiteral, Lit: v} }

// #266: repeat monthly / yearly on a record's date, and nextOccurrence.
func TestRepeatNotificationValid(t *testing.T) {
	app := subsApp()
	app.App.Notifications = []Notification{
		{ID: "renew", Title: "更新日", Repeat: "monthly", Entity: "subs", Field: "renew_on", OffsetMinutes: -1440, TitleField: "name"},
		{ID: "bday", Title: "誕生日", Repeat: "yearly", Entity: "memo", Field: "date"},
	}
	ref := Expr{Kind: ExprRef, Ref: "renew_on"}
	app.Entities[1].Fields = append(app.Entities[1].Fields,
		Field{ID: "next", Name: "x", Type: FieldDate, Computed: nextOcc(ref, lit("monthly"))},
		Field{ID: "days_left", Name: "d", Type: FieldInteger, Computed: &Expr{Kind: ExprOp, Op: "daysUntil", Args: []Expr{*nextOcc(ref, lit("yearly"))}}},
	)
	if err := app.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestRepeatNotificationErrors(t *testing.T) {
	app := subsApp()
	app.App.Notifications = []Notification{
		{ID: "fixed", Title: "t", Repeat: "monthly", Time: "09:00"},
		{ID: "alarm", Title: "t", Repeat: "yearly", Entity: "subs", Field: "at"},
	}
	ref := Expr{Kind: ExprRef, Ref: "renew_on"}
	app.Entities[1].Fields = append(app.Entities[1].Fields,
		Field{ID: "weekly", Name: "w", Type: FieldDate, Computed: nextOcc(ref, lit("weekly"))},
		Field{ID: "dyn", Name: "y", Type: FieldDate, Computed: nextOcc(ref, Expr{Kind: ExprRef, Ref: "name"})},
		Field{ID: "num", Name: "u", Type: FieldDate, Computed: nextOcc(ref, lit(1.0))},
		Field{ID: "notdate", Name: "z", Type: FieldDate, Computed: nextOcc(lit(3.0), lit("monthly"))},
		Field{ID: "asnum", Name: "q", Type: FieldInteger, Computed: nextOcc(ref, lit("monthly"))},
	)
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		"app.notifications[0]: repeat monthly needs entity + field",
		`repeat yearly needs a date or datetime field ("at" is a HH:MM string`,
		`"weekly".computed.nextOccurrence.args[1]: must be the string "monthly" or "yearly"`,
		`"dyn".computed.nextOccurrence.args[1]: must be the string`,
		`"num".computed.nextOccurrence.args[1]: must be the string`,
		"nextOccurrence expects a date but got a number",
		"yields a date but the field type is integer",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

// The button action (scheduleNotification) has no record, so monthly is rejected there too.
func TestRepeatOnActionRejected(t *testing.T) {
	app := subsApp()
	app.Screens[0].Components = []Component{{Type: CompButton, Label: "x", Action: &Action{
		Type: ActionScheduleNotif, Notification: &Notification{ID: "f", Title: "t", Repeat: "yearly", Time: "09:00"},
	}}}
	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), "repeat yearly needs entity + field") {
		t.Fatalf("got %v", err)
	}
}
