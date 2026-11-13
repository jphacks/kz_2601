package ai

import (
	"testing"
	"time"
)

func TestRecipeModel_2599(t *testing.T) {
	if got := (&Service{}).recipeModel(); got != DefaultRecipeModel {
		t.Fatalf("default: %q", got)
	}
	if got := (&Service{RecipeModel: "claude-opus-5-5"}).recipeModel(); got != "claude-opus-5-5" {
		t.Fatalf("override: %q", got)
	}
}

func TestRecipeThinking_2599(t *testing.T) {
	tests := map[string]string{
		"claude-haiku-5-5":  "disabled",
		"claude-haiku-x":    "disabled",
		"claude-opus-5-5":   "",
		"claude-sonnet-5-5": "",
		"unknown-model":     "",
		"":                  "",
	}
	for m, want := range tests {
		if got := recipeThinking(m); got != want {
			t.Errorf("recipeThinking(%q)=%q want %q", m, got, want)
		}
	}
}

func TestChatTimeout_2599(t *testing.T) {
	if got := (&Service{}).chatTimeout(); got != 40*time.Second {
		t.Fatalf("default: %v", got)
	}
	if got := (&Service{CallTimeout: 3 * time.Second}).chatTimeout(); got != 3*time.Second {
		t.Fatalf("override: %v", got)
	}
}
