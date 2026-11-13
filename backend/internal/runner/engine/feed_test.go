package engine

import (
	"testing"
	"time"
)

// FeedDay は日本の暦日で区切る。UTC 15:00 が日本の 0 時。
func TestFeedDay(t *testing.T) {
	ny := time.FixedZone("EST", -5*60*60)
	cases := []struct {
		name string
		at   time.Time
		want string
	}{
		{"utc 14:59 is still the same day in Japan", time.Date(2026, 5, 1, 14, 59, 59, 0, time.UTC), "2026-05-01"},
		{"utc 15:00 is the next day in Japan", time.Date(2026, 5, 1, 15, 0, 0, 0, time.UTC), "2026-05-02"},
		{"utc 00:00 is the same day in Japan", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), "2026-05-01"},
		{"year end before Japan midnight", time.Date(2026, 12, 31, 14, 59, 59, 999999999, time.UTC), "2026-12-31"},
		{"year end at Japan midnight", time.Date(2026, 12, 31, 15, 0, 0, 0, time.UTC), "2027-01-01"},
		{"leap day boundary", time.Date(2028, 2, 28, 15, 0, 0, 0, time.UTC), "2028-02-29"},
		{"other zone input is converted to Japan", time.Date(2026, 5, 1, 10, 0, 0, 0, ny), "2026-05-02"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FeedDay(c.at); got != c.want {
				t.Fatalf("FeedDay(%v) = %q, want %q", c.at, got, c.want)
			}
		})
	}
}
