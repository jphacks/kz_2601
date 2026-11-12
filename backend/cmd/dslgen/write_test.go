package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestWriteRemovesStaleFiles(t *testing.T) {
	want, err := Generate(dsl.SchemaJSON())
	if err != nil {
		t.Fatal(err)
	}
	var same string
	for n := range want {
		same = n
		break
	}
	if same == "" {
		t.Fatal("Generate returned no files")
	}

	dir := filepath.Join(t.TempDir(), "generated") // write creates it
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	put := func(rel, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put("Stale.kt", "// renamed away\n")
	put(same, "// old content\n")
	put("README.md", "keep\n")
	put("notes.kt.bak", "keep\n")
	put(filepath.Join("nested", "Other.kt"), "keep\n")

	if err := write(dir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "Stale.kt")); !os.IsNotExist(err) {
		t.Errorf("Stale.kt still there (err=%v)", err)
	}
	for _, rel := range []string{"README.md", "notes.kt.bak", filepath.Join("nested", "Other.kt")} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("%s was removed: %v", rel, err)
		}
	}
	for n, body := range want {
		got, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Errorf("%s: %v", n, err)
			continue
		}
		if string(got) != body {
			t.Errorf("%s differs from Generate", n)
		}
	}
	kts, err := filepath.Glob(filepath.Join(dir, "*.kt"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range kts {
		names = append(names, filepath.Base(p))
	}
	var wantNames []string
	for n := range want {
		wantNames = append(wantNames, n)
	}
	slices.Sort(wantNames)
	if !slices.Equal(names, wantNames) {
		t.Errorf(".kt files = %v, want %v", names, wantNames)
	}
}
