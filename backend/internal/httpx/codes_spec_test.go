package httpx

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Each service's packages, relative to backend/. Shared packages (middleware)
// count for both. A package that writes an error code must be listed here.
var (
	sharedCodePkgs    = []string{"internal/appcheck", "internal/auth", "internal/httpx", "internal/ratelimit"}
	appconfigCodePkgs = []string{"cmd/appconfig", "internal/appconfig", "internal/calls", "internal/coins", "internal/coins/coinsapi", "internal/entitlement"}
	aiCodePkgs        = []string{"cmd/ai", "internal/ai", "internal/appconfigclient", "internal/budget", "internal/coins"}
)

// aiRelayedCodes are appconfig codes the ai passes through unchanged from the
// DSL / versions endpoints it calls (ai.mapStoreErr).
var aiRelayedCodes = []string{
	CodeAccountDeleted,
	CodeAppCheckFailed,
	CodeAppLimitReached,
	CodeBadRequest,
	CodeForbidden,
	CodeInternal,
	CodeInvalidDSL,
	CodeNotFound,
	CodePayloadTooLarge,
	CodePlanLimitReached,
	CodeUnauthenticated,
	CodeVersionConflict,
}

const backendRoot = "../.."

// TestErrorCodesMatchSpec keeps the `error.code` enum of each OpenAPI spec equal
// to the codes its service's packages write (#523).
func TestErrorCodesMatchSpec(t *testing.T) {
	consts := codeConsts(t)
	used := codesUsedByPkg(t, consts)

	for pkg := range used {
		if !slices.Contains(sharedCodePkgs, pkg) && !slices.Contains(appconfigCodePkgs, pkg) && !slices.Contains(aiCodePkgs, pkg) {
			t.Errorf("%s writes error codes but is not assigned to a service in codes_spec_test.go", pkg)
		}
	}

	collect := func(pkgs []string, extra []string) []string {
		set := map[string]bool{}
		for _, p := range append(slices.Clone(sharedCodePkgs), pkgs...) {
			for c := range used[p] {
				set[c] = true
			}
		}
		for _, c := range extra {
			set[c] = true
		}
		out := make([]string, 0, len(set))
		for c := range set {
			out = append(out, c)
		}
		slices.Sort(out)
		return out
	}

	cases := []struct {
		spec string
		want []string
	}{
		{"openapi-appconfig.yaml", collect(appconfigCodePkgs, nil)},
		{"openapi-ai.yaml", collect(aiCodePkgs, aiRelayedCodes)},
	}
	all := map[string]bool{}
	for _, tc := range cases {
		got := specErrorEnum(t, filepath.Join(backendRoot, "..", "docs", "backend", tc.spec))
		for _, c := range got {
			all[c] = true
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s Error.code enum\n got: %v\nwant: %v\nmissing: %v\nextra: %v",
				tc.spec, got, tc.want, minus(tc.want, got), minus(got, tc.want))
		}
	}
	for name, v := range consts {
		if !all[v] {
			t.Errorf("%s (%q) is in codes.go but no service returns it", name, v)
		}
	}
}

// codeConsts parses codes.go: constant name -> value. Values must be unique
// and the declarations sorted by value (#516).
func codeConsts(t *testing.T) map[string]string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "codes.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	var order []string
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, s := range gd.Specs {
			vs := s.(*ast.ValueSpec)
			for i, n := range vs.Names {
				v, err := strconv.Unquote(vs.Values[i].(*ast.BasicLit).Value)
				if err != nil {
					t.Fatal(err)
				}
				if slices.Contains(order, v) {
					t.Errorf("duplicate code %q", v)
				}
				out[n.Name] = v
				order = append(order, v)
			}
		}
	}
	if !slices.IsSorted(order) {
		t.Errorf("codes.go is not sorted by value: %v", order)
	}
	return out
}

// codesUsedByPkg scans the non-test Go files under internal/ and cmd/ for
// references to the code constants (httpx.CodeX, or CodeX inside httpx).
// Writing a code as a string literal (`Code: "x"`, apiErr(_, "x", _)) fails.
func codesUsedByPkg(t *testing.T, consts map[string]string) map[string]map[string]bool {
	t.Helper()
	used := map[string]map[string]bool{}
	fset := token.NewFileSet()
	for _, top := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(backendRoot, top), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			rel, _ := filepath.Rel(backendRoot, filepath.Dir(path))
			rel = filepath.ToSlash(rel)
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			add := func(name string) {
				if v, ok := consts[name]; ok {
					if used[rel] == nil {
						used[rel] = map[string]bool{}
					}
					used[rel][v] = true
				}
			}
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.SelectorExpr:
					if x, ok := n.X.(*ast.Ident); ok && x.Name == "httpx" {
						add(n.Sel.Name)
					}
				case *ast.Ident:
					if rel == "internal/httpx" && filepath.Base(path) != "codes.go" {
						add(n.Name)
					}
				case *ast.KeyValueExpr:
					if k, ok := n.Key.(*ast.Ident); ok && k.Name == "Code" && isStringLit(n.Value) {
						t.Errorf("%s: error code written as a literal; use an httpx.Code* constant", fset.Position(n.Pos()))
					}
				case *ast.CallExpr:
					if i := codeArg(n.Fun); i >= 0 && i < len(n.Args) && isStringLit(n.Args[i]) {
						t.Errorf("%s: error code written as a literal; use an httpx.Code* constant", fset.Position(n.Args[i].Pos()))
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return used
}

// codeArg is the index of the code argument of apiErr(status, code, msg) and
// httpx.Error / ErrorWithDetails(w, status, code, ...), or -1.
func codeArg(fun ast.Expr) int {
	switch f := fun.(type) {
	case *ast.Ident:
		if f.Name == "apiErr" {
			return 1
		}
	case *ast.SelectorExpr:
		if x, ok := f.X.(*ast.Ident); ok && x.Name == "httpx" && (f.Sel.Name == "Error" || f.Sel.Name == "ErrorWithDetails") {
			return 2
		}
	}
	return -1
}

func isStringLit(e ast.Expr) bool {
	b, ok := e.(*ast.BasicLit)
	return ok && b.Kind == token.STRING
}

// specErrorEnum returns the block list under components/schemas/Error ->
// error -> code -> enum. Per #516 the enum is a block list, one item per line,
// sorted; anything else fails.
func specErrorEnum(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	indent := func(s string) int { return len(s) - len(strings.TrimLeft(s, " ")) }
	// Keys to descend through, each deeper than the previous one.
	keys := []string{"components:", "schemas:", "Error:", "code:", "enum:"}
	step, depth := 0, -1
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), " ")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if step < len(keys) {
			if trimmed == keys[step] && indent(line) > depth {
				step, depth = step+1, indent(line)
			}
			continue
		}
		if indent(line) <= depth || !strings.HasPrefix(trimmed, "- ") {
			break
		}
		out = append(out, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if step < len(keys) || len(out) == 0 {
		t.Fatalf("%s: no block list under components.schemas.Error...code.enum", path)
	}
	if !slices.IsSorted(out) || len(slices.Compact(slices.Clone(out))) != len(out) {
		t.Errorf("%s: Error.code enum must be sorted and unique: %v", path, out)
	}
	return out
}

func minus(a, b []string) []string {
	var out []string
	for _, x := range a {
		if !slices.Contains(b, x) {
			out = append(out, x)
		}
	}
	return out
}
