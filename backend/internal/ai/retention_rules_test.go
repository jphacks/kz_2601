package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #482: "1週間たった記録は消して" is entity.retentionDays, not a refused bulk delete.
func TestPromptTeachesRetention(t *testing.T) {
	for _, want := range []string{
		`"retentionDays": 日数`,
		"「一括削除はできません」「消せません」と断らない",
		"remove /entities/<i>/retentionDays",
		"各自が自分で作った記録だけが消える",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	for name, p := range map[string]string{"chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt} {
		if !strings.Contains(p, retentionChatRule) {
			t.Errorf("%s lacks the retention chat rule", name)
		}
	}
	// the create chat prompt must still end with the capability index (restrict.go swaps it)
	if !strings.HasSuffix(chatCreateSystemPrompt, chatCapabilityIndex) {
		t.Fatal("chat create prompt no longer ends with the capability index")
	}
	if !strings.Contains(chatCapabilityIndex, "記録の自動削除(retentionDays)") {
		t.Fatalf("capability index lacks retentionDays: %s", chatCapabilityIndex)
	}
	if got := plainTerms("retentionDays を 7 にします", LocaleJA); strings.Contains(got, "retentionDays") {
		t.Fatalf("chat leaks the DSL name: %q", got)
	}
}

// v1.0 (allowedComponents without supportedFeatures) cannot purge; newer
// builds list recordRetention; clients sending neither stay unrestricted.
func TestRetentionRestriction(t *testing.T) {
	all := make([]string, 0, len(knownComponentTypes))
	for k := range knownComponentTypes {
		all = append(all, k)
	}
	if r := NewClientRestriction(nil, nil, nil); r.Active() {
		t.Fatal("no allowedComponents = no restriction")
	}
	if r := NewClientRestriction(all, nil, []string{FeatureRecordRetention}); r.Active() {
		t.Fatal("a build that purges and draws every part must stay unrestricted (byte-identical prompt)")
	}
	old := NewClientRestriction(all, nil, nil)
	if !old.Active() || !old.NoRetention() {
		t.Fatal("v1.0 must not get retentionDays")
	}
	ctx := withRestriction(context.Background(), old)
	for name, p := range map[string]string{"create": systemPrompt, "chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt} {
		got := restrictPrompt(ctx, p)
		if strings.Contains(withoutNote(got), "## 記録の自動削除は entity.retentionDays") || strings.Contains(got, retentionChatRule) {
			t.Errorf("%s: retention rules left for a build that cannot purge", name)
		}
		if !strings.Contains(got, "最新版に更新すると使えます") {
			t.Errorf("%s: no honest answer for old builds", name)
		}
	}
	if strings.Contains(old.assets().index, "retentionDays") || strings.Contains(string(old.assets().genSch), "retentionDays") {
		t.Fatal("restricted index / schema still offer retentionDays")
	}

	raw := []byte(`{"entities":[{"id":"logs","name":"記録","retentionDays":7,"fields":[]},{"id":"tags","name":"タグ","fields":[]}],"screens":[]}`)
	if v := restrictionViolations(raw, nil, old); len(v) != 1 || !strings.Contains(v[0], "retentionDays") {
		t.Fatalf("violations = %v", v)
	}
	if v := restrictionViolations(raw, raw, old); len(v) != 0 {
		t.Fatalf("an existing period is not a new violation: %v", v)
	}
	out, notes, changed := enforceRestriction(raw, old)
	if !changed || strings.Contains(string(out), "retentionDays") || len(notes) != 1 {
		t.Fatalf("enforce: changed=%v notes=%v out=%s", changed, notes, out)
	}
	var app dsl.App
	if err := json.Unmarshal(out, &app); err != nil || len(app.RetentionEntities()) != 0 {
		t.Fatalf("enforced DSL: %v %v", err, app.RetentionEntities())
	}
}
