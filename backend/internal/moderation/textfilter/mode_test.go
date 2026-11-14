package textfilter

import "testing"

func TestModeFromEnv(t *testing.T) {
	for in, want := range map[string]Mode{"": ModeEnforce, "enforce": ModeEnforce, " LOG ": ModeLog, "off": ModeOff} {
		got, err := ModeFromEnv(func(string) string { return in })
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ModeFromEnv(func(string) string { return "of" }); err == nil {
		t.Error("typo: want an error")
	}
}
