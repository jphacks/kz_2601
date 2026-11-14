package books

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

const booksDoc = "../../../docs/backend/features/book-lookup.md"

// The source in the code is https and is the one written (terms checked) in
// the doc's source table; the kept cover host is the documented one.
func TestEndpointMatchesDoc(t *testing.T) {
	b, err := os.ReadFile(booksDoc)
	if err != nil {
		t.Fatal(err)
	}
	_, sec, ok := strings.Cut(string(b), "\n## 出どころと規約\n")
	if !ok {
		t.Fatal("no 出どころと規約 section")
	}
	sec, _, _ = strings.Cut(sec, "\n外したもの:")
	var urls []string
	for _, m := range regexp.MustCompile("`(https?://[^`]+)`").FindAllStringSubmatch(sec, -1) {
		urls = append(urls, m[1])
	}
	ep := DefaultEndpoint()
	if !strings.HasPrefix(ep.GetURL, "https://") {
		t.Errorf("GetURL %s is not https", ep.GetURL)
	}
	found := false
	for _, u := range urls {
		found = found || strings.HasPrefix(u, ep.GetURL)
	}
	if !found {
		t.Errorf("GetURL %s is not in the table of %s (%v)", ep.GetURL, booksDoc, urls)
	}
	if !strings.Contains(string(b), "`https://"+ep.CoverHost+"/`") {
		t.Errorf("cover host %s is not documented", ep.CoverHost)
	}
}
