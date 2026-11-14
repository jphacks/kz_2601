package dsl

import (
	"encoding/json"
	"strings"
	"testing"
)

// 保存した場所に着いたら知らせる・時刻を毎回少しずらす。
func TestNearNotificationValid(t *testing.T) {
	app := memoApp()
	app.App.Notifications = []Notification{
		{ID: "arrive", Title: "近くに来ました", Body: "降りる準備を", Repeat: "once", Near: &NotificationNear{Entity: "memo", LocationField: "place", RadiusM: 500}},
		{ID: "every", Title: "近くです", Repeat: "daily", Near: &NotificationNear{Entity: "memo", LocationField: "place"}},
		{ID: "rand", Title: "写真の時間", Repeat: "daily", Time: "19:00", JitterMinutes: 60},
		{ID: "early", Title: "期限が近い", Repeat: "once", Entity: "memo", Field: "date", OffsetMinutes: -30, JitterMinutes: 30},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	raw, _ := json.Marshal(app.App.Notifications[0])
	if !strings.Contains(string(raw), `"near":{"entity":"memo","locationField":"place","radiusM":500}`) {
		t.Errorf("json: %s", raw)
	}
	var back Notification
	if err := json.Unmarshal(raw, &back); err != nil || back.Near == nil || back.Near.RadiusM != 500 {
		t.Fatalf("round trip: %v %+v", err, back)
	}
}

// near watches the user's own places: a shared / public entity or a hidden
// location field is rejected.
func TestNearNotificationPrivateVisibleOnly(t *testing.T) {
	for name, c := range map[string]struct {
		edit func(*App)
		want string
	}{
		"shared": {func(a *App) { a.Entities[0].Scope = ScopeShared }, `app.notifications[0].near: entity "memo" is shared; near watches the user's own places`},
		"public": {func(a *App) { a.Entities[0].Scope = ScopePublic }, `app.notifications[0].near: entity "memo" is public; near watches the user's own places`},
		"hidden": {func(a *App) { a.Entities[0].Fields[1].Hidden = true }, `app.notifications[0].near: locationField "place" is hidden`},
	} {
		app := memoApp()
		app.App.Notifications = []Notification{{ID: "a", Title: "t", Repeat: "once", Near: &NotificationNear{Entity: "memo", LocationField: "place"}}}
		c.edit(&app)
		err := app.Validate()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestNearNotificationErrors(t *testing.T) {
	app := memoApp()
	near := func(e, f string, r int) *NotificationNear {
		return &NotificationNear{Entity: e, LocationField: f, RadiusM: r}
	}
	app.App.Notifications = []Notification{
		{ID: "a", Title: "t", Repeat: "once", Time: "09:00", Near: near("memo", "place", 0)},
		{ID: "b", Title: "t", Repeat: "weekly", Weekday: "mon", Near: near("memo", "place", 0)},
		{ID: "c", Title: "t", Repeat: "once", Near: near("nope", "place", 0)},
		{ID: "d", Title: "t", Repeat: "once", Near: near("memo", "note", 0)},
		{ID: "e", Title: "t", Repeat: "once", Near: near("memo", "gone", 0)},
		{ID: "f", Title: "t", Repeat: "once", Near: near("memo", "place", 50)},
		{ID: "g", Title: "t", Repeat: "daily", Time: "09:00", JitterMinutes: 90},
		{ID: "h", Title: "t", Repeat: "once", JitterMinutes: 5, Near: near("memo", "place", 0)},
	}
	app.Screens[0].Components = []Component{{Type: CompButton, Label: "x", Action: &Action{
		Type: ActionScheduleNotif, Notification: &Notification{ID: "i", Title: "t", Repeat: "once", Time: "09:00", JitterMinutes: 5, Near: near("memo", "place", 0)},
	}}}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		"app.notifications[0]: a near notification fires on arriving at the place; remove time",
		`app.notifications[1]: a near notification needs repeat once (first arrival only) or daily (every arrival); got "weekly"`,
		`app.notifications[2].near: entity "nope" does not exist`,
		`app.notifications[3].near: locationField "note" must be a location field (got string)`,
		`app.notifications[4].near: locationField "gone" does not exist`,
		"app.notifications[5].near: radiusM 50 out of range (100-2000)",
		"app.notifications[6]: jitterMinutes 90 out of range (1-60)",
		"app.notifications[7]: jitterMinutes shifts a time",
		"near notification belongs in app.notifications, not scheduleNotification",
		"jitterMinutes belongs in app.notifications, not scheduleNotification",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}
