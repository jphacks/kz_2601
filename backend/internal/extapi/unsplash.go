package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Unsplash: 写真を言葉で探す(Unsplash API)。無料・運営のアクセスキー
// (EXT_UNSPLASH_API_KEY・ヘッダー `Authorization: Client-ID`)・OAuth なし
// (1 回 = `runs` 1)。届くのは api.unsplash.com だけ。
//
//	searchPhotos(query, orientation?, limit?)   行: 説明・撮影者・大きさ・写真のページ
//	random(query?, orientation?)                1 件: 同じ形(毎回違う写真なのでキャッシュしない)
//
// 規約(API Terms / Guidelines): 画像は返った URL をそのまま使う(hotlink・複製しない)・
// 撮影者と Unsplash の表示(行の補足 "Photo by <名前> on Unsplash"・リンクに utm を
// 付ける)・ダウンロードした時は download_location を叩く(今は読むだけなので叩かない)。
// 行の形に画像の欄が無いので、画像の URL・撮影者のページ・download_location は
// Output.Cells(サーバーの中だけ)に置く。searchPhotos は 1 時間キャッシュ。
// 回数(demo 1 時間 50 回・本番は承認の後。正典は 1 時間 1,000 回)は人が確かめる(external-api-terms.md)。
const (
	Unsplash = "unsplash"
	// UnsplashAttribution は行と一緒に出すクレジット(撮影者の名前は行の補足にある)。
	UnsplashAttribution = "Unsplash"

	unsplashHost     = "https://api.unsplash.com"
	unsplashMaxQuery = 100
	unsplashMaxBody  = 512 << 10
	unsplashUTM      = "utm_source=tailor_it&utm_medium=referral"
)

var (
	unsplashHosts        = []string{unsplashHost}
	unsplashKeyRe        = regexp.MustCompile(`^[0-9A-Za-z_-]{20,128}$`)
	unsplashOrientations = []string{"landscape", "portrait", "squarish"}
)

func init() {
	register(Provider{ID: Unsplash, Billing: BillingFree, Hosts: unsplashHosts,
		Ops: []OpSpec{
			{Name: "searchPhotos", Args: []string{"query", "orientation", "limit"}, Check: checkUnsplashSearch, CacheTTL: time.Hour},
			{Name: "random", Args: []string{"query", "orientation"}, Check: checkUnsplashRandom},
		},
		New: func(sec Secrets) (map[string]Operation, error) {
			return newUnsplash(sec.APIKey, unsplashHost, NewHTTPClient(unsplashHosts, DefaultTimeout))
		}})
}

// unsplashQuery は query(1 行・1〜100 文字。required なら空は 400)。
func unsplashQuery(a map[string]string, required bool) (string, error) {
	q := strings.TrimSpace(a["query"])
	if q == "" {
		if required {
			return "", fmt.Errorf("%w: query is required", ErrBadArgs)
		}
		return "", nil
	}
	if len([]rune(q)) > unsplashMaxQuery {
		return "", fmt.Errorf("%w: query takes at most %d characters", ErrBadArgs, unsplashMaxQuery)
	}
	if strings.IndexFunc(q, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("%w: query must be one line", ErrBadArgs)
	}
	return q, nil
}

// unsplashOrientation は orientation(無しか landscape / portrait / squarish)。
func unsplashOrientation(a map[string]string) (string, error) {
	o := strings.TrimSpace(a["orientation"])
	if o != "" && !slices.Contains(unsplashOrientations, o) {
		return "", fmt.Errorf("%w: orientation must be one of %s", ErrBadArgs, strings.Join(unsplashOrientations, ", "))
	}
	return o, nil
}

func checkUnsplashSearch(a map[string]string) error {
	if _, err := unsplashQuery(a, true); err != nil {
		return err
	}
	if _, err := unsplashOrientation(a); err != nil {
		return err
	}
	_, err := argInt(a, "limit", 10, 1, 20)
	return err
}

func checkUnsplashRandom(a map[string]string) error {
	if _, err := unsplashQuery(a, false); err != nil {
		return err
	}
	_, err := unsplashOrientation(a)
	return err
}

// newUnsplash は base に対する op(テストは httptest を渡す)。
func newUnsplash(key, base string, hc *http.Client) (map[string]Operation, error) {
	if !unsplashKeyRe.MatchString(key) {
		return nil, errors.New("EXT_UNSPLASH_API_KEY is not set or not an access key")
	}
	u := &unsplash{key: key, base: strings.TrimRight(base, "/"), hc: hc}
	return map[string]Operation{
		"searchPhotos": OperationFunc(u.searchPhotos),
		"random":       OperationFunc(u.random),
	}, nil
}

type unsplash struct {
	key  string
	base string
	hc   *http.Client
}

// unsplashPhoto は API の写真のうち行にする分。
type unsplashPhoto struct {
	ID             string `json:"id"`
	Description    string `json:"description"`
	AltDescription string `json:"alt_description"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	URLs           struct {
		Regular string `json:"regular"`
	} `json:"urls"`
	Links struct {
		HTML             string `json:"html"`
		DownloadLocation string `json:"download_location"`
	} `json:"links"`
	User struct {
		Name  string `json:"name"`
		Links struct {
			HTML string `json:"html"`
		} `json:"links"`
	} `json:"user"`
}

// searchPhotos は /search/photos の 1 ページ目を行にする。
func (u *unsplash) searchPhotos(ctx context.Context, in Input) (Output, error) {
	q, err := unsplashQuery(in.Args, true)
	if err != nil {
		return Output{}, err
	}
	o, err := unsplashOrientation(in.Args)
	if err != nil {
		return Output{}, err
	}
	n, err := argInt(in.Args, "limit", 10, 1, 20)
	if err != nil {
		return Output{}, err
	}
	v := url.Values{"query": {q}, "per_page": {strconv.Itoa(n)}, "page": {"1"}, "content_filter": {"high"}}
	if o != "" {
		v.Set("orientation", o)
	}
	var a struct {
		Results []unsplashPhoto `json:"results"`
	}
	if err := u.get(ctx, "/search/photos?"+v.Encode(), &a); err != nil {
		return Output{}, err
	}
	rows := make([]Row, 0, len(a.Results))
	cells := make([][]string, 0, len(a.Results))
	for _, p := range a.Results {
		row, cell, ok := unsplashRow(p)
		if !ok {
			continue
		}
		rows = append(rows, row)
		cells = append(cells, cell)
		if len(rows) == n {
			break
		}
	}
	return Output{Rows: rows, Cells: cells}, nil
}

// random は /photos/random の 1 枚(item と行 1 つ)。
func (u *unsplash) random(ctx context.Context, in Input) (Output, error) {
	q, err := unsplashQuery(in.Args, false)
	if err != nil {
		return Output{}, err
	}
	o, err := unsplashOrientation(in.Args)
	if err != nil {
		return Output{}, err
	}
	v := url.Values{"content_filter": {"high"}}
	if q != "" {
		v.Set("query", q)
	}
	if o != "" {
		v.Set("orientation", o)
	}
	var p unsplashPhoto
	if err := u.get(ctx, "/photos/random?"+v.Encode(), &p); err != nil {
		return Output{}, err
	}
	row, cell, ok := unsplashRow(p)
	if !ok {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("unsplash answered an unexpected shape")}
	}
	return Output{Item: &row, Rows: []Row{row}, Cells: [][]string{cell}}, nil
}

// unsplashRow は 1 枚を行(見出し = 説明・補足 = "Photo by 撮影者 on Unsplash"・値 = 幅×高さ・
// リンク = 写真のページ)と Cells(id・画像の URL・撮影者のページ・download_location)にする。
// id か画像の URL が無い・https でないものは出さない。
func unsplashRow(p unsplashPhoto) (Row, []string, bool) {
	img := unsplashLink(p.URLs.Regular)
	if p.ID == "" || img == "" {
		return Row{}, nil, false
	}
	title := clip(unsplashLine(p.Description), 80)
	if title == "" {
		title = clip(unsplashLine(p.AltDescription), 80)
	}
	if title == "" {
		title = "写真"
	}
	who := clip(unsplashLine(p.User.Name), 50)
	if who == "" {
		who = "Unsplash"
	}
	row := Row{Title: title, Subtitle: "Photo by " + who + " on Unsplash", URL: unsplashPageLink(p.Links.HTML)}
	if p.Width > 0 && p.Height > 0 {
		row.Value = strconv.Itoa(p.Width) + "×" + strconv.Itoa(p.Height)
	}
	cell := []string{p.ID, img, unsplashPageLink(p.User.Links.HTML), unsplashLink(p.Links.DownloadLocation)}
	return row, cell, true
}

// unsplashLine は空白と改行を 1 つの空白にして 1 行にする。
func unsplashLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// unsplashLink は https の unsplash.com(とその下)だけ通す(user・port 付きは落とす)。
func unsplashLink(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" ||
		(u.Host != "unsplash.com" && !strings.HasSuffix(u.Host, ".unsplash.com")) {
		return ""
	}
	return u.String()
}

// unsplashPageLink は unsplash.com のページのリンクに規約の utm を付ける。
func unsplashPageLink(s string) string {
	l := unsplashLink(s)
	if l == "" {
		return ""
	}
	if strings.Contains(l, "?") {
		return l + "&" + unsplashUTM
	}
	return l + "?" + unsplashUTM
}

// get は 1 回の GET。401 は運営の鍵の問題(502・やり直し不可)、403 の回数超えは 429
// 扱い(やり直し可)、404 は無い写真(400)、429 / 5xx は 502(やり直し可)。
func (u *unsplash) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.base+path, nil)
	if err != nil {
		return errors.New("unsplash: building the request failed")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Version", "v1")
	req.Header.Set("Authorization", "Client-ID "+u.key)
	resp, err := u.hc.Do(req)
	if err != nil {
		return transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := ReadBody(resp.Body)
	if err == nil && len(body) > unsplashMaxBody {
		err = fmt.Errorf("body over %d bytes", unsplashMaxBody)
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &ProviderError{Status: resp.StatusCode, Retryable: true, Err: errors.New("reading the answer failed")}
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return &ProviderError{Status: resp.StatusCode, Err: errors.New("unsplash refused the access key")}
	case resp.StatusCode == http.StatusForbidden && strings.Contains(strings.ToLower(string(body)), "rate limit"):
		return statusError(http.StatusTooManyRequests)
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: unsplash has no such photo", ErrUnknown)
	case resp.StatusCode == http.StatusBadRequest:
		return fmt.Errorf("%w: unsplash refused the query", ErrUnknown)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return statusError(resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return &ProviderError{Status: resp.StatusCode, Err: errors.New("unsplash answered an unexpected shape")}
	}
	return nil
}
