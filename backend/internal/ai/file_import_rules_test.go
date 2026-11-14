package ai

import (
	"strings"
	"testing"
)

// The file import rules reach the generation prompt, and the catalog / chat
// index (both built from the schema) describe fileImport.
func TestFileImportRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## ファイルの取り込み(CSV・テキスト・PDF)", "fileImport{entity, accept: [csv]}", "fileImport{entity, accept: [pdf, text], field:"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	if !strings.Contains(catalog, "\n- fileImport: entity, accept") {
		t.Error("catalog lacks the fileImport line with its required keys")
	}
	if !strings.Contains(chatCapabilityIndex, "fileImport") {
		t.Error("chat capability index lacks fileImport")
	}
}
