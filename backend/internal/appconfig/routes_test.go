package appconfig

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// wantRoutes is the full v1 route list. A new feature adds its line here.
var wantRoutes = []string{
	"DELETE /v1/apps/{appId}",
	"DELETE /v1/apps/{appId}/entities/{entityId}/records",
	"DELETE /v1/apps/{appId}/entities/{entityId}/records/{recordId}",
	"DELETE /v1/apps/{appId}/entities/{entityId}/records/{recordId}/marks/{markId}",
	"DELETE /v1/apps/{appId}/interests/{uid}",
	"DELETE /v1/apps/{appId}/matches/{uid}",
	"DELETE /v1/apps/{appId}/members/{uid}",
	"DELETE /v1/apps/{appId}/publish",
	"DELETE /v1/me",
	"DELETE /v1/me/blocks/{uid}",
	"DELETE /v1/me/devices/{installationId}",
	"DELETE /v1/shares/{token}",
	"GET /v1/apps",
	"GET /v1/apps/{appId}",
	"GET /v1/apps/{appId}/automations",
	"GET /v1/apps/{appId}/dsl",
	"GET /v1/apps/{appId}/entities",
	"GET /v1/apps/{appId}/entities/{entityId}",
	"GET /v1/apps/{appId}/entities/{entityId}/records",
	"GET /v1/apps/{appId}/entities/{entityId}/records/{recordId}",
	"GET /v1/apps/{appId}/events",
	"GET /v1/apps/{appId}/interests",
	"GET /v1/apps/{appId}/matches",
	"GET /v1/apps/{appId}/members",
	"GET /v1/apps/{appId}/release-requests",
	"GET /v1/apps/{appId}/versions",
	"GET /v1/apps/{appId}/versions/{version}",
	"GET /v1/me/blocks",
	"GET /v1/public-apps",
	"GET /v1/release-requests",
	"GET /v1/shares/{token}",
	"PATCH /v1/apps/{appId}",
	"PATCH /v1/apps/{appId}/dsl",
	"PATCH /v1/apps/{appId}/entities/{entityId}/records/{recordId}",
	"PATCH /v1/apps/{appId}/members/{uid}",
	"POST /v1/apps",
	"POST /v1/apps/{appId}/duplicate",
	"POST /v1/apps/{appId}/entities/{entityId}/records",
	"POST /v1/apps/{appId}/interests",
	"POST /v1/apps/{appId}/interests/{uid}/accept",
	"POST /v1/apps/{appId}/publish",
	"POST /v1/apps/{appId}/release-requests",
	"POST /v1/apps/{appId}/share",
	"POST /v1/apps/{appId}/versions/{version}/restore",
	"POST /v1/feedback",
	"POST /v1/me/blocks",
	"POST /v1/metrics/ai-button",
	"POST /v1/public-apps/{id}/install",
	"POST /v1/reports",
	"POST /v1/shares/{token}/import",
	"PUT /v1/apps/order",
	"PUT /v1/apps/{appId}/dsl",
	"PUT /v1/apps/{appId}/entities/{entityId}/records/{recordId}/marks/{markId}",
	"PUT /v1/me/devices/{installationId}",
	"PUT /v1/me/notify-consent/{appId}",
}

// handleFuncPatterns collects every mux.HandleFunc pattern in the package sources.
func handleFuncPatterns(t *testing.T) []string {
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
			if !ok || len(c.Args) == 0 {
				return true
			}
			if s, ok := c.Fun.(*ast.SelectorExpr); !ok || s.Sel.Name != "HandleFunc" {
				return true
			}
			if l, ok := c.Args[0].(*ast.BasicLit); ok {
				p, _ := strconv.Unquote(l.Value)
				pats = append(pats, p)
			}
			return true
		})
	}
	slices.Sort(pats)
	return pats
}

func TestRoutes_FullList(t *testing.T) {
	want := slices.Sorted(slices.Values(wantRoutes))
	if got := handleFuncPatterns(t); !slices.Equal(got, want) {
		t.Fatalf("routes in source differ from wantRoutes\ngot:  %q\nwant: %q", got, want)
	}
	mux := New(store.NewMemory()) // duplicate patterns panic here
	param := regexp.MustCompile(`\{[^}]+\}`)
	for _, p := range want {
		method, path, _ := strings.Cut(p, " ")
		req := httptest.NewRequest(method, param.ReplaceAllString(path, "x1"), nil)
		if _, got := mux.Handler(req); got != p {
			t.Errorf("%s: matched %q (not registered by New?)", p, got)
		}
	}
}
