package feeds

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

const feedsDoc = "../../../docs/backend/features/external-feeds.md"

var docURL = regexp.MustCompile("`(https?://[^`]+)`")

// docSourceURLs splits the "出どころと規約" section of a feature doc into the
// backticked URLs of the allowed table and of the "外したもの" table.
func docSourceURLs(t *testing.T, path string) (allowed, excluded []string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, sec, ok := strings.Cut(string(b), "\n## 出どころと規約\n")
	if !ok {
		t.Fatalf("%s: no 出どころと規約 section", path)
	}
	sec, _, _ = strings.Cut(sec, "\n## ")
	in, out, ok := strings.Cut(sec, "\n外したもの:")
	if !ok {
		t.Fatalf("%s: no 外したもの table", path)
	}
	for _, m := range docURL.FindAllStringSubmatch(in, -1) {
		allowed = append(allowed, m[1])
	}
	for _, m := range docURL.FindAllStringSubmatch(out, -1) {
		excluded = append(excluded, m[1])
	}
	return allowed, excluded
}

func documented(urls []string, prefix string) bool {
	for _, u := range urls {
		if strings.HasPrefix(u, prefix) {
			return true
		}
	}
	return false
}

// The code allowlist and the documented source table stay one list: every
// production URL is https and written in the table (a source is added only
// after its terms are checked and written there), every URL in the table is
// on the code allowlist, and no excluded source is reachable.
func TestAllowlistMatchesDoc(t *testing.T) {
	ep := DefaultEndpoints()
	allowed, excluded := docSourceURLs(t, feedsDoc)
	code := map[string]string{
		"weather": ep.WeatherBase, "warnings": ep.WarningBase, "pressure": ep.Pressure,
		"holidays": ep.Holidays, "arxiv": ep.Arxiv, "diet": ep.Diet,
	}
	for id, f := range ep.RSS {
		code["rss "+id] = f.URL
	}
	for name, raw := range code {
		if !strings.HasPrefix(raw, "https://") {
			t.Errorf("%s: %s is not https", name, raw)
		}
		if !documented(allowed, raw) {
			t.Errorf("%s: %s is not in the table of %s", name, raw, feedsDoc)
		}
	}
	for _, raw := range allowed {
		if !ep.allowedURL(raw) {
			t.Errorf("documented %s is not on the code allowlist", raw)
		}
	}
	if len(excluded) == 0 {
		t.Fatal("no excluded URLs found; the doc layout changed")
	}
	for _, raw := range excluded {
		if ep.allowedURL(raw) {
			t.Errorf("excluded %s is on the allowlist", raw)
		}
	}
}
