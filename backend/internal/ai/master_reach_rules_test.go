package ai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestMasterReachRules_InPrompt(t *testing.T) {
	for _, want := range []string{"## 保存した記録を開ける一覧(マスタも)", "table と timeline の行はタップできない", `"onItemTap":{"type":"navigate","screen":"member_form"}`} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
}

// The rule asks for what lint savedRecordReach checks, no more: setting
// entities are exempt (a single one cannot be listed at all), other record-tap
// components count, a prefillRef tap does not, and a ranking is one sorted
// list rather than a table plus a list of the same entity. Part names that
// restricted builds drop (matchList, single) are not named here.
func TestMasterReachRules_MatchesLint(t *testing.T) {
	for _, want := range []string{"設定(予算・目標", "gridTable など", "prefillRef", `"sort":{"field":"total","direction":"desc"}`} {
		if !strings.Contains(masterReachRules, want) {
			t.Errorf("masterReachRules lacks %q", want)
		}
	}
}

// Rules that still told the model to rank masters in a table alone.
func TestMasterReachRules_NoTableOnlyRanking(t *testing.T) {
	for name, rule := range map[string]string{"quizRules": quizRules, "groupByRules": groupByRules} {
		for _, bad := range []string{"の table を total で sort desc", "computed agg + table"} {
			if strings.Contains(rule, bad) {
				t.Errorf("%s still says %q", name, bad)
			}
		}
	}
}

// The rule's list example, swapped into the warikan few-shot in place of its
// members list, still validates.
func TestMasterReachRules_ExampleValid(t *testing.T) {
	start := strings.Index(masterReachRules, `{"type":"list"`)
	end := strings.Index(masterReachRules[start:], "\n")
	var example map[string]any
	if err := json.Unmarshal([]byte(masterReachRules[start:start+end]), &example); err != nil {
		t.Fatalf("example is not JSON: %v", err)
	}
	var app map[string]any
	if err := json.Unmarshal([]byte(fewShotWarikan), &app); err != nil {
		t.Fatal(err)
	}
	swapped := false
	for _, s := range app["screens"].([]any) {
		for _, c := range s.(map[string]any)["components"].([]any) {
			kids, _ := c.(map[string]any)["children"].([]any)
			for i, k := range kids {
				if m := k.(map[string]any); m["type"] == "list" && m["entity"] == "members" {
					kids[i] = example
					swapped = true
				}
			}
		}
	}
	if !swapped {
		t.Fatal("warikan few-shot has no members list")
	}
	raw, err := json.Marshal(app)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dsl.Parse(raw); err != nil {
		t.Fatalf("example does not validate: %v", err)
	}
}
