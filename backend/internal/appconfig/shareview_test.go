package appconfig

import (
	"context"
	"flag"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/sharepage/*.html")

// Golden pages for docs/dsl/examples (#362). Regenerate with
// go test ./internal/appconfig -run TestSharePageGolden -update
func TestSharePageGolden(t *testing.T) {
	for _, name := range []string{"kakeibo", "gallery-input", "gallery-display", "gallery-structure", "chat"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "dsl", "examples", name+".json"))
			if err != nil {
				t.Fatal(err)
			}
			p := buildWebPage("", raw, nil)
			p.ImportURL, p.PageURL, p.Nonce = template.URL(ImportScheme+"tok"), "https://example.test/s/tok", "test-nonce"
			got, err := renderSharePage(p)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", "sharepage", name+".html")
			if *updateGolden {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0o600); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run with -update)", err)
			}
			if string(got) != string(want) {
				t.Errorf("%s differs from golden (run with -update and review the diff)", path)
			}
		})
	}
}

func getSharePage(t *testing.T, st store.Store, tok string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("GET /s/{token}", SharePage(st))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/s/"+tok, nil))
	return rr
}

const xss = `<script>alert(1)</script>"'><img src=x onerror=alert(1)>`

// Every DSL string is attacker-controlled: names, titles, labels, options,
// link URLs and the theme color must come out escaped or dropped.
func TestSharePageEscapesDSL(t *testing.T) {
	dslJSON := `{"schemaVersion":1,"app":{"name":"N","description":"` + jsonEsc(xss) + `","voiceEntry":"none",
	 "theme":{"primaryColor":"red;}body{display:none"}},
	 "initialScreen":"home","entities":[{"id":"e","name":"E","scope":"private","fields":[
	   {"id":"f","name":"` + jsonEsc(xss) + `","type":"enum","options":["` + jsonEsc(xss) + `"]}]}],
	 "screens":[{"id":"home","title":"` + jsonEsc(xss) + `","entity":"e","components":[
	   {"type":"text","text":"` + jsonEsc(xss) + `","area":"A1:F1"},
	   {"type":"picker","field":"f","area":"A2:F2"},
	   {"type":"link","text":"js","url":"javascript:alert(1)"},
	   {"type":"link","text":"ok","url":"https://example.com/a?b=1"},
	   {"type":"list","entity":"e","item":{"title":"f"},"emptyText":"` + jsonEsc(xss) + `"},
	   {"type":"futureWidget","title":"` + jsonEsc(xss) + `"}]}]}`
	st := store.NewMemory()
	if err := st.CreateShare(t.Context(), store.Share{Token: "tok_x", Name: xss, AppID: "a", DSL: []byte(dslJSON)}); err != nil {
		t.Fatal(err)
	}
	rr := getSharePage(t, st, "tok_x")
	body := rr.Body.String()
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	for _, bad := range []string{"<script", "<img", `"'>`, "javascript:", "display:none"} {
		if strings.Contains(body, bad) {
			t.Errorf("unescaped %q in page", bad)
		}
	}
	for _, want := range []string{"&lt;script&gt;", `href="https://example.com/a?b=1"`, "アプリで見られます", "tailorit://import/tok_x", AppStoreURL} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	csp := rr.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") || strings.Contains(csp, "unsafe-inline") || !strings.Contains(csp, "style-src 'nonce-") {
		t.Errorf("CSP: %q", csp)
	}
	nonce := strings.TrimSuffix(strings.SplitN(strings.SplitN(csp, "'nonce-", 2)[1], "'", 2)[0], "'")
	if !strings.Contains(body, `<style nonce="`+nonce+`">`) {
		t.Errorf("style nonce does not match CSP")
	}
	if strings.Contains(body, " style=") {
		t.Errorf("inline style attribute would be blocked by the CSP")
	}
	if got := rr.Header().Get("Cache-Control"); got != "private, max-age=60" {
		t.Errorf("Cache-Control %q", got)
	}
	if rr.Header().Get("Referrer-Policy") != "no-referrer" || rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("headers: %v", rr.Header())
	}
}

func jsonEsc(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// Records are never rendered (#253): the page only has the structure.
func TestSharePageNoRecords(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/dsl/examples/kakeibo.json")
	if err != nil {
		t.Fatal(err)
	}
	st := store.NewMemory()
	if err := st.CreateShare(t.Context(), store.Share{Token: "tok_k", Name: "家計簿", AppID: "a", DSL: raw}); err != nil {
		t.Fatal(err)
	}
	body := getSharePage(t, st, "tok_k").Body.String()
	for _, want := range []string{"<h1>家計簿</h1>", "今月の支出", "まだ支出がありません", `og:title" content="家計簿"`, "disabled"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, `<ul class="rows">`) {
		t.Error("records must not be shown")
	}
}

type fakePeek struct{ asked []string }

func (f *fakePeek) Rows(entity string, _ int) []RecordRow {
	f.asked = append(f.asked, entity)
	return []RecordRow{{Title: "<b>row</b>"}}
}

// The RecordPeek seam is only consulted for scope:"public" entities.
func TestRecordPeekPublicOnly(t *testing.T) {
	raw := []byte(`{"schemaVersion":1,"app":{"name":"B","voiceEntry":"none"},"initialScreen":"home",
	 "entities":[{"id":"pub","name":"P","scope":"public","fields":[{"id":"t","name":"T","type":"string"}]},
	             {"id":"fam","name":"F","scope":"shared","fields":[{"id":"t","name":"T","type":"string"}]},
	             {"id":"me","name":"M","fields":[{"id":"t","name":"T","type":"string"}]}],
	 "screens":[{"id":"home","title":"h","components":[
	   {"type":"list","entity":"pub","item":{"title":"t"}},
	   {"type":"list","entity":"fam","item":{"title":"t"}},
	   {"type":"list","entity":"me","item":{"title":"t"}}]}]}`)
	f := &fakePeek{}
	p := buildWebPage("B", raw, f)
	if len(f.asked) != 1 || f.asked[0] != "pub" {
		t.Fatalf("asked %v", f.asked)
	}
	out, err := renderSharePage(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "&lt;b&gt;row&lt;/b&gt;") {
		t.Error("peeked row missing or unescaped")
	}
}

// Render limits: a huge app is cut at maxWebNodes and says so.
func TestSharePageLimits(t *testing.T) {
	var comps []string
	for range maxWebNodes + 50 {
		comps = append(comps, `{"type":"text","text":"x"}`)
	}
	raw := []byte(`{"schemaVersion":1,"app":{"name":"L","voiceEntry":"none"},"initialScreen":"home","entities":[],
	 "screens":[{"id":"home","title":"h","components":[` + strings.Join(comps, ",") + `]}]}`)
	p := buildWebPage("L", raw, nil)
	if !p.Truncated || len(p.Screens[0].Stack) != maxWebNodes {
		t.Fatalf("truncated=%v nodes=%d", p.Truncated, len(p.Screens[0].Stack))
	}
	// broken JSON still gets the name and the call to action
	if p := buildWebPage("X", []byte(`{`), nil); !p.Found || p.Name != "X" || p.Screens != nil {
		t.Fatalf("broken: %+v", p)
	}
}

func TestSharePageHiddenAndRevoked(t *testing.T) {
	st := store.NewMemory()
	for _, tok := range []string{"tok_r", "tok_h"} {
		if err := st.CreateShare(t.Context(), store.Share{Token: tok, Name: "n", AppID: tok, OwnerUID: "o"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.RevokeShare(t.Context(), "tok_r"); err != nil {
		t.Fatal(err)
	}
	prev := shareHidden
	shareHidden = func(_ context.Context, _ store.Store, sh *store.Share) bool { return sh.AppID == "tok_h" }
	t.Cleanup(func() { shareHidden = prev })
	for _, tok := range []string{"tok_r", "tok_h", "missing", "bad%20tok"} {
		rr := getSharePage(t, st, tok)
		if rr.Code != http.StatusNotFound || rr.Header().Get("Cache-Control") != "no-store" || !strings.Contains(rr.Body.String(), "リンクが無効です") {
			t.Errorf("%s: %d %q", tok, rr.Code, rr.Header().Get("Cache-Control"))
		}
	}
}
