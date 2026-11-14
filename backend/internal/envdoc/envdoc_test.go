// Package envdoc only holds TestEnvDocs: it lists environment variables the
// backend reads that backend/README.md and docs/ never mention (#2360). It
// reports and never fails.
package envdoc

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	backendRoot = "../.."
	repoRoot    = "../../.."
)

// envReaders are the calls whose first argument is a variable name:
// os.Getenv / os.LookupEnv, the cmd helpers envOr / env / envFrom, and the
// injected getenv(func(string) string) most config readers take.
var envReaders = map[string]bool{"Getenv": true, "LookupEnv": true, "envOr": true, "env": true, "envFrom": true, "getenv": true}

// envName is what a variable name looks like; filters out map lookups in tests.
var envName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// readEnvNames maps each variable name to the packages (dir under backend/) reading it.
// Only string literals are seen; a name built at run time or held in a const is missed.
func readEnvNames(t *testing.T) map[string][]string {
	t.Helper()
	found := map[string][]string{}
	err := filepath.WalkDir(backendRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); n == "testdata" || n == "vendor" || (strings.HasPrefix(n, ".") && path != backendRoot) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		af, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		pkg, _ := filepath.Rel(backendRoot, filepath.Dir(path))
		ast.Inspect(af, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok || len(c.Args) == 0 {
				return true
			}
			var name string
			switch f := c.Fun.(type) {
			case *ast.Ident:
				name = f.Name
			case *ast.SelectorExpr:
				name = f.Sel.Name
			}
			lit, ok := c.Args[0].(*ast.BasicLit)
			if !envReaders[name] || !ok || lit.Kind != token.STRING {
				return true
			}
			v, _ := strconv.Unquote(lit.Value)
			if envName.MatchString(v) && !slices.Contains(found[v], pkg) {
				found[v] = append(found[v], pkg)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// readText concatenates backend/README.md, or every .md under docs/.
func readText(t *testing.T, root string, all bool) string {
	t.Helper()
	if !all {
		b, err := os.ReadFile(root)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	var sb strings.Builder
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		b, err := os.ReadFile(path)
		sb.Write(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

// mentions reports a whole-word occurrence (FOO does not count inside FOO_BAR).
func mentions(text, name string) bool {
	return regexp.MustCompile(`(^|[^A-Z0-9_])` + name + `($|[^A-Z0-9_])`).MatchString(text)
}

// TestEnvDocs: go test -run TestEnvDocs -v ./internal/envdoc
func TestEnvDocs(t *testing.T) {
	found := readEnvNames(t)
	readme := readText(t, filepath.Join(backendRoot, "README.md"), false)
	docs := readText(t, filepath.Join(repoRoot, "docs"), true)

	var nowhere, docsOnly []string
	for _, name := range slices.Sorted(maps.Keys(found)) {
		if mentions(readme, name) {
			continue
		}
		line := name + "  (" + strings.Join(slices.Sorted(slices.Values(found[name])), ", ") + ")"
		if mentions(docs, name) {
			docsOnly = append(docsOnly, line)
		} else {
			nowhere = append(nowhere, line)
		}
	}
	t.Logf("%d variables read; %d in backend/README.md", len(found), len(found)-len(nowhere)-len(docsOnly))
	t.Logf("in neither backend/README.md nor docs/ (%d):\n  %s", len(nowhere), strings.Join(nowhere, "\n  "))
	t.Logf("in docs/ but not backend/README.md (%d):\n  %s", len(docsOnly), strings.Join(docsOnly, "\n  "))
}
