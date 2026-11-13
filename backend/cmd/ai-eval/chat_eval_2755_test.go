package main

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/ai"
)

func boolPtr2755(b bool) *bool { return &b }

func TestCheckChatStep2755(t *testing.T) {
	question := &ai.ChatResult{Kind: "question", Text: "何を記録しますか?", Suggestions: []string{"支出", "体重"}, Locale: ai.LocaleJA, Source: "claude"}
	proposal := &ai.ChatResult{Kind: "proposal", Text: "こんな案はどうですか", Locale: ai.LocaleJA, Options: []ai.ChatOption{
		{ID: "a", Title: "家計簿", Detail: "支出を記録", Instruction: "家計簿を作って"},
	}}
	answer := &ai.ChatResult{Kind: "answer", Text: "それはできません。代わりに手入力にできます", Locale: ai.LocaleJA}
	ready := &ai.ChatResult{Kind: "ready", Text: "作ります", Transcript: "家計簿を作って", Locale: ai.LocaleJA, Brief: &ai.Brief{Native: []string{"camera"}}}

	tests := []struct {
		name string
		st   ChatStep
		res  *ai.ChatResult
		want []string // substrings of the misses, in order; nil = pass
	}{
		{"question expected", ChatStep{Expect: "question"}, question, nil},
		{"question when proposal expected", ChatStep{Expect: "proposal"}, question, []string{"kind=question (want proposal)"}},
		{"proposal expected", ChatStep{Expect: "proposal"}, proposal, nil},
		{"proposal when question expected", ChatStep{Expect: "question"}, proposal, []string{"kind=proposal (want question)"}},
		{"answer saying it cannot, textAny hit", ChatStep{Expect: "answer", TextAny: []string{"できません", "不可"}}, answer, nil},
		{"answer lacks textAny", ChatStep{Expect: "answer", TextAny: []string{"可能です"}}, answer, []string{"text lacks any of 可能です"}},
		{"textNot hit", ChatStep{Expect: "answer", TextNot: []string{"代わり"}}, answer, []string{"text has 代わり"}},
		{"source mismatch", ChatStep{Source: "shortcut"}, question, []string{"source=claude (want shortcut)"}},
		{"multiSelect mismatch", ChatStep{Multi: boolPtr2755(true)}, question, []string{"multiSelect=false (want true)"}},
		{"optionsNot hit", ChatStep{OptionsNot: []string{"支出"}}, proposal, []string{"option a has 支出"}},
		{"suggestion not from the app", ChatStep{SuggAny: []string{"支出"}}, question, []string{"suggestion not from the app: 体重"}},
		{"ready needs native", ChatStep{Expect: "ready", Native: []string{"camera", "map"}}, ready, []string{"native missing map"}},
		{"ready ok", ChatStep{Expect: "ready", Native: []string{"camera"}}, ready, nil},
		{"ready empty transcript", ChatStep{}, &ai.ChatResult{Kind: "ready", Locale: ai.LocaleJA}, []string{"empty transcript"}},
		{"question without chips", ChatStep{}, &ai.ChatResult{Kind: "question", Text: "?", Locale: ai.LocaleJA}, []string{"suggestions=0"}},
		{"question with empty text", ChatStep{}, &ai.ChatResult{Kind: "question", Suggestions: []string{"a"}, Locale: ai.LocaleJA}, []string{"empty question"}},
		{"proposal without options", ChatStep{}, &ai.ChatResult{Kind: "proposal", Locale: ai.LocaleJA}, []string{"options=0"}},
		{"proposal with empty option", ChatStep{}, &ai.ChatResult{Kind: "proposal", Locale: ai.LocaleJA, Options: []ai.ChatOption{{ID: "a", Title: "x"}}}, []string{"empty option"}},
		{"ja reply leaks a DSL name", ChatStep{}, &ai.ChatResult{Kind: "answer", Text: "public の記録で見られます", Locale: ai.LocaleJA}, []string{"dsl terms"}},
		{"en reply is not checked for leaks", ChatStep{}, &ai.ChatResult{Kind: "answer", Text: "It is public", Locale: ai.LocaleEN}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			miss := checkChatStep(tc.st, tc.res)
			if len(miss) != len(tc.want) {
				t.Fatalf("misses = %v, want %d containing %v", miss, len(tc.want), tc.want)
			}
			for i, w := range tc.want {
				if !strings.Contains(miss[i], w) {
					t.Errorf("miss[%d] = %q, want it to contain %q", i, miss[i], w)
				}
			}
		})
	}
}

func TestSummarizeChat2755(t *testing.T) {
	if s := summarizeChat(nil); s != (ChatSummary{}) {
		t.Errorf("empty input should summarize to zero values, got %+v", s)
	}

	recs := []ChatRecord{
		{ID: "a", OK: true, Steps: []ChatStepRecord{
			{OK: true, Attempts: 1, ClaudeMs: 100, FirstTokMs: 10, TotalMs: 120, Usage: ai.Usage{InputTokens: 50, CacheReadInputTokens: 30, CacheCreationInputTokens: 20, OutputTokens: 5}},
			{OK: true, Attempts: 0, TotalMs: 5}, // shortcut: no Claude call
		}},
		{ID: "b", OK: false, Steps: []ChatStepRecord{
			{OK: false, Attempts: 2, ClaudeMs: 300, FirstTokMs: 30, TotalMs: 330, Usage: ai.Usage{InputTokens: 200, OutputTokens: 15}},
			{OK: false, Error: "boom", Attempts: 1, ClaudeMs: 9999, TotalMs: 9999}, // errored: only counted as a step
		}},
	}
	s := summarizeChat(recs)
	if s.Cases != 2 || s.CasesOK != 1 {
		t.Errorf("cases = %d ok=%d, want 2 ok=1", s.Cases, s.CasesOK)
	}
	if s.Steps != 4 || s.StepsOK != 2 {
		t.Errorf("steps = %d ok=%d, want 4 ok=2", s.Steps, s.StepsOK)
	}
	if s.ClaudeCalls != 3 {
		t.Errorf("claudeCalls = %d, want 3 (errored step skipped)", s.ClaudeCalls)
	}
	// claude ms over steps with attempts: [100, 300]; total ms over non-error steps: [5, 120, 330]
	if s.ClaudeP50Ms != 300 || s.ClaudeP95Ms != 300 {
		t.Errorf("claude p50/p95 = %d/%d, want 300/300", s.ClaudeP50Ms, s.ClaudeP95Ms)
	}
	if s.TotalP50Ms != 120 {
		t.Errorf("total p50 = %d, want 120", s.TotalP50Ms)
	}
	if s.FirstTokP50 != 30 {
		t.Errorf("first token p50 = %d, want 30", s.FirstTokP50)
	}
	// input tokens include cache reads and creations: [100, 200]; output: [5, 15]
	if s.InTokP50 != 200 || s.OutTokP50 != 15 {
		t.Errorf("tokens p50 in/out = %d/%d, want 200/15", s.InTokP50, s.OutTokP50)
	}
}

func TestJoinInstructions2755(t *testing.T) {
	opt := func(s string) ai.ChatOption { return ai.ChatOption{Instruction: s} }
	tests := []struct {
		name string
		opts []ai.ChatOption
		want string
	}{
		{"none", nil, ""},
		{"one is sent as is", []ai.ChatOption{opt("家計簿を作って")}, "家計簿を作って"},
		{"several get bullets", []ai.ChatOption{opt("家計簿を作って"), opt("グラフを足して")}, "・家計簿を作って\n・グラフを足して"},
	}
	for _, tc := range tests {
		if got := joinInstructions(tc.opts); got != tc.want {
			t.Errorf("%s: joinInstructions = %q, want %q", tc.name, got, tc.want)
		}
	}
}
