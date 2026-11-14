package appconfig

import (
	"bufio"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The appconfig spec's servers end in /v1, so its paths are prefixed with it.
const appconfigSpecPrefix = "/v1"

// notInAppconfigSpec: routes in wantRoutes deliberately left out of
// openapi-appconfig.yaml. Every entry needs a reason.
var notInAppconfigSpec = map[string]string{}

// servedOutsideAppconfigPkg: spec operations registered by another package of
// the appconfig service, so they are not in wantRoutes.
var servedOutsideAppconfigPkg = map[string]string{
	"GET /v1/client-config":             "cmd/appconfig (appconfig.ClientConfig on the outer mux)",
	"GET /v1/health":                    "cmd/appconfig serves GET /health outside /v1 (the spec puts it under servers' /v1)",
	"GET /v1/coins/config":              "internal/coins/coinsapi",
	"GET /v1/me/coins":                  "internal/coins/coinsapi",
	"GET /v1/me/coins/ledger":           "internal/coins/coinsapi",
	"GET /v1/me/entitlements":           "internal/entitlement",
	"POST /v1/appstore/notifications":   "internal/entitlement (App Store Server Notifications)",
	"POST /v1/calls/webhook":            "internal/calls",
	"POST /v1/googleplay/notifications": "internal/entitlement (Play RTDN)",
	"POST /v1/me/coins/apple":           "internal/coins/coinsapi",
	"POST /v1/me/coins/slot":            "internal/coins/coinsapi",
	"POST /v1/me/entitlements/apple":    "internal/entitlement",
	"POST /v1/me/entitlements/google":   "internal/entitlement",
	"PUT /v1/me/coins/settings":         "internal/coins/coinsapi",
	// known drift, not served anywhere: remove from the spec or implement (#2358)
	"PUT /v1/apps/{appId}/entities/{entityId}": "not implemented; the spec still lists it",
}

var (
	specPathLine   = regexp.MustCompile(`^  (/\S*):\s*$`)
	specMethodLine = regexp.MustCompile(`^    (get|put|post|delete|patch|head|options):\s*$`)
	pathParam      = regexp.MustCompile(`\{[^}]+\}`)
)

// specOperations reads "METHOD /path" for every operation under `paths:`.
func specOperations(t *testing.T, file, prefix string) []string {
	t.Helper()
	f, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var ops []string
	inPaths, path := false, ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line != "" && line[0] != ' ' && line[0] != '#' {
			inPaths = line == "paths:"
			path = ""
			continue
		}
		if !inPaths {
			continue
		}
		if m := specPathLine.FindStringSubmatch(line); m != nil {
			path = m[1]
			continue
		}
		if m := specMethodLine.FindStringSubmatch(line); m != nil && path != "" {
			ops = append(ops, strings.ToUpper(m[1])+" "+prefix+path)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(ops) == 0 {
		t.Fatalf("%s: no operations under paths:", file)
	}
	return ops
}

// normalizeRoute blanks parameter names: {id} and {appId} compare equal.
func normalizeRoute(r string) string { return pathParam.ReplaceAllString(r, "{}") }

// TestRoutesMatchOpenAPI keeps wantRoutes and openapi-appconfig.yaml's paths in
// step (#2358). A gap is fixed in the spec, or listed above with a reason.
func TestRoutesMatchOpenAPI(t *testing.T) {
	spec := map[string]string{} // normalized -> as written
	for _, op := range specOperations(t, filepath.Join("..", "..", "..", "docs", "backend", "openapi-appconfig.yaml"), appconfigSpecPrefix) {
		spec[normalizeRoute(op)] = op
	}
	routes := map[string]string{}
	for _, r := range wantRoutes {
		routes[normalizeRoute(r)] = r
	}

	for _, n := range slices.Sorted(maps.Keys(routes)) {
		r := routes[n]
		_, listed := notInAppconfigSpec[r]
		switch _, inSpec := spec[n]; {
		case !inSpec && !listed:
			t.Errorf("%s is served but not in openapi-appconfig.yaml (add it, or to notInAppconfigSpec with a reason)", r)
		case inSpec && listed:
			t.Errorf("%s is in the spec now; drop it from notInAppconfigSpec", r)
		}
	}
	for _, n := range slices.Sorted(maps.Keys(spec)) {
		op := spec[n]
		_, elsewhere := servedOutsideAppconfigPkg[op]
		switch _, served := routes[n]; {
		case !served && !elsewhere:
			t.Errorf("%s is in openapi-appconfig.yaml but not in wantRoutes (stale spec, or add to servedOutsideAppconfigPkg)", op)
		case served && elsewhere:
			t.Errorf("%s is in wantRoutes; drop it from servedOutsideAppconfigPkg", op)
		}
	}
	for r, why := range notInAppconfigSpec {
		if why == "" || !slices.Contains(wantRoutes, r) {
			t.Errorf("notInAppconfigSpec[%q]: needs a reason and a matching wantRoutes entry", r)
		}
	}
	for op, why := range servedOutsideAppconfigPkg {
		if _, ok := spec[normalizeRoute(op)]; why == "" || !ok {
			t.Errorf("servedOutsideAppconfigPkg[%q]: needs a reason and a matching spec operation", op)
		}
	}
}
