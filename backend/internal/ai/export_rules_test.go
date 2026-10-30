package ai

import (
	"strings"
	"testing"
)

// #421: the export rules reach the generation prompt, the catalog lists the
// action with its keys, and chat no longer calls file export impossible.
func TestExportRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## 書き出し(CSV / PDF・#421)", "export{entity, format: csv}", "shareButton(文面だけ)"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	if !strings.Contains(catalog, "\n- export{") || !strings.Contains(catalog, "format") {
		t.Error("catalog lacks the export action line")
	}
	if !strings.Contains(chatCapabilityIndex, "export(") {
		t.Error("chat capability index lacks export")
	}
	for name, p := range map[string]string{"edit": chatEditSystemPrompt, "create": chatCreateSystemPrompt} {
		if strings.Contains(p, "ファイル書き出し") {
			t.Errorf("%s chat prompt still says file export is impossible", name)
		}
	}
}
