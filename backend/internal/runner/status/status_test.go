package status

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestResult(t *testing.T) {
	cases := []struct {
		outcome, reason string
		paused          bool
		want            string
	}{
		{"", "", false, ResultNone},
		{"ok", "", false, ResultOK},
		{"ok", "", true, ResultNotifyPaused},
		{"ok", "app_limit", false, ResultLimit},
		{"ok", "app_limit", true, ResultLimit},
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
		{"skipped_disabled", "", false, ResultDisabled},
		{"skipped_allowance", "", false, ResultAllowanceExhausted},
		{"skipped_overflow_cap", "", false, ResultOverflowCapReached},
		{"skipped_coins", "", false, ResultCoinsInsufficient},
		{"ok", "allowance_exhausted", false, ResultAllowanceExhausted},
		{"ok", "overflow_cap_reached", true, ResultOverflowCapReached},
		{"ok", "coins_insufficient", false, ResultCoinsInsufficient},
		{"something_new", "", false, ResultOther},
	}
	for _, c := range cases {
		if got := Result(c.outcome, c.reason, c.paused); got != c.want {
			t.Errorf("Result(%q, %q, %v) = %q, want %q", c.outcome, c.reason, c.paused, got, c.want)
		}
	}
}

func TestFailureReason(t *testing.T) {
	cases := map[string]string{
		"send_failed":     FailureNotifyFailed,
		"not_readable":    FailureRecordUnavailable,
		"not_found":       FailureRecordUnavailable,
		"mismatch":        FailureMismatch,
		"write_rejected":  FailureWriteRejected,
		"feed_wait":       FailureSourceUnavailable,
		"timeout":         FailureTimeout,
		"scope_violation": FailureInternal, // never told apart for the creator
		"error":           FailureInternal,
		"":                FailureInternal,
		"something_new":   FailureInternal,
	}
	for code, want := range cases {
		if got := FailureReason(code); got != want {
			t.Errorf("FailureReason(%q) = %q, want %q", code, got, want)
		}
	}
}

func TestIsFailureReason(t *testing.T) {
	failures := []string{
		FailureInternal,
		FailureMismatch,
		FailureNotifyFailed,
		FailureRecordUnavailable,
		FailureSourceUnavailable,
		FailureTimeout,
		FailureWriteRejected,
	}
	// a new Failure* constant must be added above too
	sorted := slices.Sorted(slices.Values(failures))
	if all := constWords(t, "Failure"); !slices.Equal(sorted, all) {
		t.Fatalf("table %v, Failure* constants %v", sorted, all)
	}
	for _, w := range failures {
		if !IsFailureReason(w) {
			t.Errorf("IsFailureReason(%q) = false", w)
		}
	}
	for _, w := range []string{
		"",
		"Timeout",
		"TIMEOUT",
		" timeout",
		"timeout ",
		"Notify_Failed",
		CodeSendFailed,
		CodeScopeViolation,
		CodeNotFound,
		CodeError,
		ResultFailed,
		ResultOther,
		"unknown",
	} {
		if IsFailureReason(w) {
			t.Errorf("IsFailureReason(%q) = true", w)
		}
	}
}

// TestResultsMatchSpec keeps the Result* words equal to the openapi
// AutomationResult enum iOS decodes (#347).
func TestResultsMatchSpec(t *testing.T) {
	got := constWords(t, "Result")
	if want := specEnum(t, "../../../../docs/backend/openapi-appconfig.yaml", "AutomationResult:"); !slices.Equal(got, want) {
		t.Errorf("Result* words %v, openapi AutomationResult enum %v", got, want)
	}
}

// TestFailureReasonsMatchSpec keeps the Failure* words equal to the openapi
// AutomationFailureReason enum iOS decodes (#347).
func TestFailureReasonsMatchSpec(t *testing.T) {
	got := constWords(t, "Failure")
	if want := specEnum(t, "../../../../docs/backend/openapi-appconfig.yaml", "AutomationFailureReason:"); !slices.Equal(got, want) {
		t.Errorf("Failure* words %v, openapi AutomationFailureReason enum %v", got, want)
	}
}

// constWords returns the sorted string constants of status.go named prefix*.
func constWords(t *testing.T, prefix string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "status.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || !strings.HasPrefix(vs.Names[0].Name, prefix) || len(vs.Values) != 1 {
			return true
		}
		if lit, ok := vs.Values[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			v, _ := strconv.Unquote(lit.Value)
			got = append(got, v)
		}
		return true
	})
	slices.Sort(got)
	return got
}

// specEnum reads the block-list enum of the schema named key.
func specEnum(t *testing.T, path, key string) []string {
	t.Helper()
	fh, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fh.Close() }()
	var out []string
	inSchema, inEnum := false, false
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == key:
			inSchema = true
		case inSchema && !inEnum && line == "enum:":
			inEnum = true
		case inEnum && strings.HasPrefix(line, "- "):
			out = append(out, strings.TrimSpace(strings.TrimPrefix(line, "- ")))
		case inEnum:
			if len(out) == 0 {
				t.Fatalf("%s: %s enum is not a block list", path, key)
			}
			return out
		}
	}
	t.Fatalf("%s: no enum under %s", path, key)
	return nil
}
