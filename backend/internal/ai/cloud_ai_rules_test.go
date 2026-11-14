package ai

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCloudAIRulesReachPrompt(t *testing.T) {
	for _, want := range []string{"## アプリの中の AI(要約・添削・分類・提案・相談)", "task summarize", "talk{question", "写真を AI に見せて"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
}

// The two shapes the rules teach must pass the DSL validation as written.
func TestCloudAIRulesShapesValidate(t *testing.T) {
	diary := dsl.App{
		SchemaVersion: dsl.SchemaVersion,
		App:           dsl.AppMeta{Name: "日記", VoiceEntry: dsl.VoiceEntryNone},
		InitialScreen: "form",
		Entities: []dsl.Entity{{ID: "diary", Name: "日記", Fields: []dsl.Field{
			{ID: "body", Name: "本文", Type: dsl.FieldText},
			{ID: "summary", Name: "要約", Type: dsl.FieldText},
			{ID: "mood", Name: "気分", Type: dsl.FieldEnum, Options: []string{"良い", "普通", "悪い"}},
		}}},
		Screens: []dsl.Screen{{ID: "form", Title: "書く", Entity: "diary", Components: []dsl.Component{
			{Type: dsl.CompAiButton, ID: "s", Task: "summarize", Input: "body", Output: "summary"},
			{Type: dsl.CompAiButton, ID: "c", Task: "classify", Input: "body", Output: "mood"},
			{Type: dsl.CompAiButton, ID: "a", Task: "suggest", Input: "body", Output: "summary", Prompt: "おばあちゃんの口調で、危ない点を 1 つ"},
		}}},
	}
	if err := diary.Validate(); err != nil {
		t.Fatalf("diary: %v", err)
	}
	talk := dsl.App{
		SchemaVersion: dsl.SchemaVersion,
		App:           dsl.AppMeta{Name: "相談", VoiceEntry: dsl.VoiceEntryNone},
		InitialScreen: "form",
		Entities: []dsl.Entity{{ID: "talk", Name: "相談", Fields: []dsl.Field{
			{ID: "question", Name: "相談", Type: dsl.FieldText, Required: true},
			{ID: "answer", Name: "答え", Type: dsl.FieldText},
		}}},
		Screens: []dsl.Screen{{ID: "form", Title: "聞く", Entity: "talk", Components: []dsl.Component{
			{Type: dsl.CompAiButton, ID: "ask", Task: "generate", Entity: "talk", Limit: 10, Output: "answer",
				Prompt: "これまでの相談(下の材料)を踏まえて、次の相談に答えて: {{question}}"},
		}}},
	}
	if err := talk.Validate(); err != nil {
		t.Fatalf("talk: %v", err)
	}
}
