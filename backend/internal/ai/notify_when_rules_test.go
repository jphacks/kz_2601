package ai

import (
	"context"
	"strings"
	"testing"
)

// #265: "少なくなったら教えて" is app.notifications[].when, not a refused feature.
func TestPromptTeachesNotifyWhen(t *testing.T) {
	for _, want := range []string{
		`repeat: "once", when: 条件の式`,
		`{"op":"gt","args":[{"agg":"count","entity":"items","filter":{"field":"stock","op":"lt","value":5}},0]}`,
		"ほかの人の操作だけで条件を満たした時にすぐ届けるのは今はできない",
		"replace /app/notifications/<i>/when",
		condNotifyCanLine,
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	if strings.Contains(systemPrompt, condNotifyCannotLine) {
		t.Error("system prompt still says condition notifications are impossible")
	}
	for name, p := range map[string]string{"chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt} {
		if !strings.Contains(p, notifyWhenChatRule) {
			t.Errorf("%s lacks the when chat rule", name)
		}
	}
	if !strings.HasSuffix(chatCreateSystemPrompt, chatCapabilityIndex) {
		t.Fatal("chat create prompt no longer ends with the capability index")
	}
}

// Builds without notification.when get the old "できない" rule, no when in the
// schema, and generated when notifications are dropped whole (not just the
// key: an old build would read the rest as a 09:00 reminder).
func TestNotifyWhenRestriction(t *testing.T) {
	all := make([]string, 0, len(knownComponentTypes))
	for k := range knownComponentTypes {
		all = append(all, k)
	}
	if r := NewClientRestriction(all, nil, []string{FeatureRecordRetention, FeatureNotificationWhen, FeatureMatching}); r.Active() {
		t.Fatal("a current build must stay unrestricted")
	}
	old := NewClientRestriction(all, nil, []string{FeatureRecordRetention, FeatureMatching})
	if !old.Active() || !old.NoWhen() || old.NoRetention() {
		t.Fatalf("a build without notification.when: active=%v noWhen=%v", old.Active(), old.NoWhen())
	}
	ctx := withRestriction(context.Background(), old)
	got := restrictPrompt(ctx, systemPrompt)
	if strings.Contains(withoutNote(got), "## 条件で知らせる通知は app.notifications の when") || !strings.Contains(got, condNotifyCannotLine) {
		t.Error("create prompt: when rules left / cannot line not restored")
	}
	for name, p := range map[string]string{"chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt} {
		got := restrictPrompt(ctx, p)
		if strings.Contains(got, notifyWhenChatRule) || !strings.Contains(got, noNotifyWhenChatRule) {
			t.Errorf("%s: chat rule not swapped", name)
		}
	}
	if strings.Contains(string(old.assets().genSch), `"when"`) {
		t.Fatal("restricted generation schema still offers when")
	}

	raw := []byte(`{"app":{"name":"在庫","notifications":[` +
		`{"id":"low","title":"少ない","body":"","repeat":"once","when":{"op":"lt","args":[{"agg":"sum","entity":"items","field":"qty"},5]}},` +
		`{"id":"daily","title":"記録","body":"","repeat":"daily","time":"20:00"}]},"entities":[],"screens":[]}`)
	if v := restrictionViolations(raw, nil, old); len(v) != 1 || !strings.Contains(v[0], `"low"`) {
		t.Fatalf("violations = %v", v)
	}
	if v := restrictionViolations(raw, raw, old); len(v) != 0 {
		t.Fatalf("an existing when notification is not a new violation: %v", v)
	}
	out, notes, changed := enforceRestriction(raw, old)
	if !changed || strings.Contains(string(out), `"low"`) || !strings.Contains(string(out), `"daily"`) || len(notes) != 1 {
		t.Fatalf("enforce: changed=%v notes=%v out=%s", changed, notes, out)
	}
}
