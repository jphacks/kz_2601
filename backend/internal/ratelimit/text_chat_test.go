package ratelimit

import (
	"net/http/httptest"
	"testing"
)

// An aiChat turn (task chat) weighs TextLong: it carries up to 16 KB of turns.
func TestWeightTextChat(t *testing.T) {
	w := DefaultWeights
	for body, want := range map[string]int{
		`{"task":"chat","instruction":"x","messages":[{"role":"user","text":"hi"}]}`:                 3,
		`{"task":"chat","length":"long","instruction":"x","messages":[{"role":"user","text":"hi"}]}`: 3,
		`{"task":"summarize","input":"x"}`:                                                           2,
	} {
		r := httptest.NewRequest("POST", "/v1/text", nil)
		if got := w.Weight(r, []byte(body)); got != want {
			t.Errorf("%s: weight %d, want %d", body, got, want)
		}
	}
	w.TextLong = 5
	r := httptest.NewRequest("POST", "/v1/text", nil)
	if got := w.Weight(r, []byte(`{"task":"chat"}`)); got != 5 {
		t.Errorf("chat follows TextLong: %d", got)
	}
}
