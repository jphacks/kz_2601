package auth

import (
	"context"
	"testing"
)

func TestDevVerifier(t *testing.T) {
	var v TokenVerifier = DevVerifier{}
	uid, err := v.VerifyIDToken(context.Background(), "dev-smoke")
	if err != nil || uid != "dev-smoke" {
		t.Fatalf("got %q, %v", uid, err)
	}
	if _, err := v.VerifyIDToken(context.Background(), "bad uid!"); err == nil {
		t.Fatal("invalid characters must be rejected")
	}
}
