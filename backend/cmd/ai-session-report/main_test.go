package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/fsenv"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

func hashOf(uid string) string { return httpx.UIDHash(uid) }

func TestGuard(t *testing.T) {
	cases := []struct {
		name      string
		env       map[string]string
		allowProd bool
		ok        bool
	}{
		{"no project", map[string]string{}, false, false},
		{"prod refused", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject}, false, false},
		{"prod allowed", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject}, true, true},
		{"prod named db is not prod", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject, "FIRESTORE_DATABASE": "stg"}, false, true},
		{"emulator", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject, "FIRESTORE_EMULATOR_HOST": "127.0.0.1:8089"}, false, true},
		{"stg on prod without override", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject, "APP_ENV": "stg"}, true, false},
		{"other project", map[string]string{"GOOGLE_CLOUD_PROJECT": "demo-kz2601"}, false, true},
	}
	for _, c := range cases {
		getenv := func(k string) string { return c.env[k] }
		err := guard(fsenv.FromEnv(getenv), c.allowProd, getenv)
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v", c.name, err)
		}
	}
}

func TestReadUIDs(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "uids.txt")
	if err := os.WriteFile(file, []byte("# consenting users\nu2  # second\n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readUIDs("u1", file)
	if err != nil || strings.Join(got, ",") != "u1,u2" {
		t.Fatalf("got %v %v", got, err)
	}
	if _, err := readUIDs("u1,u1", ""); err == nil {
		t.Error("duplicate accepted")
	}
	if _, err := readUIDs("bad uid", ""); err == nil || strings.Contains(err.Error(), "bad uid") {
		t.Errorf("malformed uid must be refused without echoing it: %v", err)
	}
	many := make([]string, maxUIDs+1)
	for i := range many {
		many[i] = "u" + strings.Repeat("x", i+1)
	}
	if _, err := readUIDs(strings.Join(many, ","), ""); err == nil {
		t.Error("too many uids accepted")
	}
}

func TestValidate(t *testing.T) {
	out := t.TempDir()
	ok := options{uids: []string{"u1"}, outDir: out, format: "md", days: 14, cfg: config{Gap: time.Minute, Top: 1}}
	if err := validate(ok); err != nil {
		t.Fatal(err)
	}
	none := ok
	none.uids = nil
	if err := validate(none); err == nil {
		t.Error("no uids must be refused (never read every user)")
	}
	bad := ok
	bad.format = "csv"
	if err := validate(bad); err == nil {
		t.Error("bad format accepted")
	}
	long := ok
	long.days = 365
	if err := validate(long); err == nil {
		t.Error("days > 90 accepted")
	}
	// this test runs inside the repository: an out-dir here must be refused
	wd, _ := os.Getwd()
	inRepo := ok
	inRepo.outDir = filepath.Join(wd, "reports")
	if err := validate(inRepo); err == nil || !strings.Contains(err.Error(), "git repository") {
		t.Errorf("out-dir inside the repository accepted: %v", err)
	}
}

// TestNoWriteCalls keeps the tool read-only by construction: only
// firestore.go may import the Firestore package, it may not call any write
// API, and no file imports internal/store (which has write methods).
func TestNoWriteCalls(t *testing.T) {
	forbidden := map[string]bool{
		"Set": true, "Create": true, "Update": true, "Delete": true, "Add": true,
		"Batch": true, "BulkWriter": true, "RunTransaction": true, "NewDoc": true,
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.HasSuffix(path, "internal/store") {
				t.Errorf("%s imports internal/store (it has write methods): read through fsReader only", name)
			}
			if strings.HasPrefix(path, "cloud.google.com/go/firestore") && name != "firestore.go" {
				t.Errorf("%s imports %s: only firestore.go (fsReader) may", name, path)
			}
		}
		if name != "firestore.go" {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && forbidden[sel.Sel.Name] {
				t.Errorf("%s: write-like call .%s()", fset.Position(call.Pos()), sel.Sel.Name)
			}
			return true
		})
	}
	if checked < 4 {
		t.Fatalf("checked only %d files", checked)
	}
}

func TestLogFilter(t *testing.T) {
	since := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	got := logFilter([]string{"u1", "u2"}, []string{"ai-prod"}, since, since.Add(14*24*time.Hour))
	for _, want := range []string{`service_name=("ai-prod")`, hashOf("u1"), hashOf("u2"), `"ai.generate" OR "ai.usage"`, `"ai.chat"`, `timestamp>="2026-10-23T00:00:00Z"`} {
		if !strings.Contains(got, want) {
			t.Errorf("filter misses %s: %s", want, got)
		}
	}
	if strings.Contains(got, `"u1"`) {
		t.Error("filter must carry hashes, not raw uids")
	}
}
