package coins

import "testing"

func TestErrorStrings_3278(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&InsufficientError{Required: 30, Balance: 12, Payer: "self"}, "coins insufficient: required 30, balance 12"},
		{&InsufficientError{Payer: "creator"}, "coins insufficient: required 0, balance 0"},
		{&OverflowCapError{Cap: 500, Spent: 500}, "overflow cap reached"},
		{&OverflowCapError{}, "overflow cap reached"},
	} {
		if got := tc.err.Error(); got != tc.want {
			t.Errorf("%T.Error() = %q, want %q", tc.err, got, tc.want)
		}
	}
}
