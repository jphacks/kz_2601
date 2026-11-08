package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// "目的地に近づいたら" is app.notifications[].near and "ランダムな時間に" is
// jitterMinutes, in the create prompt, the chat prompts and the schema.
func TestPromptTeachesNotifyNear(t *testing.T) {
	for _, want := range []string{
		"near: {entity, locationField, radiusM}",
		"repeat once = 最初に着いた 1 回だけ",
		"位置はサーバーに送らない",
		"replace /app/notifications/<i>/near/radiusM",
		`time "20:00" + jitterMinutes 60`,
		"offsetMinutes -N と jitterMinutes N",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	for name, p := range map[string]string{"chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt} {
		if !strings.Contains(p, notifyNearChatRule) {
			t.Errorf("%s lacks the near chat rule", name)
		}
	}
	if !strings.HasSuffix(chatCreateSystemPrompt, chatCapabilityIndex) {
		t.Fatal("chat create prompt no longer ends with the capability index")
	}
	for _, want := range []string{`"near":`, `"jitterMinutes":`, `"radiusM":`} {
		if !strings.Contains(string(GenerationSchema()), want) {
			t.Errorf("generation schema lacks %s", want)
		}
	}
}

// Builds without notification.near: no rules, no keys in the schema, near
// notifications dropped whole (an old build would read them as 09:00
// reminders) and jitterMinutes dropped.
func TestNotifyNearRestriction(t *testing.T) {
	all := allComponentTypes()
	old := NewClientRestriction(all, nil, featuresWithout(FeatureNotificationNear))
	if !old.Active() || !old.NoNear() || old.NoWhen() {
		t.Fatalf("a build without notification.near: active=%v noNear=%v noWhen=%v", old.Active(), old.NoNear(), old.NoWhen())
	}
	ctx := withRestriction(context.Background(), old)
	if got := restrictPrompt(ctx, systemPrompt); strings.Contains(withoutNote(got), "## 場所に着いたら知らせる通知は app.notifications の near") || !strings.Contains(got, strings.TrimSpace(noNearNote)) {
		t.Error("create prompt: near rules left / note missing")
	}
	for name, p := range map[string]string{"chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt} {
		if got := restrictPrompt(ctx, p); strings.Contains(got, notifyNearChatRule) || !strings.Contains(got, noNotifyNearChatRule) {
			t.Errorf("%s: chat rule not swapped", name)
		}
	}
	// placeSearch also has a "near" key, so look at the Notification def only.
	if props := notificationProps(t, generationSchema); props["near"] == nil || props["jitterMinutes"] == nil {
		t.Fatal("full generation schema lacks near / jitterMinutes")
	}
	if props := notificationProps(t, old.assets().genSch); props["near"] != nil || props["jitterMinutes"] != nil {
		t.Fatal("restricted generation schema still offers near / jitterMinutes")
	}

	raw := []byte(`{"app":{"name":"駅","notifications":[` +
		`{"id":"arrive","title":"もうすぐ","body":"","repeat":"once","near":{"entity":"stations","locationField":"place","radiusM":800}},` +
		`{"id":"rand","title":"写真","body":"","repeat":"daily","time":"20:00","jitterMinutes":60},` +
		`{"id":"daily","title":"記録","body":"","repeat":"daily","time":"21:00"}]},"entities":[],"screens":[]}`)
	if v := restrictionViolations(raw, nil, old); len(v) != 2 || !strings.Contains(v[0], `"arrive"`) || !strings.Contains(v[1], `"rand"`) {
		t.Fatalf("violations = %v", v)
	}
	if v := restrictionViolations(raw, raw, old); len(v) != 0 {
		t.Fatalf("existing ones are not new violations: %v", v)
	}
	out, notes, changed := enforceRestriction(raw, old)
	s := string(out)
	if !changed || strings.Contains(s, `"arrive"`) || strings.Contains(s, "jitterMinutes") || !strings.Contains(s, `"rand"`) || !strings.Contains(s, `"daily"`) || len(notes) != 1 {
		t.Fatalf("enforce: changed=%v notes=%v out=%s", changed, notes, out)
	}
}

func notificationProps(t *testing.T, sch []byte) map[string]any {
	t.Helper()
	var root struct {
		Defs map[string]struct {
			Properties map[string]any `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(sch, &root); err != nil {
		t.Fatal(err)
	}
	n, ok := root.Defs["Notification"]
	if !ok {
		t.Fatal("generation schema has no Notification def")
	}
	return n.Properties
}
