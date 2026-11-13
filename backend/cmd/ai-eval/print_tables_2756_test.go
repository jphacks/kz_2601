package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/ai"
)

// captureStdout2756 runs f with os.Stdout redirected to a pipe and returns
// what was written. The tables print straight to stdout, so this avoids
// touching the production signatures.
func captureStdout2756(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() { os.Stdout = orig }()
	f()
	w.Close()
	os.Stdout = orig
	return <-done
}

func TestPrintTable2756(t *testing.T) {
	run := Run{
		Date: "2026-11-13", Model: "m1", Effort: "low", Stream: true,
		Cases: []Record{
			{ID: "c1", Kind: "create", Locale: "ja", OK: true, Attempts: 1, ClaudeMs: 1200, FirstEventMs: 300,
				Usage:   ai.Usage{InputTokens: 100, CacheReadInputTokens: 20, OutputTokens: 40},
				Metrics: &Metrics{Tabs: 2, Screens: 3, Components: 7, AreaRatio: 0.5, Notifications: []string{"daily"}},
				Summary: "a|b"},
			{ID: "c2", Kind: "edit", Locale: "en", OK: false, Attempts: 2, AreaFallback: true, Downgraded: true,
				Error: "boom", ExpectMisses: []string{"x", "y"}},
		},
		Summary: Summary{N: 2, OK: 1, FirstPass: 1, Failed: 1, ExpectMisses: 2},
	}
	out := captureStdout2756(t, func() { printTable(run) })
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	has := func(want string) {
		t.Helper()
		for _, l := range lines {
			if strings.Contains(l, want) {
				return
			}
		}
		t.Errorf("no line contains %q in:\n%s", want, out)
	}
	has("## 2026-11-13 m1 effort=low stream=true")
	has("| id | kind | loc | ok | att | fb | claude ms | first ms | in/out tok | tabs | scr | comp | area% | notif | misses | summary |")
	has("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|")
	has("| c1 | create | ja | OK | 1 | - | 1200 | 300 | 120/40 | 2 | 3 | 7 | 50% | daily | - | a/b |")
	has("| c2 | edit | en | FAIL | 2 | area+full | 0 | - | 0/0 | - | - | - | - | - | x; y | boom |")
	has("n=2 ok=1 firstPass=1 regenerated=0 areaFallback=0 failed=1 expectMisses=2")
	has("create p50=0ms")
	// c1 has Metrics, so it gets a logic line; c2 does not
	has("logic c1: ")
	for _, l := range lines {
		if strings.HasPrefix(l, "logic c2") {
			t.Errorf("case without metrics should have no logic line: %q", l)
		}
	}
	// every table row has the same column count as the header
	for _, l := range lines {
		if strings.HasPrefix(l, "| c") && strings.Count(l, "|") != 17 {
			t.Errorf("row has %d pipes, want 17: %q", strings.Count(l, "|"), l)
		}
	}
}

func TestPrintChatTable2756(t *testing.T) {
	run := ChatRun{
		Date: "2026-11-13", Model: "m1", Effort: "low",
		Cases: []ChatRecord{
			{ID: "k1", Locale: "ja", OK: true, Steps: []ChatStepRecord{
				{Say: "家計簿", Kind: "question", Source: "claude", Turn: 1, ClaudeMs: 800, FirstTokMs: 200,
					Usage: ai.Usage{InputTokens: 10, OutputTokens: 5}, Text: "何を\n記録?", Suggestions: []string{"支出", "体重"}},
				{Say: "支出", Kind: "proposal", Turn: 2, Brief: &ai.Brief{Native: []string{"camera", "map"}},
					Options: []ai.ChatOption{{Title: "T", Instruction: "I"}},
					Gen:     &ChatGenRecord{OK: true, Mode: "create", TotalMs: 50, Summary: "done"}},
			}},
			{ID: "k2", Locale: "en", OK: false, Steps: []ChatStepRecord{
				{Say: "a|b", Error: "boom", Misses: []string{"m1", "m2"}},
			}},
		},
		Summary: ChatSummary{Cases: 2, CasesOK: 1, Steps: 3, StepsOK: 2, ClaudeCalls: 1, ClaudeP50Ms: 800},
	}
	out := captureStdout2756(t, func() { printChatTable(run) })
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	has := func(want string) {
		t.Helper()
		for _, l := range lines {
			if strings.Contains(l, want) {
				return
			}
		}
		t.Errorf("no line contains %q in:\n%s", want, out)
	}
	has("## chat 2026-11-13 m1 effort=low")
	has("| id | loc | step | say | kind | src | turn | claude ms | 1st tok ms | in/out tok | text | suggestions | native | misses |")
	has("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|")
	has("| k1 | ja | 1 | 家計簿 | question | claude | 1 | 800 | 200 | 10/5 | 何を 記録? | 支出 / 体重 | - | - |")
	has("| k1 | ja | 2 | 支出 | proposal |  | 2 | 0 | 0 | 0/0 |  | - [T: I] → generate ok=true unchanged=false create 50ms done | camera,map | - |")
	has("| k2 | en | 1 | a/b | ERROR: boom |  | 0 | 0 | 0 | 0/0 |  | - | - | m1; m2 |")
	has("cases=2 ok=1 | steps=3 ok=2 | claude calls=1 p50=800ms")
	has("failed: k2")
	for _, l := range lines {
		if strings.HasPrefix(l, "| k") && strings.Count(l, "|") != 15 {
			t.Errorf("row has %d pipes, want 15: %q", strings.Count(l, "|"), l)
		}
	}
}

func TestPrintChatTableAllOK2756(t *testing.T) {
	run := ChatRun{Cases: []ChatRecord{{ID: "k1", OK: true}}, Summary: ChatSummary{Cases: 1, CasesOK: 1}}
	out := captureStdout2756(t, func() { printChatTable(run) })
	if strings.Contains(out, "failed:") {
		t.Errorf("no failed line expected when every case passed:\n%s", out)
	}
}
