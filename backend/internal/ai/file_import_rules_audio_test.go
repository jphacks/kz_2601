package ai

import (
	"strings"
	"testing"
)

// The audio import rules reach the prompt after the file import section, and
// the generation schema offers accept audio.
func TestFileImportAudioRulesReachPrompt(t *testing.T) {
	want := "fileImport{entity, accept: [audio], field:"
	if !strings.Contains(systemPrompt, want) {
		t.Fatalf("systemPrompt lacks %q", want)
	}
	if strings.Index(systemPrompt, "## ファイルの取り込み") > strings.Index(systemPrompt, "## 録音ファイルの文字起こし") {
		t.Error("audio section should follow the file import section")
	}
	if strings.Contains(fileImportRules, "音声・動画") {
		t.Error("fileImportRules still says audio files cannot be read")
	}
	if !strings.Contains(string(GenerationSchema()), `"csv","text","pdf","audio"`) {
		t.Error("generation schema lacks accept audio")
	}
}
