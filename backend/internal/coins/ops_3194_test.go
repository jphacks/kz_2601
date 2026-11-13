package coins

import (
	"testing"
	"time"
)

// TestStoredTime: UTC at microseconds (what Firestore keeps); the zero time
// stays zero.
func TestStoredTime_3194(t *testing.T) {
	jst := time.FixedZone("JST", 9*3600)
	base := time.Date(2026, 11, 13, 12, 34, 56, 0, time.UTC)

	for _, tc := range []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{"zero stays zero", time.Time{}, time.Time{}},
		{"zero in another zone stays zero", time.Time{}.In(jst), time.Time{}.In(jst)},
		{"whole seconds unchanged", base, base},
		{"nanoseconds are truncated, not rounded", base.Add(1999 * time.Nanosecond), base.Add(1 * time.Microsecond)},
		{"999 ns drop to the second", base.Add(999 * time.Nanosecond), base},
		{"microseconds kept", base.Add(123456 * time.Microsecond), base.Add(123456 * time.Microsecond)},
		{"last nanosecond of the second", base.Add(999999999 * time.Nanosecond), base.Add(999999 * time.Microsecond)},
		{"other zone becomes the same instant in UTC", base.In(jst), base},
		{"before 1970", time.Date(1969, 12, 31, 23, 59, 59, 123456789, time.UTC), time.Date(1969, 12, 31, 23, 59, 59, 123456000, time.UTC)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := storedTime(tc.in)
			if !got.Equal(tc.want) {
				t.Fatalf("storedTime(%v) = %v, want %v", tc.in, got, tc.want)
			}
			if !tc.in.IsZero() && got.Location() != time.UTC {
				t.Fatalf("location = %v, want UTC", got.Location())
			}
			if got.Nanosecond()%1000 != 0 {
				t.Fatalf("sub-microsecond left: %d ns", got.Nanosecond())
			}
		})
	}
	// idempotent: what is stored reads back as the same value
	in := base.In(jst).Add(777 * time.Nanosecond)
	if once := storedTime(in); !storedTime(once).Equal(once) {
		t.Fatal("storedTime is not idempotent")
	}
}
