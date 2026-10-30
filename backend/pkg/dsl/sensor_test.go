package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sensorApp() App {
	app := Initial()
	app.Entities = []Entity{{ID: "reading", Name: "記録", Fields: []Field{
		{ID: "v", Name: "値", Type: FieldNumber},
		{ID: "at", Name: "時刻", Type: FieldDatetime, Required: true, DefaultValue: "now"},
		{ID: "memo", Name: "メモ", Type: FieldString},
	}}}
	app.Screens = append(app.Screens, Screen{ID: "detail", Title: "d"})
	return app
}

func fp(v float64) *float64 { return &v }

func TestSensorValueValid(t *testing.T) {
	app := sensorApp()
	app.Screens[0].Components = []Component{
		{Type: CompSensorValue, Sensor: "tilt", Label: "傾き", Style: "level",
			Threshold: &SensorThreshold{Above: fp(30), Message: "傾きすぎ", Action: &Action{Type: ActionNavigate, Screen: "detail"}}},
		{Type: CompSensorValue, Sensor: "acceleration", Label: "揺れ", Style: "gauge", Min: fp(0), Max: fp(2),
			Entity: "reading", Field: "v", DateField: "at", Threshold: &SensorThreshold{Above: fp(0.5), Save: true}},
		{Type: CompSensorValue, Sensor: "pressure", Label: "気圧", Entity: "reading", Field: "v", Seconds: 60, Limit: 120},
		{Type: CompSensorValue, Sensor: "roll", Label: "左右", Threshold: &SensorThreshold{Above: fp(20), Below: fp(-20)}},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("valid sensorValue rejected: %v", err)
	}
}

func TestSensorValueSemanticErrors(t *testing.T) {
	app := sensorApp()
	app.Entities = append(app.Entities, Entity{ID: "strict", Name: "s", Fields: []Field{
		{ID: "v", Name: "値", Type: FieldNumber},
		{ID: "need", Name: "必須", Type: FieldString, Required: true},
	}})
	s := func(mod func(c *Component)) Component {
		c := Component{Type: CompSensorValue, Sensor: "tilt", Label: "傾き"}
		mod(&c)
		return c
	}
	app.Screens[0].Components = []Component{
		s(func(c *Component) { c.Sensor = "temperature" }),
		s(func(c *Component) { c.Label = "" }),
		s(func(c *Component) { c.Style = "dial" }),
		s(func(c *Component) { c.Sensor = "pressure"; c.Style = "level" }),
		s(func(c *Component) { c.Min = fp(10); c.Max = fp(5) }),
		s(func(c *Component) { c.Entity = "reading"; c.Field = "v"; c.Seconds = 7200 }),
		s(func(c *Component) { c.Entity = "reading"; c.Field = "v"; c.Seconds = 1; c.Limit = 5000 }),
		s(func(c *Component) { c.Entity = "reading"; c.Field = "v"; c.Limit = 10 }),
		s(func(c *Component) { c.Threshold = &SensorThreshold{} }),
		s(func(c *Component) { c.Threshold = &SensorThreshold{Above: fp(1), Below: fp(2)} }),
		s(func(c *Component) {
			c.Threshold = &SensorThreshold{Above: fp(1), Action: &Action{Type: ActionDeleteRecord, Entity: "reading"}}
		}),
		s(func(c *Component) {
			c.Threshold = &SensorThreshold{Above: fp(1), Action: &Action{Type: ActionNavigate, Screen: "gone"}}
		}),
		s(func(c *Component) { c.Seconds = 10 }),
		s(func(c *Component) { c.Entity = "nope"; c.Field = "v" }),
		s(func(c *Component) { c.Entity = "reading"; c.Field = "memo" }),
		s(func(c *Component) { c.Entity = "reading"; c.Field = "v"; c.DateField = "memo" }),
		s(func(c *Component) { c.Entity = "strict"; c.Field = "v" }),
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		"sensorValue sensor must be tilt",
		"sensorValue needs label",
		"style must be number, gauge or level",
		"style level is only for sensor tilt",
		"sensorValue max must be > min",
		"sensorValue seconds must be 1..3600",
		"sensorValue limit must be 1..1000",
		"limit is the count of interval saves",
		"threshold needs above or below",
		"threshold below must be < above",
		"threshold action must be navigate",
		`threshold.action: navigate target screen "gone"`,
		"need entity (where the values are saved)",
		`sensorValue entity "nope" does not exist`,
		`sensorValue field "memo" must be a stored number field`,
		`sensorValue dateField "memo" must be a stored datetime field`,
		`required field "need" of entity "strict" needs a defaultValue`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

// The schema keeps temperature out: a DSL asking for it never reaches Validate.
func TestSensorValueSchemaRejectsTemperature(t *testing.T) {
	raw := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home",
"screens":[{"id":"home","title":"h","components":[{"type":"sensorValue","sensor":"temperature","label":"気温"}]}]}`)
	if _, err := Parse(raw); err == nil {
		t.Fatal("sensor temperature should fail schema validation")
	}
}

// The bundled example parses (lint: TestLintExamplesClean; the logging entity needs no form).
func TestSensorValueExample(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "sensors.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatalf("sensors.json: %v", err)
	}
	sensors := map[string]bool{}
	for _, c := range app.Screens[0].Components {
		if c.Type == CompSensorValue {
			sensors[c.Sensor] = true
		}
	}
	for _, want := range []string{"tilt", "pressure", "altitude"} {
		if !sensors[want] {
			t.Errorf("sensors.json lacks sensor %s", want)
		}
	}
}
