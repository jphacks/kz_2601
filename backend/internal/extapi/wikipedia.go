package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
)

// Wikipedia: 記事の要約と検索(Wikimedia REST API・MediaWiki API)。無料・鍵なし・
// OAuth なし(1 回 = `runs` 1)。届くのは ja / en の wikipedia.org だけ。
//
//	summary(title, lang?)      1 件: 見出し = 題名・補足 = 説明・値 = 本文の先頭 300 文字・リンク
//	search(q, lang?, limit?)   行: 題名・説明・リンク(opensearch)
//
// lang は ja(既定)か en。同じ引数の答えは 1 時間キャッシュ。Wikimedia の
// User-Agent の決まりに従い、連絡先入りの UA を付ける。本文は CC BY-SA 4.0 なので、
// アプリは WikipediaAttribution を行と一緒に出す。
const (
	Wikipedia = "wikipedia"
	// WikipediaAttribution は行と一緒に出す出典(CC BY-SA 4.0)。
	WikipediaAttribution = "Wikipedia(CC BY-SA 4.0)"

	wikipediaUserAgent  = "TailorIt/1.0 (https://tailor-it.app)"
	wikipediaMaxTitle   = 200
	wikipediaMaxExtract = 300
	wikipediaMaxBody    = 256 << 10
)

// wikipediaLangs は使える言語(= 出口の host)。
var wikipediaLangs = []string{"ja", "en"}

var wikipediaHosts = []string{"https://ja.wikipedia.org", "https://en.wikipedia.org"}

func init() {
	register(Provider{ID: Wikipedia, Billing: BillingFree, Hosts: wikipediaHosts,
		Ops: []OpSpec{
			{Name: "summary", Args: []string{"title", "lang"}, Check: checkWikipediaSummary, CacheTTL: time.Hour},
			{Name: "search", Args: []string{"q", "lang", "limit"}, Check: checkWikipediaSearch, CacheTTL: time.Hour},
		},
		New: func(Secrets) (map[string]Operation, error) {
			bases := map[string]string{}
			for _, l := range wikipediaLangs {
				bases[l] = "https://" + l + ".wikipedia.org"
			}
			return newWikipedia(bases, NewHTTPClient(wikipediaHosts, DefaultTimeout)), nil
		}})
}

// wikipediaLang は lang(既定 ja)。
func wikipediaLang(a map[string]string) (string, error) {
	l := strings.TrimSpace(a["lang"])
	if l == "" {
		return "ja", nil
	}
	if !slices.Contains(wikipediaLangs, l) {
		return "", fmt.Errorf("%w: lang must be one of %s", ErrBadArgs, strings.Join(wikipediaLangs, ", "))
	}
	return l, nil
}

// wikipediaText は 1 行・1〜max 文字の引数。
func wikipediaText(a map[string]string, name string, max int) (string, error) {
	t := strings.TrimSpace(a[name])
	if t == "" {
		return "", fmt.Errorf("%w: %s is required", ErrBadArgs, name)
	}
	if len([]rune(t)) > max {
		return "", fmt.Errorf("%w: %s takes at most %d characters", ErrBadArgs, name, max)
	}
	if strings.IndexFunc(t, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("%w: %s must be one line", ErrBadArgs, name)
	}
	if t == "." || t == ".." {
		// MediaWiki に無い題名。パスの区切りとして読まれないよう先に断る
		return "", fmt.Errorf("%w: %s is not a page title", ErrBadArgs, name)
	}
	return t, nil
}

func checkWikipediaSummary(a map[string]string) error {
	if _, err := wikipediaText(a, "title", wikipediaMaxTitle); err != nil {
		return err
	}
	_, err := wikipediaLang(a)
	return err
}

func checkWikipediaSearch(a map[string]string) error {
	if _, err := wikipediaText(a, "q", wikipediaMaxTitle); err != nil {
		return err
	}
	if _, err := wikipediaLang(a); err != nil {
		return err
	}
	_, err := argInt(a, "limit", 10, 1, 20)
	return err
}

// newWikipedia は言語ごとの base に対する op(テストは httptest を渡す)。
func newWikipedia(bases map[string]string, hc *http.Client) map[string]Operation {
	w := &wikipedia{bases: bases, hc: hc}
	return map[string]Operation{
		"summary": OperationFunc(w.summary),
		"search":  OperationFunc(w.search),
	}
}

type wikipedia struct {
	bases map[string]string
	hc    *http.Client
}

// wikipediaSummary は REST の page/summary のうち行にする分。
type wikipediaSummary struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Extract     string `json:"extract"`
	ContentURLs struct {
		Desktop struct {
			Page string `json:"page"`
		} `json:"desktop"`
	} `json:"content_urls"`
}

// summary は記事 1 件(無い題名は 400)。
func (w *wikipedia) summary(ctx context.Context, in Input) (Output, error) {
	title, err := wikipediaText(in.Args, "title", wikipediaMaxTitle)
	if err != nil {
		return Output{}, err
	}
	lang, err := wikipediaLang(in.Args)
	if err != nil {
		return Output{}, err
	}
	u := w.bases[lang] + "/api/rest_v1/page/summary/" + url.PathEscape(strings.ReplaceAll(title, " ", "_"))
	var s wikipediaSummary
	if err := w.get(ctx, u, &s); err != nil {
		return Output{}, err
	}
	if s.Title == "" {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("wikipedia answered an unexpected shape")}
	}
	row := Row{Title: s.Title, Subtitle: s.Description, Value: wikipediaCut(s.Extract, wikipediaMaxExtract)}
	if w.pageURL(lang, s.ContentURLs.Desktop.Page) {
		row.URL = s.ContentURLs.Desktop.Page
	}
	return Output{Item: &row, Rows: []Row{row}}, nil
}

// search は opensearch の答え([q, [題名], [説明], [URL]])を行にする。
func (w *wikipedia) search(ctx context.Context, in Input) (Output, error) {
	q, err := wikipediaText(in.Args, "q", wikipediaMaxTitle)
	if err != nil {
		return Output{}, err
	}
	lang, err := wikipediaLang(in.Args)
	if err != nil {
		return Output{}, err
	}
	n, err := argInt(in.Args, "limit", 10, 1, 20)
	if err != nil {
		return Output{}, err
	}
	v := url.Values{"action": {"opensearch"}, "search": {q}, "limit": {fmt.Sprint(n)}, "namespace": {"0"}, "format": {"json"}}
	var raw []json.RawMessage
	if err := w.get(ctx, w.bases[lang]+"/w/api.php?"+v.Encode(), &raw); err != nil {
		return Output{}, err
	}
	var titles, descs, urls []string
	if len(raw) < 4 || json.Unmarshal(raw[1], &titles) != nil || json.Unmarshal(raw[2], &descs) != nil || json.Unmarshal(raw[3], &urls) != nil {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("wikipedia answered an unexpected shape")}
	}
	rows := make([]Row, 0, len(titles))
	for i, t := range titles {
		if t == "" {
			continue
		}
		r := Row{Title: t}
		if i < len(descs) {
			r.Subtitle = wikipediaCut(descs[i], wikipediaMaxExtract)
		}
		if i < len(urls) && w.pageURL(lang, urls[i]) {
			r.URL = urls[i]
		}
		rows = append(rows, r)
		if len(rows) == n {
			break
		}
	}
	return Output{Rows: rows}, nil
}

// pageURL は同じ言語の wikipedia.org のページだけ通す。
func (w *wikipedia) pageURL(lang, u string) bool {
	return u != "" && strings.HasPrefix(u, w.bases[lang]+"/")
}

// wikipediaCut は先頭 n 文字(文字単位)。
func wikipediaCut(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// get は 1 回の GET。404 は無いページ(400)、429 / 5xx は 502(やり直し可)。
func (w *wikipedia) get(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", wikipediaUserAgent)
	req.Header.Set("Api-User-Agent", wikipediaUserAgent)
	resp, err := w.hc.Do(req)
	if err != nil {
		return transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: wikipedia has no such page", ErrUnknown)
	case resp.StatusCode == http.StatusBadRequest:
		return fmt.Errorf("%w: wikipedia refused the title", ErrUnknown)
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return &ProviderError{Status: resp.StatusCode, Retryable: true, Err: errors.New("wikipedia unavailable")}
	case resp.StatusCode != http.StatusOK:
		return &ProviderError{Status: resp.StatusCode, Err: errors.New("wikipedia refused")}
	}
	body, err := ReadBody(resp.Body)
	if err == nil && len(body) > wikipediaMaxBody {
		err = fmt.Errorf("body over %d bytes", wikipediaMaxBody)
	}
	if err != nil {
		return &ProviderError{Status: resp.StatusCode, Err: err}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return &ProviderError{Status: resp.StatusCode, Err: errors.New("wikipedia answered an unexpected shape")}
	}
	return nil
}
