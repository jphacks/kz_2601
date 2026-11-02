package dsl

import (
	"encoding/json"
	"strings"
	"testing"
)

func whenExpr(t *testing.T, s string) *Expr {
	t.Helper()
	var e Expr
	if err := json.Unmarshal([]byte(s), &e); err != nil {
		t.Fatalf("expr %s: %v", s, err)
	}
	return &e
}

// #265: 在庫(elapsed の合計)が 5 を下回ったら知らせる。
func TestWhenNotificationValid(t *testing.T) {
	app := memoApp()
	app.App.Notifications = []Notification{{
		ID: "low", Title: "在庫が少なくなりました", Body: "補充しましょう", Repeat: "once",
		When: whenExpr(t, `{"op":"lt","args":[{"agg":"sum","entity":"memo","field":"elapsed"},5]}`),
	}}
	if err := app.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	raw, _ := json.Marshal(app.App.Notifications[0])
	if !strings.Contains(string(raw), `"when":{"op":"lt"`) {
		t.Errorf("json lacks when: %s", raw)
	}
	var back Notification
	if err := json.Unmarshal(raw, &back); err != nil || back.When == nil {
		t.Fatalf("round trip: %v %+v", err, back)
	}
}

func TestWhenNotificationErrors(t *testing.T) {
	app := memoApp()
	sum := `{"agg":"sum","entity":"memo","field":"elapsed"}`
	app.App.Notifications = []Notification{
		{ID: "a", Title: "t", Repeat: "once", Time: "09:00", When: whenExpr(t, `{"op":"lt","args":[`+sum+`,5]}`)},
		{ID: "b", Title: "t", Repeat: "daily", When: whenExpr(t, `{"op":"lt","args":[`+sum+`,5]}`)},
		{ID: "c", Title: "t", Repeat: "once", Entity: "memo", Field: "date", When: whenExpr(t, `{"op":"lt","args":[`+sum+`,5]}`)},
		{ID: "d", Title: "t", Repeat: "once", When: whenExpr(t, sum)},                                        // number, not a condition
		{ID: "e", Title: "t", Repeat: "once", When: whenExpr(t, `{"op":"lt","args":[{"ref":"elapsed"},5]}`)}, // no row
		{ID: "f", Title: "t", Repeat: "once", When: whenExpr(t, `{"op":"lt","args":[{"agg":"sum","entity":"nope","field":"x"},5]}`)},
	}
	app.Screens[0].Components = []Component{{Type: CompButton, Label: "x", Action: &Action{
		Type: ActionScheduleNotif, Notification: &Notification{ID: "g", Title: "t", Repeat: "once", When: whenExpr(t, `{"op":"lt","args":[`+sum+`,5]}`)},
	}}}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		"app.notifications[0]: a when notification fires when the condition turns true; remove time",
		`app.notifications[1]: a when notification needs repeat once`,
		"app.notifications[2]: a when notification fires when the condition turns true; remove entity/field",
		"app.notifications[3].when: condition must be true/false (got number)",
		`app.notifications[4].when`,
		`app.notifications[5].when`,
		"notification when (condition) belongs in app.notifications",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}
