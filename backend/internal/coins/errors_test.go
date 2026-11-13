package coins

import "testing"

func TestErrorStrings_S7Backend009(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		err  error
		want string
	}{
		{"insufficient", &InsufficientError{Required: 30, Balance: 12, Payer: "self"}, "coins insufficient: required 30, balance 12"},
		{"insufficient creator", &InsufficientError{Required: 1, Balance: 0, Payer: "creator"}, "coins insufficient: required 1, balance 0"},
		{"overflow cap", &OverflowCapError{Cap: 100, Spent: 100}, "overflow cap reached"},
		{"overflow cap zero", &OverflowCapError{}, "overflow cap reached"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.err.Error(); got != c.want {
				t.Fatalf("Error() = %q, want %q", got, c.want)
			}
		})
	}
}
