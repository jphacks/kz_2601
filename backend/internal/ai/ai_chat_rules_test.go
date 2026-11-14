package ai

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestAIChatRulesReachPrompt(t *testing.T) {
	for _, want := range []string{"## アプリの中の AI と何往復も話す(aiChat)", "aiButton との使い分け", "は aiChat(上の節", "会話の entity は private", "roleField は必須", "は aiChat に speak: true"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	// the chat rules come before the cloud AI rules that point "上の節"
	if strings.Index(systemPrompt, aiChatRules) > strings.Index(systemPrompt, cloudAIRules) {
		t.Fatal("aiChat rules must precede the cloud AI rules")
	}
}

// The shape the rules teach (turn{text, role} + aiChat) validates and lints
// clean; the example of docs/dsl is the same shape.
func TestAIChatRulesShapeValidates(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "dsl", "examples", "ai_counselor.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if w := app.Lint(); len(w) != 0 {
		t.Fatalf("lint: %v", w)
	}
}

// A client without aiChat (an older app or the review build) loses the
// routing lines; the fallback to talk records stays.
func TestAIChatRulesDroppedWithoutAIChat(t *testing.T) {
	var allowed []string
	for c := range knownComponentTypes {
		if c != string(dsl.CompAiChat) {
			allowed = append(allowed, c)
		}
	}
	ctx := withRestriction(context.Background(), NewRestriction(allowed, nil))
	got := withoutNote(restrictPrompt(ctx, systemPrompt))
	if strings.Contains(got, "」は aiChat") || strings.Contains(got, "speak: true") {
		t.Fatal("aiChat routing line still in the restricted prompt")
	}
	if !strings.Contains(got, "talk{question") {
		t.Fatal("talk-record fallback missing")
	}
}
