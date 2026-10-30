package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckPhotoAI(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "dsl", "examples", "receipt_ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if miss := checkPhotoAI(map[string]any{"photoAnalyze": []any{"amount", "date"}}, receipt); miss != nil {
		t.Fatalf("receipt should pass: %v", miss)
	}
	if miss := checkPhotoAI(map[string]any{"photoAnalyze": []any{"jointAngle"}}, receipt); miss == nil {
		t.Fatal("receipt has no jointAngle")
	}
	if miss := checkPhotoAI(map[string]any{}, receipt); miss != nil {
		t.Fatalf("no key = no check: %v", miss)
	}
}
