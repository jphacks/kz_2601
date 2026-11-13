package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestSplitList2757(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"  ", nil},
		{" , ,", nil},
		{"a", []string{"a"}},
		{"a,b", []string{"a", "b"}},
		{" a , b ", []string{"a", "b"}},
		{"a,b,", []string{"a", "b"}},
		{"a,,b", []string{"a", "b"}},
		// Duplicates are kept as given (no de-duplication).
		{"a,a", []string{"a", "a"}},
	}
	for _, c := range cases {
		if got := splitList(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitList(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func TestHasKind2757(t *testing.T) {
	s := &session{Steps: []step{{Kind: "a"}, {Kind: "b"}}}
	if !hasKind(s, "b") {
		t.Error("hasKind(b) = false, want true")
	}
	if hasKind(s, "c") {
		t.Error("hasKind(c) = true, want false")
	}
	if hasKind(&session{}, "a") {
		t.Error("hasKind on empty session = true, want false")
	}
}

func TestDefaultOutDir2757(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	got := defaultOutDir()
	if got == "" || !strings.HasPrefix(got, home) {
		t.Fatalf("defaultOutDir() = %q, want under %q", got, home)
	}
	// Must be outside the repository so reports are never committed.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(got, wd) {
		t.Errorf("defaultOutDir() = %q is inside the repo %q", got, wd)
	}
}
