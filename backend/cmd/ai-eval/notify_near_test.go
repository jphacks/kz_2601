package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestNotifyNearMetrics(t *testing.T) {
	app := parseExample(t, "warikan")
	if miss := checkNotifyNear(map[string]any{"notifyNear": true, "notifyNearRepeat": "once", "jitter": true}, app); len(miss) != 3 {
		t.Fatalf("no near / jitter must miss all: %v", miss)
	}
	app.Entities = append(app.Entities, dsl.Entity{ID: "places", Name: "場所", Fields: []dsl.Field{
		{ID: "name", Name: "名前", Type: dsl.FieldString}, {ID: "spot", Name: "場所", Type: dsl.FieldLocation},
	}})
	app.App.Notifications = append(app.App.Notifications,
		dsl.Notification{ID: "bad", Title: "t", Repeat: "daily", Near: &dsl.NotificationNear{Entity: "places", LocationField: "name"}},
		dsl.Notification{ID: "arrive", Title: "t", Repeat: "once", Near: &dsl.NotificationNear{Entity: "places", LocationField: "spot"}},
		dsl.Notification{ID: "rand", Title: "t", Repeat: "daily", Time: "20:00", JitterMinutes: 60})
	if miss := checkNotifyNear(map[string]any{"notifyNear": true, "notifyNearRepeat": "once", "jitter": true}, app); len(miss) != 0 {
		t.Fatal(miss)
	}
	if miss := checkNotifyNear(map[string]any{"notifyNearRepeat": "daily"}, app); len(miss) != 1 {
		t.Fatalf("wrong repeat must miss: %v", miss)
	}
}
