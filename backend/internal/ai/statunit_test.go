package ai

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Shape of the #173 demo output: sum over reps (unit 回) labelled with the unit.
const statUnitDSL = `{"schemaVersion":1,"app":{"name":"筋トレ","icon":"figure.strengthtraining.traditional","theme":{"primaryColor":"#E53935"},"voiceEntry":"floating"},"initialScreen":"home",
"entities":[{"id":"workout","name":"記録","fields":[{"id":"menu","name":"メニュー","type":"string"},{"id":"reps","name":"回数","type":"number","unit":"回"},{"id":"weight","name":"重さ","type":"number","unit":"kg"},{"id":"date","name":"日付","type":"date","defaultValue":"now"}]}],
"screens":[{"id":"home","title":"筋トレ","components":[
{"type":"stat","id":"sum","area":"A1:B2","label":"今日の合計回数(回)","entity":"workout","aggregate":"sum","field":"reps"},
{"type":"stat","id":"max","area":"C1:D2","label":"最大重量 （KG）","entity":"workout","aggregate":"max","field":"weight"},
{"type":"stat","id":"cnt","area":"E1:F2","label":"セット数(回)","value":{"agg":"count","entity":"workout"}},
{"type":"section","id":"sec","area":"A3:F4","children":[{"type":"stat","id":"avg","label":"平均回数（回）","entity":"workout","aggregate":"avg","field":"reps"}]},
{"type":"list","id":"l","area":"A5:F12","entity":"workout","item":{"title":"menu","trailing":"reps"}}]}]}`

func TestFixStatUnitLabels(t *testing.T) {
	if _, err := dsl.Parse([]byte(statUnitDSL)); err != nil {
		t.Fatalf("fixture must validate: %v", err)
	}
	out := fixStatUnitLabels([]byte(statUnitDSL))
	app, err := dsl.Parse(out)
	if err != nil {
		t.Fatalf("fixed DSL must validate: %v", err)
	}
	labels := map[string]string{}
	var walk func(cs []dsl.Component)
	walk = func(cs []dsl.Component) {
		for _, c := range cs {
			labels[c.ID] = c.Label
			walk(c.Children)
		}
	}
	walk(app.Screens[0].Components)
	// count (value mode) keeps the unit in the label: its value is a bare number
	want := map[string]string{"sum": "今日の合計回数", "max": "最大重量", "cnt": "セット数(回)", "avg": "平均回数"}
	for id, w := range want {
		if labels[id] != w {
			t.Errorf("%s: label %q, want %q", id, labels[id], w)
		}
	}
	if again := fixStatUnitLabels(out); string(again) != string(out) {
		t.Error("already clean DSL must come back unchanged")
	}
}

func TestTrimUnitSuffix(t *testing.T) {
	for _, tc := range []struct {
		label, unit, want string
		ok                bool
	}{
		{"合計(回)", "回", "合計", true},
		{"(回)", "回", "(回)", false},
		{"合計(kg)", "回", "合計(kg)", false},
		{"Total (reps)", "reps", "Total", true},
		{"合計回数", "回", "合計回数", false},
	} {
		got, ok := trimUnitSuffix(tc.label, tc.unit)
		if got != tc.want || ok != tc.ok {
			t.Errorf("trimUnitSuffix(%q, %q) = %q, %v", tc.label, tc.unit, got, ok)
		}
	}
}
