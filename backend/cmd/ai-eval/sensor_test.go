package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckSensor(t *testing.T) {
	above := 30.0
	app := &dsl.App{Screens: []dsl.Screen{{ID: "home", Components: []dsl.Component{
		{Type: dsl.CompSensorValue, Sensor: "tilt", Threshold: &dsl.SensorThreshold{Above: &above}},
		{Type: dsl.CompVStack, Children: []dsl.Component{{Type: dsl.CompSensorValue, Sensor: "pressure", Entity: "reading"}}},
	}}}}
	pass := map[string]any{"sensor": []any{"tilt", "pressure"}, "sensorSave": true, "sensorThreshold": true, "sensorDecline": false}
	if m := checkSensor(pass, app, ""); len(m) != 0 {
		t.Errorf("want no misses, got %v", m)
	}
	if m := checkSensor(map[string]any{"sensor": []any{"altitude"}}, app, ""); len(m) != 1 {
		t.Errorf("missing altitude should miss, got %v", m)
	}

	empty := &dsl.App{}
	decline := map[string]any{"sensorDecline": true}
	if m := checkSensor(decline, empty, "iPhone には温度のセンサーが無いため自動では測れません。"); len(m) != 0 {
		t.Errorf("honest decline should pass, got %v", m)
	}
	if m := checkSensor(decline, empty, "温度を記録するアプリです。"); len(m) != 1 {
		t.Errorf("silent manual app should miss, got %v", m)
	}
	if m := checkSensor(decline, app, "センサーが無いので測れません"); len(m) != 1 {
		t.Errorf("faking with another sensor should miss, got %v", m)
	}
	if bad := validateExpect(pass); len(bad) != 0 {
		t.Errorf("sensor keys not registered: %v", bad)
	}
}
