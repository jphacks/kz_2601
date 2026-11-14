package runner_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// #337 layer 1: the engine (and notify, which reads through interfaces)
// never import internal/store, so they cannot name another app's data; only
// the appId-bound handle (scope) talks to the store.
func TestImportBoundary(t *testing.T) {
	for _, dir := range []string{"engine", "notify", "notify/apns"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("%s: no files (%v)", dir, err)
		}
		for _, f := range files {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), f, src, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range parsed.Imports {
				path, _ := strconv.Unquote(imp.Path.Value)
				if strings.HasSuffix(path, "/internal/store") || strings.Contains(path, "/internal/store/") ||
					strings.HasSuffix(path, "/internal/runner/scope") || path == "cloud.google.com/go/firestore" {
					t.Errorf("%s imports %s", f, path)
				}
			}
		}
	}
}
