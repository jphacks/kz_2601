package dsl

// sensorValue (#419): a live iPhone sensor value (CoreMotion device motion /
// CMAltimeter), measured only while the screen is open. With entity + field it
// saves the current value (button, every `seconds`, or when threshold fires).
// Temperature / humidity do not exist: the iPhone has no such sensor.

// SensorThreshold fires once when the value enters the condition.
type SensorThreshold struct {
	Above   *float64 `json:"above,omitempty"`
	Below   *float64 `json:"below,omitempty"`
	Message string   `json:"message,omitempty"`
	Save    bool     `json:"save,omitempty"`
	Action  *Action  `json:"action,omitempty"`
}

// Sensors is the closed set of sensorValue.sensor values.
var Sensors = map[string]bool{
	"tilt": true, "pitch": true, "roll": true, "acceleration": true, "rotation": true, "pressure": true, "altitude": true,
}

// Sensor logging bounds (iOS clamps the same way).
const (
	MaxSensorLogSeconds = 3600
	MaxSensorLogLimit   = 1000
)

var sensorStyles = map[string]bool{"": true, "number": true, "gauge": true, "level": true}

// sensorActions are the actions a threshold may run without a tap.
var sensorActions = map[ActionType]bool{ActionNavigate: true, ActionScheduleNotif: true, ActionCancelNotif: true}

func checkSensorValue(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	if !Sensors[c.Sensor] {
		add("%s: sensorValue sensor must be tilt|pitch|roll|acceleration|rotation|pressure|altitude (temperature / humidity do not exist on iPhone)", where)
	}
	if c.Label == "" {
		add("%s: sensorValue needs label", where)
	}
	if !sensorStyles[c.Style] {
		add("%s: sensorValue style must be number, gauge or level", where)
	} else if c.Style == "level" && c.Sensor != "tilt" {
		add("%s: sensorValue style level is only for sensor tilt", where)
	}
	if c.Min != nil && c.Max != nil && *c.Max <= *c.Min {
		add("%s: sensorValue max must be > min", where)
	}
	if c.Seconds < 0 || c.Seconds > MaxSensorLogSeconds {
		add("%s: sensorValue seconds must be 1..%d", where, MaxSensorLogSeconds)
	}
	if c.Limit < 0 || c.Limit > MaxSensorLogLimit {
		add("%s: sensorValue limit must be 1..%d", where, MaxSensorLogLimit)
	}
	if c.Limit > 0 && c.Seconds == 0 {
		add("%s: sensorValue limit is the count of interval saves; set seconds or remove limit", where)
	}
	if t := c.Threshold; t != nil {
		if t.Above == nil && t.Below == nil {
			add("%s: sensorValue threshold needs above or below", where)
		}
		if t.Above != nil && t.Below != nil && *t.Below >= *t.Above {
			add("%s: sensorValue threshold below must be < above (fires outside the band)", where)
		}
		if t.Action != nil && !sensorActions[t.Action.Type] {
			add("%s: sensorValue threshold action must be navigate, scheduleNotification or cancelNotification", where)
		}
	}
	saves := c.Seconds > 0 || (c.Threshold != nil && c.Threshold.Save)
	if c.Entity == "" {
		if c.Field != "" || c.DateField != "" || saves {
			add("%s: sensorValue field / dateField / seconds / threshold.save need entity (where the values are saved)", where)
		}
		return
	}
	e := entities[c.Entity]
	if e == nil {
		add("%s: sensorValue entity %q does not exist", where, c.Entity)
		return
	}
	if f := e.FieldByID(c.Field); f == nil {
		add("%s: sensorValue field %q not in entity %q (fields: %s)", where, c.Field, e.ID, fieldIDs(e))
	} else if (f.Type != FieldNumber && f.Type != FieldInteger) || f.Computed != nil {
		add("%s: sensorValue field %q must be a stored number field", where, c.Field)
	}
	if c.DateField != "" {
		if f := e.FieldByID(c.DateField); f == nil {
			add("%s: sensorValue dateField %q not in entity %q (fields: %s)", where, c.DateField, e.ID, fieldIDs(e))
		} else if f.Type != FieldDatetime || f.Computed != nil {
			add("%s: sensorValue dateField %q must be a stored datetime field", where, c.DateField)
		}
	}
	for _, f := range e.Fields {
		if f.Required && f.Computed == nil && f.DefaultValue == nil && f.ID != c.Field && f.ID != c.DateField {
			add("%s: sensorValue saves without a form, so required field %q of entity %q needs a defaultValue (or make it optional)", where, f.ID, e.ID)
		}
	}
}
