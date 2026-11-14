package store

import (
	"reflect"
	"testing"
	"time"
)

func scheduleAllFields2767() Schedule {
	base := time.Date(2026, 3, 1, 12, 34, 56, 123456000, time.UTC)
	return Schedule{
		ID:               "APP1|uid1|rule1",
		AppID:            "APP1",
		UID:              "uid1",
		Scope:            "private",
		RuleID:           "rule1",
		Every:            "monthly",
		Time:             "07:05",
		Weekday:          "mon",
		Day:              28,
		TimeZone:         "Asia/Tokyo",
		Version:          7,
		CreatorAnonymous: true,
		Status:           "active",
		NextRunAt:        base.Add(24 * time.Hour),
		LastDueAt:        base,
		CreatedAt:        base.Add(-48 * time.Hour),
		UpdatedAt:        base.Add(-time.Hour),
	}
}

func TestScheduleDocRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name string
		want Schedule
	}{
		{"全部の欄", scheduleAllFields2767()},
		{"欄が空", Schedule{ID: "APP2|uid2|rule2"}},
	} {
		d := toScheduleDoc(&tc.want)
		got := d.toSchedule(tc.want.ID)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: toSchedule(toScheduleDoc(s)) = %#v, want %#v", tc.name, got, tc.want)
		}
	}
}

func TestScheduleDocTakesIDFromTheCallerNotTheDoc(t *testing.T) {
	s := scheduleAllFields2767()
	d := toScheduleDoc(&s)
	if got := d.toSchedule("OTHER|uid|rule").ID; got != "OTHER|uid|rule" {
		t.Errorf("ID = %q, want %q", got, "OTHER|uid|rule")
	}
}

func TestScheduleDocKeepsMicrosecondTimes(t *testing.T) {
	s := scheduleAllFields2767()
	d := toScheduleDoc(&s)
	got := d.toSchedule(s.ID)
	for _, tc := range []struct {
		what        string
		want, after time.Time
	}{
		{"nextRunAt", s.NextRunAt, got.NextRunAt},
		{"lastDueAt", s.LastDueAt, got.LastDueAt},
		{"createdAt", s.CreatedAt, got.CreatedAt},
		{"updatedAt", s.UpdatedAt, got.UpdatedAt},
	} {
		if !tc.after.Equal(tc.want) {
			t.Errorf("%s = %s, want %s", tc.what, tc.after.Format(time.RFC3339Nano), tc.want.Format(time.RFC3339Nano))
		}
		if tc.after != tc.after.Truncate(time.Microsecond) {
			t.Errorf("%s = %s, want マイクロ秒まで", tc.what, tc.after.Format(time.RFC3339Nano))
		}
	}
}

func TestOriginDocRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name string
		want *RecordOrigin
	}{
		{"全部の欄", &RecordOrigin{RuleID: "rule1", Version: 7, EventID: "ev1", ActorUID: "uid1"}},
		{"書いた人が無い", &RecordOrigin{RuleID: "rule2", Version: 1, EventID: "ev2"}},
		{"欄が空", &RecordOrigin{}},
	} {
		got := toOriginDoc(tc.want).toOrigin()
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: toOrigin(toOriginDoc(o)) = %#v, want %#v", tc.name, got, tc.want)
		}
	}
}

func TestOriginDocKeepsNilAsNil(t *testing.T) {
	if got := toOriginDoc(nil); got != nil {
		t.Errorf("toOriginDoc(nil) = %#v, want nil", got)
	}
	var d *originDoc
	if got := d.toOrigin(); got != nil {
		t.Errorf("(*originDoc)(nil).toOrigin() = %#v, want nil", got)
	}
	if got := toOriginDoc(nil).toOrigin(); got != nil {
		t.Errorf("toOriginDoc(nil).toOrigin() = %#v, want nil", got)
	}
}

func TestOriginDocDoesNotShareThePointer(t *testing.T) {
	want := &RecordOrigin{RuleID: "rule1", Version: 7, EventID: "ev1", ActorUID: "uid1"}
	got := toOriginDoc(want).toOrigin()
	if got == want {
		t.Fatal("toOrigin(toOriginDoc(o)) は同じ *RecordOrigin を返した、別の値が欲しい")
	}
	got.RuleID = "changed"
	if want.RuleID != "rule1" {
		t.Errorf("元の RuleID = %q, want %q", want.RuleID, "rule1")
	}
}
