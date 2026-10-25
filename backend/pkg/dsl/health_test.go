package dsl

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHealthStatSemanticErrors(t *testing.T) {
	goal, zero := 8000.0, 0.0
	app := memoApp()
	app.Screens[0].Components = []Component{
		{Type: CompHealthStat, Metric: "steps", Label: "今日の歩数", Target: &goal}, // ok
		{Type: CompHealthStat, Metric: "weight", Period: "week", Label: "体重"},  // ok
		{Type: CompHealthStat, Metric: "sleep", Label: "睡眠"},                   // bad metric
		{Type: CompHealthStat, Metric: "steps", Period: "month", Label: "歩数"},  // bad period
		{Type: CompHealthStat, Metric: "steps"},                                // no label
		{Type: CompHealthStat, Metric: "weight", Label: "体重", Target: &goal},   // target on weight
		{Type: CompHealthStat, Metric: "steps", Label: "歩数", Target: &zero},    // target <= 0
		{Type: CompHealthStat, Metric: "steps", Label: "歩数", Entity: "memo"},   // entity not allowed
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		"healthStat metric must be steps or weight",
		"healthStat period must be today or week",
		"healthStat needs label",
		"healthStat target is only for metric steps",
		"healthStat target must be > 0",
		"healthStat reads device data; remove entity/field",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
	if len(ve.Problems) != 6 {
		t.Errorf("want 6 problems, got %v", ve.Problems)
	}
}

func TestHealthFieldSemanticErrors(t *testing.T) {
	cases := []struct {
		f    Field
		want string
	}{
		{Field{ID: "w", Name: "体重", Type: FieldNumber, Unit: "kg", Health: "weight"}, ""},
		{Field{ID: "w", Name: "体重", Type: FieldNumber, Health: "weight"}, ""},
		{Field{ID: "w", Name: "体重", Type: FieldString, Health: "weight"}, "health weight needs a stored number field"},
		{Field{ID: "w", Name: "体重", Type: FieldNumber, Unit: "lb", Health: "weight"}, "unit must be kg"},
		{Field{ID: "w", Name: "体重", Type: FieldNumber, Health: "height"}, "health must be weight"},
	}
	for _, tc := range cases {
		app := memoApp()
		app.Entities[0].Fields = append(app.Entities[0].Fields, tc.f)
		err := app.Validate()
		if tc.want == "" {
			if err != nil {
				t.Errorf("%+v: unexpected %v", tc.f, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%+v: want %q, got %v", tc.f, tc.want, err)
		}
	}
}

// The schema accepts healthStat and Field.health (and rejects unknown metrics).
func TestHealthSchema(t *testing.T) {
	app := memoApp()
	app.Entities[0].Fields = append(app.Entities[0].Fields, Field{ID: "w", Name: "体重", Type: FieldNumber, Unit: "kg", Health: "weight"})
	app.Screens[0].Components = []Component{{Type: CompHealthStat, Metric: "steps", Period: "week", Label: "1 週間の歩数"}}
	for i := range app.Screens {
		if app.Screens[i].Components == nil {
			app.Screens[i].Components = []Component{}
		}
	}
	raw, err := json.Marshal(app)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSchema(raw); err != nil {
		t.Fatalf("schema: %v", err)
	}
	bad := strings.Replace(string(raw), `"metric":"steps"`, `"metric":"sleep"`, 1)
	if ValidateSchema([]byte(bad)) == nil {
		t.Error("schema should reject metric sleep")
	}
}
