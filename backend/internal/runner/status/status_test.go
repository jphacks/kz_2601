package status

import "testing"

func TestResult(t *testing.T) {
	cases := []struct {
		outcome, reason string
		paused          bool
		want            string
	}{
		{"", "", false, ResultNone},
		{"ok", "", false, ResultOK},
		{"ok", "", true, ResultNotifyPaused},
		{"skipped_quota", "", false, ResultLimit},
		{"skipped_global", "", false, ResultServiceLimit},
		{"skipped_paused", "", false, ResultServicePaused},
		{"skipped_plan", "", false, ResultPlan},
		{"skipped_stopped", "operator_stop", false, ResultStopped},
		{"skipped_stopped", "app_hidden", false, ResultStopped},
		{"skipped_stopped", "rule_removed", false, ResultChanged},
		{"skipped_stopped", "version_missing", false, ResultChanged},
		{"skipped_stopped", "creator_deleted", false, ResultUnavailable},
		{"skipped_stopped", "app_deleted", false, ResultUnavailable},
		{"skipped_stale", "", false, ResultLate},
		{"skipped_late", "", false, ResultLate},
		{"skipped_inactive", "", false, ResultInactive},
		{"gave_up", "", false, ResultFailed},
		{"something_new", "", false, ResultOther},
	}
	for _, c := range cases {
		if got := Result(c.outcome, c.reason, c.paused); got != c.want {
			t.Errorf("Result(%q, %q, %v) = %q, want %q", c.outcome, c.reason, c.paused, got, c.want)
		}
	}
}
