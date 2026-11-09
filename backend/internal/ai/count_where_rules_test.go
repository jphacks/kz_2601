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

// parent_child.go must not teach the parent-only count as 参加人数 while
// this section says not to, and the parent list must stay tappable (the
// answer button sits on the parent's form; why table rows can't be tapped is
// taught once in master_reach_rules.go).
func TestCountWhereRules_AgreeWithParentChild(t *testing.T) {
	if strings.Contains(systemPrompt, "親ごとの数(コメント数・参加人数)") {
		t.Error("parent_child rules still give the parent-only count for 参加人数")
	}
	for _, want := range []string{
		"子の値で絞る数は「親ごとに、子のうち特定の値だけ数える」",
		"親の一覧の list(onItemTap でフォーム)",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
}
