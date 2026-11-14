package dsl

import (
	"encoding/json"
	"strings"
	"testing"
)

// Notification.alarm: 止めるまで鳴るアラーム(iOS AlarmKit)。時刻・レコードの時刻の通知に付けられる。
func TestAlarmNotificationValid(t *testing.T) {
	app := memoApp()
	app.App.Notifications = []Notification{
		{ID: "wake", Title: "起きる時間です", Repeat: "daily", Time: "07:00", Alarm: true},
		{ID: "gym", Title: "ジム", Repeat: "weekly", Weekday: "mon", Time: "06:30", Alarm: true},
		{ID: "rec", Title: "アラーム", Repeat: "daily", Entity: "memo", Field: "note", TitleField: "note", Alarm: true},
	}
	app.Screens[0].Components = []Component{{Type: CompButton, Label: "x", Action: &Action{
		Type: ActionScheduleNotif, Notification: &Notification{ID: "nap", Title: "昼寝おわり", Repeat: "once", Time: "13:30", Alarm: true},
	}}}
	if err := app.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	raw, _ := json.Marshal(app.App.Notifications[0])
	if !strings.Contains(string(raw), `"alarm":true`) {
		t.Errorf("json lacks alarm: %s", raw)
	}
	var back Notification
	if err := json.Unmarshal(raw, &back); err != nil || !back.Alarm {
		t.Fatalf("round trip: %v %+v", err, back)
	}
	// 普通の通知には出さない(古い DSL とバイト単位で同じ)
	plain, _ := json.Marshal(Notification{ID: "n", Title: "t", Repeat: "once"})
	if strings.Contains(string(plain), "alarm") {
		t.Errorf("alarm should be omitted: %s", plain)
	}
}

func TestAlarmNotificationRejectsWhen(t *testing.T) {
	app := memoApp()
	app.App.Notifications = []Notification{{
		ID: "low", Title: "t", Repeat: "once", Alarm: true,
		When: whenExpr(t, `{"op":"lt","args":[{"agg":"sum","entity":"memo","field":"elapsed"},5]}`),
	}}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	if joined := strings.Join(ve.Problems, "\n"); !strings.Contains(joined, "app.notifications[0]: a when notification fires when the condition turns true; remove alarm") {
		t.Errorf("missing alarm+when problem in:\n%s", joined)
	}
}
