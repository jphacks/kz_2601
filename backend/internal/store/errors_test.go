package store

import "testing"

func TestErrorStrings_S7Backend009(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		err  error
		want string
	}{
		{"held limit", &HeldLimitError{Held: 5, Limit: 5}, "held app limit: 5 of 5"},
		{"held limit zero", &HeldLimitError{}, "held app limit: 0 of 0"},
		{"conflict", &ConflictError{Current: 7}, "version conflict: current is 7"},
		{"conflict zero", &ConflictError{}, "version conflict: current is 0"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.err.Error(); got != c.want {
				t.Fatalf("Error() = %q, want %q", got, c.want)
			}
		})
	}
}
