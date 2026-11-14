package dsl

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

const enumReportMin = 15 // only enums with more values than this are reported

// TestDSLEnumDocsReport_2970 logs the values of large schema enums (more than
// 15 values, e.g. PlaceSearch.category) that docs/dsl/README.md and
// expressions.md never write as `value`. It never fails: it is a hint for when
// to file an issue to document them. See it with `go test -v -run DSLEnumDocsReport`.
func TestDSLEnumDocsReport_2970(t *testing.T) {
	var docs strings.Builder
	for _, p := range []string{"../../../docs/dsl/README.md", "../../../docs/dsl/expressions.md"} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		docs.Write(b)
	}
	var root any
	if err := json.Unmarshal(SchemaJSON(), &root); err != nil {
		t.Fatal(err)
	}

	var lines []string
	var walk func(path string, v any)
	walk = func(path string, v any) {
		switch x := v.(type) {
		case map[string]any:
			if enum, ok := x["enum"].([]any); ok && len(enum) > enumReportMin {
				var missing []string
				for _, e := range enum {
					if s, ok := e.(string); ok && !strings.Contains(docs.String(), "`"+s+"`") {
						missing = append(missing, s)
					}
				}
				if len(missing) > 0 {
					lines = append(lines, path+": "+itoa(len(missing))+"/"+itoa(len(enum))+" not in docs: "+strings.Join(missing, ", "))
				}
			}
			for k, c := range x {
				walk(path+"."+k, c)
			}
		case []any:
			for i, c := range x {
				walk(path+"["+itoa(i)+"]", c)
			}
		}
	}
	walk("#", root)

	sort.Strings(lines)
	if len(lines) == 0 {
		t.Log("every value of the large enums is written in docs/dsl")
		return
	}
	for _, l := range lines {
		t.Log(l)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
