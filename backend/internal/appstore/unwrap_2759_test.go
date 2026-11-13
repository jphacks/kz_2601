package appstore

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorUnwrap2759(t *testing.T) {
	base := errors.New("boom")
	err := fmt.Errorf("wrapped: %w", reject(ReasonChain, "bad chain: %w", base))
	if !errors.Is(err, base) {
		t.Fatal("errors.Is does not reach the cause through *Error")
	}
	var e *Error
	if !errors.As(err, &e) || e.Reason != ReasonChain {
		t.Fatalf("errors.As: %+v", e)
	}
	if errors.Unwrap(e) == nil {
		t.Fatal("Unwrap returned nil")
	}
	if errors.Is(err, errors.New("other")) {
		t.Fatal("unrelated error matched")
	}
}
