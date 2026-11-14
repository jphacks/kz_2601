package store

import "testing"

func TestErrorStrings_3278(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&HeldLimitError{Held: 3, Limit: 3}, "held app limit: 3 of 3"},
		{&HeldLimitError{}, "held app limit: 0 of 0"},
		{&ConflictError{Current: 7}, "version conflict: current is 7"},
		{&ConflictError{}, "version conflict: current is 0"},
	} {
		if got := tc.err.Error(); got != tc.want {
			t.Errorf("%T.Error() = %q, want %q", tc.err, got, tc.want)
		}
	}
}
