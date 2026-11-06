package appconfig

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

func getSharePageLang(t *testing.T, st store.Store, tok, accept string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("GET /s/{token}", SharePage(st))
	req := httptest.NewRequest(http.MethodGet, "/s/"+tok, nil)
	if accept != "" {
		req.Header.Set("Accept-Language", accept)
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

func TestShareLang(t *testing.T) {
	for accept, want := range map[string]string{
		"":                           "ja",
		"ja-JP":                      "ja",
		"ja,en-US;q=0.9":             "ja",
		"en-US,en;q=0.9":             "en",
		"en-GB;q=0.8, ja;q=0.5":      "en",
		"fr-FR,fr;q=0.9,ja;q=0.8":    "ja",
		"zh-Hant-TW":                 "en",
		"*":                          "en",
		" EN ":                       "en",
		"jav":                        "en", // Javanese is not ja
		"ja-Latn-JP-x-private;q=0.1": "ja",
	} {
		if got := shareLang(accept); got != want {
			t.Errorf("shareLang(%q) = %q, want %q", accept, got, want)
		}
	}
}

// Every {{t "key"}} in the template has wording in every language, and the
// languages have the same keys (#278).
func TestSharePageTextKeys(t *testing.T) {
	keysOf := func(m map[string]string) []string {
		var ks []string
		for k := range m {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		return ks
	}
	ja := keysOf(sharePageText["ja"])
	if got := keysOf(sharePageText["en"]); strings.Join(got, ",") != strings.Join(ja, ",") {
		t.Errorf("en keys %v != ja keys %v", got, ja)
	}
	for _, m := range regexp.MustCompile(`t "([A-Za-z0-9]+)"`).FindAllStringSubmatch(sharePageSrc, -1) {
		for lang, text := range sharePageText {
			if text[m[1]] == "" {
				t.Errorf("%s: no wording for %q", lang, m[1])
			}
		}
	}
}

// #278: someone without the app gets the App Store link and the steps to come
// back to this page after installing; the open button is the same import URL.
func TestSharePageInstallSteps(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/dsl/examples/kakeibo.json")
	if err != nil {
		t.Fatal(err)
	}
	st := store.NewMemory()
	if err := st.CreateShare(t.Context(), store.Share{Token: "tok_i", Name: "家計簿", AppID: "a", DSL: raw}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		accept, lang string
		want         []string
		not          []string
	}{
		{"ja-JP,ja;q=0.9", "ja",
			[]string{"Tailor It をまだ入れていない場合", "このページに戻って、もう一度「アプリで開く」を押すと参加できます", ">アプリで開く</a>", ">App Store で入手</a>"},
			[]string{"Open in the app"}},
		{"en-US,en;q=0.9", "en",
			[]string{"Don&#39;t have Tailor It yet?", "Come back to this page and tap &#34;Open in the app&#34; again to join", ">Open in the app</a>", ">Get it on the App Store</a>", "An app was shared with you"},
			[]string{"アプリで開く", "App Store で入手", "記録はアプリで見られます", "アプリが共有されました"}},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			rr := getSharePageLang(t, st, "tok_i", tc.accept)
			if rr.Code != http.StatusOK {
				t.Fatalf("status %d", rr.Code)
			}
			body := rr.Body.String()
			want := append([]string{
				`<html lang="` + tc.lang + `">`,
				`<a class="btn" href="` + AppStoreURL + `" rel="noopener">`,
				`<a class="btn primary" href="tailorit://import/tok_i">`,
				`<section class="install" aria-labelledby="install-h">`,
			}, tc.want...)
			for _, w := range want {
				if !strings.Contains(body, w) {
					t.Errorf("missing %q", w)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(body, n) {
					t.Errorf("unexpected %q", n)
				}
			}
			if strings.Count(body, "<li>") < 3 {
				t.Errorf("install steps are not a 3-item list")
			}
			if v := rr.Header().Get("Vary"); v != "Accept-Language" {
				t.Errorf("Vary %q", v)
			}
		})
	}
}

// An invalid link keeps the App Store link (in the viewer's language) and has
// no install steps: there is nothing to join.
func TestSharePageInvalidLang(t *testing.T) {
	st := store.NewMemory()
	for accept, want := range map[string]string{"ja": "App Store で Tailor It を見る", "en": "See Tailor It on the App Store"} {
		rr := getSharePageLang(t, st, "tok_none", accept)
		body := rr.Body.String()
		if rr.Code != http.StatusNotFound || !strings.Contains(body, want) || !strings.Contains(body, `href="`+AppStoreURL+`"`) {
			t.Errorf("%s: status %d, body lacks %q or the store link", accept, rr.Code, want)
		}
		if strings.Contains(body, "install-h") {
			t.Errorf("%s: install steps on an invalid link", accept)
		}
	}
}

// renderSharePage without a language (goldens, fallback) is the Japanese page.
func TestRenderSharePageDefaultLang(t *testing.T) {
	got, err := renderSharePage(webPage{StoreURL: AppStoreURL, Nonce: "n"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `<html lang="ja">`) || !strings.Contains(string(got), "リンクが無効です") {
		t.Errorf("default page is not Japanese")
	}
}
