package ai

import (
	"strings"
	"testing"
)

// The swapped sentences must still exist in chat.go / chat_first.go: if
// someone rewrites them, the Replace silently stops and the prompts would
// say both "ask" and "don't ask".
func TestClarifyRules_ReplacedAskLines(t *testing.T) {
	for name, p := range map[string]string{"chat": chatSystemPrompt, "chatCreate": chatCreateSystemPrompt} {
		if strings.Contains(p, clarifyAskLine) || !strings.Contains(p, clarifyAskTuned) {
			t.Errorf("%s: ask line not swapped", name)
		}
		for _, want := range []string{clarifyRules, appDefaultsChat, "最大 2 問", "1 つ目はおすすめ"} {
			if !strings.Contains(p, want) {
				t.Errorf("%s lacks %.30q", name, want)
			}
		}
	}
	if strings.Contains(chatCreateSystemPrompt, clarifyKindOnlyLine) || !strings.Contains(chatCreateSystemPrompt, clarifyKindOnlyTuned) {
		t.Error("create: kind-only line not swapped")
	}
	if !strings.HasSuffix(chatCreateSystemPrompt, chatCapabilityIndex) {
		t.Error("create prompt must end with the capability index")
	}
	if !strings.Contains(chatEditSystemPrompt, clarifyEditRule) || strings.Contains(chatEditSystemPrompt, clarifyRules) {
		t.Error("edit prompt: only the edit rule")
	}
	if strings.Contains(chatEditSystemPrompt, appDefaultsChat) {
		t.Error("edit prompt does not need the defaults table")
	}
}

func TestAppDefaults_TableInPrompts(t *testing.T) {
	if !strings.Contains(systemPrompt, appDefaultsRules) {
		t.Fatal("generate prompt lacks the defaults table")
	}
	table := appDefaultsTable()
	if n := strings.Count(table, "\n") + 1; n != len(appDefaults)+2 {
		t.Fatalf("table rows %d, want %d", n, len(appDefaults)+2)
	}
	seen := map[string]bool{}
	for _, d := range appDefaults {
		if d.kind == "" || d.record == "" || d.home == "" || d.notify == "" || seen[d.kind] {
			t.Errorf("bad row %+v", d)
		}
		seen[d.kind] = true
		for _, cell := range []string{d.kind, d.record, d.home, d.notify} {
			if strings.Contains(cell, "|") || strings.Contains(cell, "\n") {
				t.Errorf("cell breaks the table: %q", cell)
			}
		}
	}
	// the common requests the clarify rules name
	for _, k := range []string{"家計簿", "買い物", "習慣", "服薬", "水分", "筋トレ", "日記", "勉強"} {
		if !strings.Contains(table, k) {
			t.Errorf("table lacks %s", k)
		}
	}
}

// The chat prompts carry the table, so it must be in plain words (#258).
func TestAppDefaults_PlainWords(t *testing.T) {
	if got := LeakedTerms(appDefaultsChat + clarifyRules + clarifyEditRule); len(got) > 0 {
		t.Errorf("DSL words in the chat rules: %v", got)
	}
}
