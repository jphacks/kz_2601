package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestModeAndSourceArgs(t *testing.T) {
	cases := []struct {
		apply      bool
		export     string
		wantMode   string
		wantSource string
	}{
		{false, "", "dry-run", "firebase auth"}, // the defaults of -apply / -export
		{true, "", "APPLY", "firebase auth"},
		{false, "users.json", "dry-run", "export file"},
		{true, "users.json", "APPLY", "export file"},
	}
	for _, c := range cases {
		if got := mode(c.apply); got != c.wantMode {
			t.Errorf("mode(%v) = %q, want %q", c.apply, got, c.wantMode)
		}
		if got := source(c.export); got != c.wantSource {
			t.Errorf("source(%q) = %q, want %q", c.export, got, c.wantSource)
		}
	}
}

func TestErrorCodeArgs(t *testing.T) {
	secret := "uid-abc user@example.com"
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"grpc code", status.Error(codes.PermissionDenied, secret), "PermissionDenied"},
		{"wrapped grpc code", fmt.Errorf("write: %w", status.Error(codes.Aborted, secret)), "Aborted"},
		{"grpc unknown", status.Error(codes.Unknown, secret), "unknown"},
		{"plain error", errors.New(secret), "unknown"},
		// a bare context error is not a grpc status: it stays "unknown"
		{"bare context error", context.DeadlineExceeded, "unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := errorCode(c.err)
			if got != c.want {
				t.Fatalf("errorCode = %q, want %q", got, c.want)
			}
			if strings.Contains(got, "uid-abc") || strings.Contains(got, "@") {
				t.Fatalf("errorCode leaked the error text: %q", got)
			}
		})
	}
}
