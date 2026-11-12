package ai

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The ai spec's servers end in /v1, so its paths are prefixed with it.
const aiSpecPrefix = "/v1"

// aiRoutesNotInSpec: registered routes deliberately left out of openapi-ai.yaml.
var aiRoutesNotInSpec = map[string]string{
	"GET /healthz": "local-only alias of /health; Cloud Run's front end swallows /healthz",
}

// aiSpecServedAs: spec operations served at a different pattern.
var aiSpecServedAs = map[string]string{
	// the spec lists /health under servers' /v1; the handler serves it at the root
	"GET /v1/health": "GET /health",
}

var (
	aiSpecPathLine   = regexp.MustCompile(`^  (/\S*):\s*$`)
	aiSpecMethodLine = regexp.MustCompile(`^    (get|put|post|delete|patch|head|options):\s*$`)
	aiPathParam      = regexp.MustCompile(`\{[^}]+\}`)
)

// aiMuxPatterns collects the string pattern of every mux.Handle / HandleFunc in
// the package sources.
func aiMuxPatterns(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var pats []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(af, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok || len(c.Args) != 2 {
				return true
			}
			if s, ok := c.Fun.(*ast.SelectorExpr); !ok || (s.Sel.Name != "Handle" && s.Sel.Name != "HandleFunc") {
				return true
			}
			l, ok := c.Args[0].(*ast.BasicLit)
			if !ok || l.Kind != token.STRING {
				t.Errorf("%s: %s pattern is not a string literal; this test cannot read it", f, c.Fun.(*ast.SelectorExpr).Sel.Name)
				return true
			}
			p, _ := strconv.Unquote(l.Value)
			pats = append(pats, p)
			return true
		})
	}
	return pats
}

// aiSpecOperations reads "METHOD /v1/path" for every operation under `paths:`.
func aiSpecOperations(t *testing.T) []string {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "..", "docs", "backend", "openapi-ai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var ops []string
	inPaths, path := false, ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line != "" && line[0] != ' ' && line[0] != '#' {
			inPaths, path = line == "paths:", ""
			continue
		}
		if !inPaths {
			continue
		}
		if m := aiSpecPathLine.FindStringSubmatch(line); m != nil {
			path = m[1]
		} else if m := aiSpecMethodLine.FindStringSubmatch(line); m != nil && path != "" {
			ops = append(ops, strings.ToUpper(m[1])+" "+aiSpecPrefix+path)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(ops) == 0 {
		t.Fatal("openapi-ai.yaml: no operations under paths:")
	}
	return ops
}

// TestRoutesMatchOpenAPI keeps the ai mux (generate, chat, text, health) and
// openapi-ai.yaml's paths in step (#2359). Parameter names are not compared.
func TestRoutesMatchOpenAPI(t *testing.T) {
	norm := func(r string) string { return aiPathParam.ReplaceAllString(r, "{}") }
	routes := map[string]string{}
	for _, p := range aiMuxPatterns(t) {
		if _, dup := routes[norm(p)]; dup {
			t.Errorf("%s registered twice", p)
		}
		routes[norm(p)] = p
	}
	spec := map[string]string{}
	for _, op := range aiSpecOperations(t) {
		if as, ok := aiSpecServedAs[op]; ok {
			op = as
		}
		spec[norm(op)] = op
	}
	for _, want := range []string{"POST /v1/generate", "POST /v1/chat", "POST /v1/text", "GET /health"} {
		if _, ok := routes[want]; !ok {
			t.Errorf("%s not found in the sources; the pattern reader is broken", want)
		}
	}

	for _, n := range slices.Sorted(maps.Keys(routes)) {
		r := routes[n]
		_, listed := aiRoutesNotInSpec[r]
		switch _, inSpec := spec[n]; {
		case !inSpec && !listed:
			t.Errorf("%s is served but not in openapi-ai.yaml (add it, or to aiRoutesNotInSpec with a reason)", r)
		case inSpec && listed:
			t.Errorf("%s is in the spec now; drop it from aiRoutesNotInSpec", r)
		}
	}
	for _, n := range slices.Sorted(maps.Keys(spec)) {
		if _, ok := routes[n]; !ok {
			t.Errorf("%s is in openapi-ai.yaml but the ai does not serve it", spec[n])
		}
	}
	for r, why := range aiRoutesNotInSpec {
		if _, ok := routes[norm(r)]; why == "" || !ok {
			t.Errorf("aiRoutesNotInSpec[%q]: needs a reason and a registered route", r)
		}
	}
}
