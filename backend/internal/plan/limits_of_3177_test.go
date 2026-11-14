package plan

import "testing"

func TestLimitsOf_3177(t *testing.T) {
	t.Parallel()
	l := Limits{AIUnitsDaily: 9, Apps: 2, RecordsPerApp: 5000, PhotosPerApp: 500}
	tests := []struct {
		resource string
		want     int
	}{
		{Apps, 2},
		{RecordsPerApp, 5000},
		{PhotosPerApp, 500},
		{"知らない名前", 0},
		{"", 0},
		{"aiUnitsDaily", 0}, // AIUnitsDaily は Of の対象ではない
	}
	for _, tc := range tests {
		t.Run(tc.resource, func(t *testing.T) {
			t.Parallel()
			if got := l.Of(tc.resource); got != tc.want {
				t.Errorf("Of(%q) = %d, want %d", tc.resource, got, tc.want)
			}
		})
	}
	if got := (Limits{}).Of(Apps); got != 0 {
		t.Errorf("空の Limits の Of(Apps) = %d, want 0", got)
	}
}
