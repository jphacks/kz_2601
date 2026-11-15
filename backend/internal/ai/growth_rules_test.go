package ai

import (
	"context"
	"strings"
	"testing"
)

// growth: the rules reach the generation prompt, the catalog lists the part
// with its keys, and a client without growth loses every rule line.
func TestGrowthRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## 数値で育つ絵", "歩数で木に実がなる", "歩くほど花が咲く", "貯金が増えると木が育つ", `"live": true`, "kind tree"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	line := ""
	for _, l := range strings.Split(catalog, "\n") {
		if strings.Contains(l, "growth") && strings.Contains(l, "target") {
			line = l
			break
		}
	}
	for _, key := range []string{"metric", "live", "since", "entity", "value", "step", "kind"} {
		if !strings.Contains(line, key) {
			t.Errorf("growth catalog line lacks %s: %q", key, line)
		}
	}
}

// The chat builds the demo phrasings without a question and names no part.
func TestGrowthChatRule(t *testing.T) {
	for name, p := range map[string]string{"chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt} {
		if !strings.Contains(p, "## 数で育つ絵") || !strings.Contains(p, "聞き返さずに ready") || !strings.Contains(p, "画面を開いた時から") {
			t.Errorf("%s lacks the growth chat rule", name)
		}
	}
	if !strings.HasSuffix(chatCreateSystemPrompt, chatCapabilityIndex) {
		t.Error("chat create prompt must keep ending with the capability index")
	}
	if strings.Contains(growthChatRule, "growth") || strings.Contains(growthChatRule, "metric") || strings.Contains(growthChatRule, "since") {
		t.Error("chat rule names a part (#258)")
	}
}

func TestGrowthRulesDroppedWithoutPart(t *testing.T) {
	all := make([]string, 0, len(knownComponentTypes))
	for k := range knownComponentTypes {
		if k != "growth" {
			all = append(all, k)
		}
	}
	ctx := withRestriction(context.Background(), NewRestriction(all, nil))
	got := withoutNote(restrictPrompt(ctx, systemPrompt))
	i := strings.Index(got, "## 数値で育つ絵")
	if i < 0 {
		t.Fatal("heading missing")
	}
	block := got[i+len("## 数値で育つ絵"):]
	if j := strings.Index(block, "\n## "); j >= 0 {
		block = block[:j]
	}
	if strings.Contains(block, "growth") {
		t.Errorf("growth rule lines survived the restriction: %q", block)
	}
}
