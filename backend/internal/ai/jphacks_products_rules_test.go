package ai

import (
	"strings"
	"testing"
)

func TestJPHACKSProductsRules_InPrompt(t *testing.T) {
	for _, s := range []string{
		`"unique":["slot"]`,
		"gridTable{entity: task, columnField: urgency, rowField: importance",
		`{"op":"div","args":[{"agg":"count","entity":"menu","filter":{"field":"done","op":"eq","value":true}}`,
		"randomPicker の filter done eq false",
		`{"op":"clamp","args":[{"op":"add","args":[50,{"agg":"sum","entity":"habit_log"`,
	} {
		if !strings.Contains(systemPrompt, s) {
			t.Errorf("systemPrompt lacks %q", s)
		}
	}
}
