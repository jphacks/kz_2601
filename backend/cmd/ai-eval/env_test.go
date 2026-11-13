package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// unsetForTest clears k for this test and restores the previous state afterwards.
func unsetForTest(t *testing.T, k string) {
	t.Helper()
	t.Setenv(k, "")
	if err := os.Unsetenv(k); err != nil {
		t.Fatal(err)
	}
}

func TestLoadEnvFile(t *testing.T) {
	const pre = "S4_AIEVAL_ENV_"
	for _, k := range []string{"A", "B", "C", "D", "E", "SPACED", "NOEQ", "EMPTY"} {
		unsetForTest(t, pre+k)
	}
	t.Setenv(pre+"KEEP", "original")

	body := "" +
		"export " + pre + "A=1\n" +
		"# " + pre + "D=commented\n" +
		"   # indented comment\n" +
		pre + "B=\"q\"\n" +
		pre + "C='s'\n" +
		pre + "NOEQ\n" +
		"\n" +
		"  " + pre + "SPACED = padded value  \n" +
		pre + "E=a=b\n" +
		pre + "EMPTY=\n" +
		pre + "KEEP=from-file\n"
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loadEnvFile(path); err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}

	want := map[string]string{
		"A":      "1",
		"B":      "q",
		"C":      "s",
		"SPACED": "padded value",
		"E":      "a=b",
		"EMPTY":  "",
		"KEEP":   "original",
	}
	for k, v := range want {
		got, ok := os.LookupEnv(pre + k)
		if !ok {
			t.Errorf("%s not set", pre+k)
			continue
		}
		if got != v {
			t.Errorf("%s = %q, want %q", pre+k, got, v)
		}
	}
	for _, k := range []string{"D", "NOEQ"} {
		if v, ok := os.LookupEnv(pre + k); ok {
			t.Errorf("%s should stay unset, got %q", pre+k, v)
		}
	}
}

func TestLoadEnvFileMissing(t *testing.T) {
	err := loadEnvFile(filepath.Join(t.TempDir(), "nope.env"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("loadEnvFile(missing) = %v, want fs.ErrNotExist", err)
	}
}
