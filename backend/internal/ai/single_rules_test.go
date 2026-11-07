package ai

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #267: one-of values are a single entity read with {"setting": ...}.
func TestPromptTeachesSingleEntity(t *testing.T) {
	for _, want := range []string{
		`**"single": true**`,
		`{"setting":"cycle_days"}`,
		settingsSingleLine,
		notifySettingBudget,
		"single の entity を list / table",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	for _, gone := range []string{settingsLatestLine, notifyLatestBudget} {
		if strings.Contains(systemPrompt, gone) {
			t.Errorf("system prompt still reads settings with latest: %q", gone)
		}
	}
	for name, p := range map[string]string{"chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt} {
		if !strings.Contains(p, singleChatRule) {
			t.Errorf("%s lacks the single chat rule", name)
		}
		if !strings.Contains(p, retentionChatRule) || !strings.Contains(p, repeatChatRule) {
			t.Errorf("%s lost another chat rule", name)
		}
	}
	if !strings.HasSuffix(chatCreateSystemPrompt, chatCapabilityIndex) {
		t.Fatal("chat create prompt no longer ends with the capability index")
	}
	if !strings.Contains(catalog, `{"setting":"cycle_days"}`) {
		t.Error("catalog recipes lack the cycle setting")
	}
}

// singleDoc is a valid DSL with a single settings entity read from a computed
// and a stat (rules_test style: small, not an example file).
const singleDoc = `{"schemaVersion":1,"app":{"name":"生理","voiceEntry":"floating"},"initialScreen":"home",
"entities":[
 {"id":"periods","name":"生理","fields":[{"id":"start_on","name":"開始日","type":"date"},
  {"id":"next_on","name":"次","type":"date","computed":{"op":"addDays","args":[{"ref":"start_on"},{"setting":"cycle_days"}]}}]},
 {"id":"settings","name":"設定","single":true,"fields":[{"id":"cycle_days","name":"周期","type":"integer","defaultValue":28}]}],
"screens":[
 {"id":"home","title":"記録","toolbar":[{"label":"追加","action":{"type":"navigate","screen":"form"}},{"label":"周期","action":{"type":"navigate","screen":"cfg"}}],
  "components":[{"type":"stat","label":"周期","value":{"setting":"cycle_days"}},{"type":"list","entity":"periods","item":{"title":"start_on","trailing":"next_on"}}]},
 {"id":"form","title":"記録","entity":"periods","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"periods"}}],"components":[{"type":"datePicker","field":"start_on"}]},
 {"id":"cfg","title":"周期","entity":"settings","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"settings"}}],"components":[{"type":"numberField","field":"cycle_days"}]}]}`

// Builds without singleEntity: no single in the schema / prompt, generated
// single entities are violations, and enforcement turns them back into
// records read with agg latest (a DSL those builds can run).
func TestSingleRestriction(t *testing.T) {
	if _, err := dsl.Parse([]byte(singleDoc)); err != nil {
		t.Fatal(err)
	}
	all := make([]string, 0, len(knownComponentTypes))
	for k := range knownComponentTypes {
		all = append(all, k)
	}
	old := NewClientRestriction(all, nil, []string{FeatureRecordRetention, FeatureNotificationWhen, FeatureMatching})
	if !old.Active() || !old.NoSingle() || old.NoWhen() {
		t.Fatalf("a build without singleEntity: active=%v noSingle=%v", old.Active(), old.NoSingle())
	}
	ctx := withRestriction(context.Background(), old)
	got := restrictPrompt(ctx, systemPrompt)
	if strings.Contains(withoutNote(got), `{"setting"`) || strings.Contains(withoutNote(got), "single の entity") {
		t.Error("create prompt: single rules or setting recipes left")
	}
	for _, want := range []string{settingsLatestLine, notifyLatestBudget, budgetLatestRecipe} {
		if !strings.Contains(got, want) {
			t.Errorf("create prompt: %q not restored", want)
		}
	}
	for name, p := range map[string]string{"chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt} {
		got := restrictPrompt(ctx, p)
		if strings.Contains(got, singleChatRule) || !strings.Contains(got, noSingleChatRule) {
			t.Errorf("%s: chat rule not swapped", name)
		}
	}
	sch := string(old.assets().genSch)
	if strings.Contains(sch, `"single"`) || strings.Contains(sch, "ExprSetting") {
		t.Fatal("restricted generation schema still offers single / setting")
	}

	raw := []byte(singleDoc)
	if v := restrictionViolations(raw, nil, old); len(v) != 1 || !strings.Contains(v[0], `"settings"`) {
		t.Fatalf("violations = %v", v)
	}
	if v := restrictionViolations(raw, raw, old); len(v) != 0 {
		t.Fatalf("an existing single entity is not a new violation: %v", v)
	}
	out, notes, changed := enforceRestriction(raw, old)
	if !changed || len(notes) != 1 || strings.Contains(string(out), `"single"`) || strings.Contains(string(out), `"setting"`) {
		t.Fatalf("enforce: changed=%v notes=%v out=%s", changed, notes, out)
	}
	if !strings.Contains(string(out), `{"agg":"latest","entity":"settings","field":"cycle_days"}`) {
		t.Fatalf("setting must become agg latest: %s", out)
	}
	if _, err := dsl.Parse(out); err != nil {
		t.Fatalf("the enforced DSL must stay valid: %v", err)
	}
	// a current build keeps it
	cur := NewClientRestriction(all, nil, []string{FeatureRecordRetention, FeatureNotificationWhen, FeatureMatching, FeatureSingleEntity, FeatureDataRoles, FeatureAddressed})
	if cur.Active() {
		t.Fatal("a current build must stay unrestricted")
	}
}

// The d_period case expects a single entity and a setting (ai-eval).
func TestPeriodCaseExpectsSingle(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("eval", "m0disc_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, ln := range strings.Split(string(raw), "\n") {
		if strings.Contains(ln, `"id": "d_period"`) {
			if !strings.Contains(ln, `"single": true`) || !strings.Contains(ln, `"setting": true`) {
				t.Fatalf("d_period expect lacks single / setting: %s", ln)
			}
			return
		}
	}
	t.Fatal("d_period case not found")
}
