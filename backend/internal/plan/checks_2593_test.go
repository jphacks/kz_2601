package plan

import "testing"

func TestModeChecks2593(t *testing.T) {
	cases := []struct {
		m    Mode
		want bool
	}{
		{ModeOff, false},
		{ModeEnforce, true},
		{ModeLog, true},
		{Mode(""), true}, // zero Mode counts as enforce
	}
	for _, c := range cases {
		if got := c.m.Checks(); got != c.want {
			t.Errorf("Mode(%q).Checks() = %v, want %v", c.m, got, c.want)
		}
	}
}
