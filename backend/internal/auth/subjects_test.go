package auth

import (
	"context"
	"slices"
	"testing"
)

// Subjects come from firebase.identities (Apple / Google only) and travel
// with the identity in the context.
func TestSubjects(t *testing.T) {
	got := subjectsOf(map[string]interface{}{
		"apple.com":  []interface{}{"001.abc", "001.abc"},
		"google.com": []interface{}{"g1"},
		"email":      []interface{}{"a@example.com"},
		"phone":      []interface{}{"+810000"},
	})
	if want := []string{"apple.com:001.abc", "google.com:g1"}; !slices.Equal(got, want) {
		t.Fatalf("subjects %v, want %v", got, want)
	}
	if got := subjectsOf(nil); len(got) != 0 {
		t.Fatalf("none %v", got)
	}
	if s := debugIdentity("anon-1").Subjects; s != nil {
		t.Fatalf("anonymous %v", s)
	}
	ctx := ContextWithIdentity(context.Background(), debugIdentity("user-1"))
	if s := Subjects(ctx); !slices.Equal(s, []string{"dev:user-1"}) || UID(ctx) != "user-1" {
		t.Fatalf("context %v", s)
	}
	if s := Subjects(context.Background()); s != nil {
		t.Fatalf("empty %v", s)
	}
}
