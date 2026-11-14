package feeds

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// TestFeedsDocsTable (#2967): the providers the server accepts
// (dsl.FeedProviders) and the provider column of the source tables in
// external-feeds.md stay one list. A provider missing from the table, or a
// row for a provider that is not registered, fails.
func TestFeedsDocsTable(t *testing.T) {
	b, err := os.ReadFile(feedsDoc)
	if err != nil {
		t.Fatal(err)
	}
	_, sec, ok := strings.Cut(string(b), "\n## 出どころと規約\n")
	if !ok {
		t.Fatal("no 出どころと規約 section")
	}
	sec, _, _ = strings.Cut(sec, "\n## ")
	sec, _, ok = strings.Cut(sec, "\n外したもの:") // the excluded candidates are not providers
	if !ok {
		t.Fatal("no 外したもの table")
	}

	documented := map[string]bool{}
	for _, line := range strings.Split(sec, "\n") {
		if !strings.HasPrefix(line, "| ") {
			continue
		}
		cell, _, _ := strings.Cut(strings.TrimPrefix(line, "| "), " |")
		name := strings.TrimSpace(strings.Fields(cell + " ")[0])
		if name == "provider" || strings.Trim(name, "-") == "" {
			continue // header and separator rows
		}
		documented[name] = true
	}

	var missing, extra []string
	for p := range dsl.FeedProviders {
		if !documented[p] {
			missing = append(missing, p)
		}
	}
	for p := range documented {
		if !dsl.FeedProviders[p] {
			extra = append(extra, p)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		t.Errorf("providers not in the %s source table: %v", feedsDoc, missing)
	}
	if len(extra) > 0 {
		t.Errorf("table rows for providers that are not registered in dsl.FeedProviders: %v", extra)
	}
}
