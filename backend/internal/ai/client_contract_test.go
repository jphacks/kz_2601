package ai

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// #353: the client's declared features and the backend's known keys must not
// drift. A key iOS sends that the backend does not know is silently ignored,
// so the feature stays off for that build.

const clientCapabilitiesSwift = "../../../ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/ClientCapabilities.swift"

var swiftStringList = regexp.MustCompile(`\[("[^"\]]+"(?:,\s*"[^"\]]+")*)\]`)

// iosLists returns the string-array literals inside the Swift block that
// starts at decl (review build first, then the normal build).
func iosLists(t *testing.T, decl string) [][]string {
	t.Helper()
	src := string(readDoc(t, clientCapabilitiesSwift))
	i := strings.Index(src, decl)
	if i < 0 {
		t.Fatalf("%s not found in ClientCapabilities.swift", decl)
	}
	block := src[i:]
	if j := strings.Index(block, "\n    }\n"); j >= 0 {
		block = block[:j]
	}
	var out [][]string
	for _, m := range swiftStringList.FindAllStringSubmatch(block, -1) {
		var keys []string
		for _, q := range strings.Split(m[1], ",") {
			keys = append(keys, strings.Trim(strings.TrimSpace(q), `"`))
		}
		out = append(out, keys)
	}
	return out
}

func TestIOSSupportedFeaturesAreKnown(t *testing.T) {
	lists := iosLists(t, "public static var supportedFeatures")
	if len(lists) != 2 {
		t.Fatalf("want review and normal lists, got %d", len(lists))
	}
	review, normal := lists[0], lists[1]
	for _, f := range normal {
		if !slices.Contains(currentFeatures, f) {
			t.Errorf("iOS sends %q but the backend does not know it", f)
		}
	}
	for _, f := range currentFeatures {
		if !slices.Contains(normal, f) {
			t.Errorf("backend feature %q is not sent by the normal iOS build", f)
		}
	}
	for _, f := range review {
		if !slices.Contains(normal, f) {
			t.Errorf("review build sends %q that the normal build does not", f)
		}
	}
}

func TestIOSDisabledFeaturesAreKnown(t *testing.T) {
	lists := iosLists(t, "public static let reviewDisabledFeatures")
	if len(lists) != 1 {
		t.Fatalf("want one list, got %d", len(lists))
	}
	known := []string{FeatureFieldHealth, FeatureCameraCapture, FeatureSharedData}
	for _, f := range lists[0] {
		if !slices.Contains(known, f) {
			t.Errorf("iOS disables %q but the backend does not know it", f)
		}
	}
}

// clientFeatures is the table the restriction reads; currentFeatures is the
// list tests use for a current build, so it must cover the table.
func TestCurrentFeaturesCoverTable(t *testing.T) {
	for _, f := range clientFeatures {
		if !slices.Contains(currentFeatures, f.key) {
			t.Errorf("clientFeature %q missing from currentFeatures", f.key)
		}
	}
}

func TestUnknownSupportedFeaturesAreIgnored(t *testing.T) {
	all := allComponentTypes()
	extra := append(slices.Clone(currentFeatures), "future.part", " ", "")
	if r := NewClientRestriction(all, nil, extra); r.Active() {
		t.Fatal("unknown supportedFeatures must not restrict a current build")
	}
	less := featuresWithout(FeatureAILong)
	a := NewClientRestriction(all, nil, less)
	b := NewClientRestriction(all, nil, append(slices.Clone(less), "future.part"))
	if !a.Active() || a.key != b.key {
		t.Fatalf("unknown key changed the restriction: %q vs %q", a.key, b.key)
	}
	if NewClientRestriction(nil, nil, []string{"future.part"}).Active() {
		t.Fatal("no allowedComponents = no restriction")
	}
}

// Every known supportedFeatures value is listed in the API contract and the DSL README §5.2.
func TestSupportedFeaturesDocumented(t *testing.T) {
	for _, doc := range []string{
		filepath.Join("..", "..", "..", "docs", "backend", "openapi-ai.yaml"),
		filepath.Join("..", "..", "..", "docs", "dsl", "README.md"),
	} {
		b, err := os.ReadFile(doc)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range currentFeatures {
			if !strings.Contains(string(b), "`"+f+"`") {
				t.Errorf("%s does not list supportedFeatures %q", filepath.Base(doc), f)
			}
		}
	}
}
