package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Sensor (#419) expect keys: sensor ([name, ...]: each is the sensor of some
// sensorValue), sensorSave (bool: some sensorValue saves to an entity),
// sensorThreshold (bool: some sensorValue has a threshold), sensorDecline
// (bool: no sensorValue and the summary honestly says the iPhone cannot
// measure it, e.g. temperature). Cases: internal/ai/eval/sensor_cases.json.

func sensorValues(a *dsl.App) []dsl.Component {
	var out []dsl.Component
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if x.Type == dsl.CompSensorValue {
					out = append(out, x)
				}
			}
		}
	}
	return out
}

// declinesSensor: the summary says there is no sensor / it cannot measure.
func declinesSensor(summary string) bool {
	if strings.Contains(summary, "センサー") &&
		(strings.Contains(summary, "測れ") || strings.Contains(summary, "でき") || strings.Contains(summary, "無い") || strings.Contains(summary, "ない")) {
		return true
	}
	l := strings.ToLower(summary)
	return strings.Contains(l, "sensor") && (strings.Contains(l, "can't") || strings.Contains(l, "cannot") || strings.Contains(l, "no "))
}

func checkSensor(exp map[string]any, a *dsl.App, summary string) []string {
	svs := sensorValues(a)
	var miss []string
	if want, ok := exp["sensor"].([]any); ok {
		var got []string
		for _, c := range svs {
			got = append(got, c.Sensor)
		}
		for _, w := range want {
			if !slices.Contains(got, w.(string)) {
				miss = append(miss, fmt.Sprintf("sensor=%v", got))
				break
			}
		}
	}
	if want, ok := exp["sensorSave"].(bool); ok {
		got := slices.ContainsFunc(svs, func(c dsl.Component) bool { return c.Entity != "" })
		if got != want {
			miss = append(miss, fmt.Sprintf("sensorSave=%v", got))
		}
	}
	if want, ok := exp["sensorThreshold"].(bool); ok {
		got := slices.ContainsFunc(svs, func(c dsl.Component) bool { return c.Threshold != nil })
		if got != want {
			miss = append(miss, fmt.Sprintf("sensorThreshold=%v", got))
		}
	}
	if want, ok := exp["sensorDecline"].(bool); ok {
		got := len(svs) == 0 && declinesSensor(summary)
		if got != want {
			miss = append(miss, fmt.Sprintf("sensorDecline=%v (sensorValue=%d)", got, len(svs)))
		}
	}
	return miss
}
