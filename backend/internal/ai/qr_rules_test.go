package ai

import (
	"strings"
	"testing"
)

// #422: the QR rules reach the generation prompt, and the catalog / chat
// index (both built from the schema) describe qrCode.
func TestQRRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## QR コードを作って見せる(#422)", "**qrCode**", "text に例(https://example.com 等)を書かない"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	if !strings.Contains(catalog, "\n- qrCode: / ") {
		t.Error("catalog lacks the qrCode line (all keys optional)")
	}
	if !strings.Contains(chatCapabilityIndex, "qrCode") {
		t.Error("chat capability index lacks qrCode (chat would answer that making a QR is impossible)")
	}
}
