package ai

import (
	"strings"
	"testing"
)

// #3694: the live rule reaches the generation prompt and the schema-built
// catalog lists the key on healthStat.
func TestHealthLiveRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## 歩数のリアルタイム更新(live・#3694)", `"live": true`, "リアルタイム", "歩きながら", "今すぐ反映", "live は付けない"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	if !strings.Contains(catalog, "live") {
		t.Error("catalog lacks the live key")
	}
	line := ""
	for _, l := range strings.Split(catalog, "\n") {
		if strings.Contains(l, "healthStat") && strings.Contains(l, "metric") {
			line = l
			break
		}
	}
	if line == "" || !strings.Contains(line, "live") {
		t.Errorf("healthStat catalog line lacks live: %q", line)
	}
}
