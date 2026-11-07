package ratelimit

import (
	"net/http/httptest"
	"testing"
)

// aiButton length long weighs TextLong whatever the task.
func TestWeightTextLong(t *testing.T) {
	w := DefaultWeights
	for body, want := range map[string]int{
		`{"instruction":"x","length":"long"}`:               3,
		`{"task":"summarize","input":"x","length":"long"}`:  3,
		`{"task":"summarize","input":"x","length":"short"}`: 2,
		`{"instruction":"x","length":"short"}`:              1,
	} {
		r := httptest.NewRequest("POST", "/v1/text", nil)
		if got := w.Weight(r, []byte(body)); got != want {
			t.Errorf("%s: weight %d, want %d", body, got, want)
		}
	}
	cfg, err := ConfigFromEnv(func(k string) string {
		if k == "RATE_LIMIT_WEIGHT_TEXT_LONG" {
			return "5"
		}
		return ""
	})
	if err != nil || cfg.Weights.TextLong != 5 || DefaultWeights.TextLong != 3 {
		t.Fatalf("config: %+v %v", cfg.Weights, err)
	}
}
