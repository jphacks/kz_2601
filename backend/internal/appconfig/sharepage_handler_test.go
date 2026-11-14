package appconfig

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// #362: handler paths the page tests above do not reach.

func shareStore(t *testing.T, tok, dslJSON string) store.Store {
	t.Helper()
	st := store.NewMemory()
	if err := st.CreateShare(t.Context(), store.Share{Token: tok, Name: "共有アプリ", AppID: "a", DSL: []byte(dslJSON)}); err != nil {
		t.Fatal(err)
	}
	return st
}

const smallShareDSL = `{"schemaVersion":1,"app":{"name":"共有アプリ","voiceEntry":"none"},"initialScreen":"home","entities":[],
 "screens":[{"id":"home","title":"ホーム","components":[{"type":"text","text":"こんにちは"}]}]}`

// og:url follows SHARE_WEB_BASE_URL, else the request's scheme and host.
func TestSharePageOGURL(t *testing.T) {
	st := shareStore(t, "tok_og", smallShareDSL)
	cases := []struct {
		name, env, host, proto, want string
	}{
		{"env wins, trailing slash dropped", "https://share.example/", "api.example", "http", "https://share.example/s/tok_og"},
		{"forwarded proto", "", "api.example", "http", "http://api.example/s/tok_og"},
		{"https by default", "", "api.example", "", "https://api.example/s/tok_og"},
		{"plain localhost", "", "localhost:8080", "", "http://localhost:8080/s/tok_og"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("SHARE_WEB_BASE_URL", c.env)
			mux := http.NewServeMux()
			mux.Handle("GET /s/{token}", SharePage(st))
			req := httptest.NewRequest(http.MethodGet, "/s/tok_og", nil)
			req.Host = c.host
			if c.proto != "" {
				req.Header.Set("X-Forwarded-Proto", c.proto)
			}
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			if want := `<meta property="og:url" content="` + c.want + `">`; !strings.Contains(rr.Body.String(), want) {
				t.Errorf("missing %s", want)
			}
		})
	}
}

// The invalid-link page names no app: no og tags, no import link.
func TestSharePageInvalidHasNoOG(t *testing.T) {
	rr := getSharePage(t, store.NewMemory(), "tok_missing")
	body := rr.Body.String()
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status %d", rr.Code)
	}
	for _, bad := range []string{"og:title", "og:url", "og:description", "tailorit://"} {
		if strings.Contains(body, bad) {
			t.Errorf("invalid page has %q", bad)
		}
	}
}

// Each response gets its own CSP nonce.
func TestSharePageNoncePerRequest(t *testing.T) {
	st := shareStore(t, "tok_n", smallShareDSL)
	a := getSharePage(t, st, "tok_n").Header().Get("Content-Security-Policy")
	b := getSharePage(t, st, "tok_n").Header().Get("Content-Security-Policy")
	if a == "" || a == b {
		t.Fatalf("nonce reused: %q", a)
	}
}

// A snapshot under the DSL size cap can still render past maxSharePageBytes
// (escaping and repeated options grow it). The page then falls back to the
// name and the call to action instead of failing.
func TestSharePageTooBigFallsBack(t *testing.T) {
	opt := strings.Repeat("<", 40)
	opts := make([]string, maxWebOptions)
	for i := range opts {
		opts[i] = `"` + opt + `"`
	}
	pickers := make([]string, maxWebNodes)
	for i := range pickers {
		pickers[i] = `{"type":"picker","field":"f"}`
	}
	dslJSON := `{"schemaVersion":1,"app":{"name":"大きいアプリ","voiceEntry":"none"},"initialScreen":"home",
	 "entities":[{"id":"e","name":"E","scope":"private","fields":[{"id":"f","name":"` + strings.Repeat("<", 60) + `","type":"enum","options":[` + strings.Join(opts, ",") + `]}]}],
	 "screens":[{"id":"home","title":"h","entity":"e","components":[` + strings.Join(pickers, ",") + `]}]}`
	if len(dslJSON) > 64<<10 {
		t.Fatalf("fixture is %d bytes, over the DSL cap", len(dslJSON))
	}
	if _, err := renderSharePage(func() webPage { p := buildWebPage("x", []byte(dslJSON), nil); p.Lang = "ja"; return p }()); !errors.Is(err, errPageTooBig) {
		t.Fatalf("fixture does not exceed the page cap: %v", err)
	}

	rr := getSharePage(t, shareStore(t, "tok_big", dslJSON), "tok_big")
	body := rr.Body.String()
	if rr.Code != http.StatusOK || len(body) > maxSharePageBytes {
		t.Fatalf("status %d, %d bytes", rr.Code, len(body))
	}
	for _, want := range []string{"<h1>共有アプリ</h1>", "tailorit://import/tok_big", AppStoreURL} {
		if !strings.Contains(body, want) {
			t.Errorf("fallback missing %q", want)
		}
	}
	if strings.Contains(body, `<section class="screen"`) || strings.Contains(body, "<select") {
		t.Errorf("fallback still has screens")
	}
}
