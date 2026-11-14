package ratelimit

import (
	"net/http/httptest"
	"testing"
)

// A photo (aiButton inputPhoto) weighs TextImage, or more when the task /
// length weighs more; image null is no photo.
func TestWeightTextImage(t *testing.T) {
	w := DefaultWeights
	w.TextLong = 4
	for body, want := range map[string]int{
		`{"instruction":"x","image":{"mediaType":"image/jpeg","data":"AA"}}`:                  3,
		`{"task":"summarize","image":{"mediaType":"image/jpeg","data":"AA"}}`:                 3,
		`{"task":"summarize","length":"long","image":{"mediaType":"image/jpeg","data":"AA"}}`: 4,
		`{"instruction":"x","image":null}`:                                                    1,
		`{"task":"summarize","input":"x"}`:                                                    2,
	} {
		r := httptest.NewRequest("POST", "/v1/text", nil)
		if got := w.Weight(r, []byte(body)); got != want {
			t.Errorf("%s: weight %d, want %d", body, got, want)
		}
	}
	cfg, err := ConfigFromEnv(func(k string) string {
		if k == "RATE_LIMIT_WEIGHT_TEXT_IMAGE" {
			return "6"
		}
		return ""
	})
	if err != nil || cfg.Weights.TextImage != 6 || DefaultWeights.TextImage != 3 {
		t.Fatalf("config: %+v %v", cfg.Weights, err)
	}
}
