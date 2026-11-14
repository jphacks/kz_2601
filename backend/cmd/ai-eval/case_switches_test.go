package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/ai"
)

func TestCaseSwitchesTextImage(t *testing.T) {
	svc := &ai.Service{}
	restore := caseSwitches(svc, Case{ID: "p", TextImage: true})
	if !svc.TextImage {
		t.Fatal("textImage case: TextImage off")
	}
	restore()
	if svc.TextImage {
		t.Fatal("not restored")
	}
	svc.TextImage = true
	caseSwitches(svc, Case{ID: "q"})
	if svc.TextImage {
		t.Fatal("case without textImage: TextImage still on")
	}
}
