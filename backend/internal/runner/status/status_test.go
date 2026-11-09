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
		{"something_new", "", false, ResultOther},
	}
	for _, c := range cases {
		if got := Result(c.outcome, c.reason, c.paused); got != c.want {
			t.Errorf("Result(%q, %q, %v) = %q, want %q", c.outcome, c.reason, c.paused, got, c.want)
		}
	}
}

// TestResultsMatchSpec keeps the Result* words equal to the openapi
// AutomationResult enum iOS decodes (#347).
func TestResultsMatchSpec(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "status.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || !strings.HasPrefix(vs.Names[0].Name, "Result") || len(vs.Values) != 1 {
			return true
		}
		if lit, ok := vs.Values[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			v, _ := strconv.Unquote(lit.Value)
			got = append(got, v)
		}
		return true
	})
	slices.Sort(got)
	if want := specEnum(t, "../../../../docs/backend/openapi-appconfig.yaml", "AutomationResult:"); !slices.Equal(got, want) {
		t.Errorf("Result* words %v, openapi AutomationResult enum %v", got, want)
	}
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
