package ai

import (
	"strings"
	"testing"
)

// The system prompt teaches a per-parent count filtered by a value, as a
// filter list (AND) with the row's id (docs/dsl/examples/schedule_poll.json).
func TestCountWhereRules_InPrompt(t *testing.T) {
	for _, want := range []string{
		"## 親ごとに、子のうち特定の値だけ数える(日程調整・出欠)",
		`"filter":[{"field":"day","op":"eq","value":{"ref":"id"}},{"field":"status","op":"eq","value":"参加できる"}]`,
		"配列(AND)",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	if !strings.Contains(catalog, `"filter"?:filter|[filter]`) {
		t.Error("catalog agg grammar lacks the filter list")
	}
}
