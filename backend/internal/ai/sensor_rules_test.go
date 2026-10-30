package ai

import (
	"strings"
	"testing"
)

// #419: the sensor rules reach the generation and chat prompts, and the
// catalog / chat index (built from the schema) describe sensorValue.
func TestSensorRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## 端末のセンサー(#419)", "sensor tilt・style level", "温度・気温・室温・体温・湿度は iPhone にセンサーが無いので測れない"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	for name, p := range map[string]string{"edit": chatEditSystemPrompt, "create": chatCreateSystemPrompt} {
		if !strings.Contains(p, "iPhone には温度・湿度のセンサーが無いので測れません") {
			t.Errorf("%s chat prompt lacks the temperature / humidity answer", name)
		}
	}
	if !strings.Contains(catalog, "\n- sensorValue: sensor(tilt") {
		t.Error("catalog lacks the sensorValue line with its required keys")
	}
	if !strings.Contains(chatCapabilityIndex, "sensorValue") {
		t.Error("chat capability index lacks sensorValue")
	}
}

// A review build cannot render sensorValue: the routing lines go, the honest
// "no temperature sensor" lines stay.
func TestSensorRulesRestricted(t *testing.T) {
	for name, p := range map[string]string{"generate": systemPrompt, "edit": chatEditSystemPrompt, "create": chatCreateSystemPrompt} {
		got := withoutNote(restrictPrompt(reviewCtx(), p))
		if strings.Contains(got, "」は sensorValue") {
			t.Errorf("%s: restricted prompt still routes to sensorValue", name)
		}
		if !strings.Contains(got, "湿度は iPhone にセンサーが無い") {
			t.Errorf("%s: restricted prompt lost the temperature / humidity decline", name)
		}
	}
}

func TestSensorValuePlainTerm(t *testing.T) {
	if got := plainTerms("sensorValue で傾きを出します", LocaleJA); strings.Contains(got, "sensorValue") {
		t.Errorf("plainTerms kept the type name: %q", got)
	}
}
