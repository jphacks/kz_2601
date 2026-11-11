package appconfig

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func getAccountDeletePage(t *testing.T, accept string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	MountAccountDeletePage(mux)
	req := httptest.NewRequest(http.MethodGet, AccountDeletePath, nil)
	if accept != "" {
		req.Header.Set("Accept-Language", accept)
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

func TestAccountDeletePageLanguages(t *testing.T) {
	const jaSubject = "%E3%82%A2%E3%82%AB%E3%82%A6%E3%83%B3%E3%83%88%E5%89%8A%E9%99%A4%E3%81%AE%E4%BE%9D%E9%A0%BC"
	cases := []struct {
		accept, lang, title, subject string
	}{
		{"", "ja", "アカウントの削除", jaSubject},
		{"ja-JP,ja;q=0.9", "ja", "アカウントの削除", jaSubject},
		{"en-US,en;q=0.9", "en", "Delete your account", "Account%20deletion%20request"},
		{"fr-FR", "en", "Delete your account", "Account%20deletion%20request"},
	}
	for _, c := range cases {
		t.Run(c.accept, func(t *testing.T) {
			rr := getAccountDeletePage(t, c.accept)
			if rr.Code != http.StatusOK {
				t.Fatalf("status %d", rr.Code)
			}
			body := rr.Body.String()
			for _, want := range []string{
				`<html lang="` + c.lang + `">`,
				"<title>" + c.title + " - Tailor It</title>",
				`href="mailto:` + SupportEmail + `?subject=` + c.subject + `"`,
			} {
				if !strings.Contains(body, want) {
					t.Errorf("missing %q", want)
				}
			}
			if strings.Contains(body, "<script") {
				t.Error("page must not run scripts")
			}
			h := rr.Header()
			if got := h.Get("Content-Type"); got != "text/html; charset=utf-8" {
				t.Errorf("Content-Type %q", got)
			}
			if csp := h.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") || !strings.Contains(body, `<style nonce="`) {
				t.Errorf("CSP %q", csp)
			}
			if h.Get("Vary") != "Accept-Language" {
				t.Error("missing Vary: Accept-Language")
			}
		})
	}
}

// Every language has every key (in-app steps, email, what goes, what stays,
// cannot be undone).
func TestAccountDeleteTextKeys(t *testing.T) {
	ja := accountDeleteText["ja"]
	for lang, text := range accountDeleteText {
		if len(text) != len(ja) {
			t.Errorf("%s: %d keys, ja has %d", lang, len(text), len(ja))
		}
		for k, v := range ja {
			if strings.TrimSpace(text[k]) == "" {
				t.Errorf("%s: missing %q (ja: %s)", lang, k, v)
			}
		}
	}
}

func TestAccountDeletePageGetOnly(t *testing.T) {
	mux := http.NewServeMux()
	MountAccountDeletePage(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, AccountDeletePath, nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: status %d", rr.Code)
	}
}
