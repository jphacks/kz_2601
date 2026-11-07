package plan

import (
	"strings"
	"testing"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestFromEnv(t *testing.T) {
	got, err := FromEnv(envOf(nil))
	if err != nil || got != Default {
		t.Fatalf("unset = Default: %+v %v", got, err)
	}
	got, err = FromEnv(envOf(map[string]string{
		"PLAN_FREE_APPS": "3", "PLAN_PRO_RECORDS_PER_APP": "0", "PLAN_FREE_PHOTOS_PER_APP": " 7 ",
		"PLAN_LIMITS_MODE": "LOG", "RATE_LIMIT_USER_DAILY": "40", "RATE_LIMIT_PRO_DAILY": "500",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got.Free.Apps != 3 || got.Pro.RecordsPerApp != 0 || got.Free.PhotosPerApp != 7 || got.Mode != ModeLog ||
		got.Free.AIUnitsDaily != 40 || got.Pro.AIUnitsDaily != 500 || got.Pro.Apps != Default.Pro.Apps {
		t.Fatalf("overrides: %+v", got)
	}
	for k, v := range map[string]string{"PLAN_PRO_APPS": "-1", "PLAN_FREE_RECORDS_PER_APP": "many", "PLAN_LIMITS_MODE": "enforced", "RATE_LIMIT_PRO_DAILY": "x"} {
		if _, err := FromEnv(envOf(map[string]string{k: v})); err == nil {
			t.Errorf("%s=%q must fail", k, v)
		}
	}
}

func TestAllows(t *testing.T) {
	tb := Table{Free: Limits{Apps: 2}, Pro: Limits{Apps: 0}}
	for _, c := range []struct {
		p    Plan
		used int
		want bool
	}{
		{Free, 0, true}, {Free, 1, true}, {Free, 2, false}, {Free, 9, false},
		{Pro, 1000, true}, // 0 = not counted
	} {
		if got := tb.Allows(c.p, Apps, c.used); got != c.want {
			t.Errorf("Allows(%s, %d) = %v", c.p, c.used, got)
		}
	}
	if !tb.Allows(Free, "unknown", 99) {
		t.Error("an unknown resource is not limited")
	}
}

func TestModeStop(t *testing.T) {
	if ModeOff.Stop(t.Context(), Free, Apps, "u", 1, 1) || ModeLog.Stop(t.Context(), Free, Apps, "u", 1, 1) {
		t.Fatal("off / log never stop")
	}
	if !ModeEnforce.Stop(t.Context(), Free, Apps, "u", 1, 1) || !Mode("").Stop(t.Context(), Free, Apps, "u", 1, 1) {
		t.Fatal("enforce and the zero Mode stop")
	}
}

func TestMessage(t *testing.T) {
	for _, c := range []struct {
		upgrade, byCreator bool
		want               string
	}{
		{true, false, "Tailor It Pro にすると"},
		{false, false, "消すと"},
		{true, true, "作った人が Tailor It Pro"},
		{false, true, "作った人のプラン"},
	} {
		got := Message(RecordsPerApp, 10, c.upgrade, c.byCreator)
		if !strings.Contains(got, c.want) || !strings.Contains(got, "10 件") {
			t.Errorf("Message(%v,%v) = %q", c.upgrade, c.byCreator, got)
		}
	}
}
