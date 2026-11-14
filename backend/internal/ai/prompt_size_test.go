package ai

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// Prompt size budgets (bytes). These are guards, not targets: init rule files
// append to the prompts silently, and every byte is sent (and billed) on each
// call, so cost and latency scale with size. Raising a budget is a deliberate
// decision — trim or merge sections first. Calibration: ~2.5 bytes/token for
// this mostly-Japanese text, so 300,000 bytes ≈ 120K tokens.
const (
	systemPromptBudget      = 300_000 // systemPrompt (create turns)
	editPromptBudget        = 310_000 // systemPrompt + "\n" + editRules (edit turns)
	serverRulesPromptBudget = 40_000  // serverRulesPrompt (server rules pass)
	promptBreakdownTop      = 15
)

func TestPromptSizeBudget(t *testing.T) {
	for _, c := range []struct {
		name   string
		prompt string
		budget int
	}{
		{"systemPrompt", systemPrompt, systemPromptBudget},
		{"systemPrompt+editRules", systemPrompt + "\n" + editRules, editPromptBudget},
		// with AI_ON_DEMAND_RULES=1: the JPHACKS recipes leave the cached block (on_demand_rules.go)
		{"systemPrompt (on-demand core)", onDemandCore(systemPrompt), systemPromptBudget},
		{"serverRulesPrompt", serverRulesPrompt, serverRulesPromptBudget},
	} {
		n := len(c.prompt)
		t.Logf("%s: %d bytes (budget %d, ~%dK tokens)", c.name, n, c.budget, n*2/5/1000)
		if n > c.budget {
			t.Errorf("%s is %d bytes, over the %d-byte budget.\nLargest sections:\n%s",
				c.name, n, c.budget, promptBreakdown(c.prompt, promptBreakdownTop))
		}
	}
}

// promptBreakdown splits on "\n\n## " headings (how init rule files append)
// and lists the largest sections.
func promptBreakdown(p string, top int) string {
	type sec struct {
		head string
		n    int
	}
	parts := strings.Split(p, "\n\n## ")
	secs := make([]sec, 0, len(parts))
	for i, s := range parts {
		head, _, _ := strings.Cut(s, "\n")
		if i == 0 {
			head = "(preamble) " + head
		}
		if r := []rune(head); len(r) > 60 {
			head = string(r[:60]) + "…"
		}
		secs = append(secs, sec{head, len(s)})
	}
	sort.SliceStable(secs, func(i, j int) bool { return secs[i].n > secs[j].n })
	if len(secs) > top {
		secs = secs[:top]
	}
	var b strings.Builder
	for _, s := range secs {
		fmt.Fprintf(&b, "  %7d  %s\n", s.n, s.head)
	}
	return b.String()
}

func TestPromptBreakdown(t *testing.T) {
	got := promptBreakdown("intro\n\n## A\nxx\n\n## BB\nxxxxxx", 2)
	want := "        9  BB\n        5  (preamble) intro\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
