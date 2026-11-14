package dsl

import (
	"encoding/json"
	"strings"
	"testing"
)

// #347: enabled false parses and validates (a rule that is off is still a
// valid rule), selects no write, and its description says it is off; a
// non-boolean is refused by the schema; omitted / true is on.
func TestServerRuleEnabled(t *testing.T) {
	off := func(t *testing.T, v string) []byte {
		t.Helper()
		var doc map[string]any
		if err := json.Unmarshal(serverRulesRaw(t), &doc); err != nil {
			t.Fatal(err)
		}
		rules := doc["serverRules"].([]any)
		var val any
		if err := json.Unmarshal([]byte(v), &val); err != nil {
			t.Fatal(err)
		}
		rules[1].(map[string]any)["enabled"] = val // new_message
		raw, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}

	a, err := Parse(off(t, "false"))
	if err != nil {
		t.Fatalf("enabled false: %v", err)
	}
	r := &a.ServerRules[1]
	if r.ID != "new_message" || r.IsEnabled() || !a.ServerRules[0].IsEnabled() {
		t.Fatalf("IsEnabled: %s %v %v", r.ID, r.IsEnabled(), a.ServerRules[0].IsEnabled())
	}
	if got := a.MatchRecordRules("messages", TriggerCreated, nil); len(got) != 0 {
		t.Fatalf("a rule that is off matched: %v", ruleIDs(got))
	}
	if got := a.DescribeServerRule(r); got != "「メッセージ」が作られた時、参加者全員に通知"+DisabledNote {
		t.Fatalf("description: %s", got)
	}

	on, err := Parse(off(t, "true"))
	if err != nil {
		t.Fatal(err)
	}
	if !on.ServerRules[1].IsEnabled() || len(on.MatchRecordRules("messages", TriggerCreated, nil)) != 1 ||
		strings.Contains(on.DescribeServerRule(&on.ServerRules[1]), DisabledNote) {
		t.Fatal("enabled true is on")
	}

	if _, err := Parse(off(t, `"no"`)); err == nil || !strings.Contains(err.Error(), "enabled") {
		t.Fatalf("enabled \"no\": %v", err)
	}
}
