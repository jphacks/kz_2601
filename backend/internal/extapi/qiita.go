package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Qiita: 記事を探す・人の記事を読む(Qiita API v2 https://qiita.com/api/v2/docs)。
// 無料・鍵なし・OAuth なし: 1 回 = `runs` 1。届く先は qiita.com だけ。
//
//	search(q, limit?)           行: 記事ごと(見出し = 題・補足 = 「user id・日付」・値 = いいね・リンク = 記事)   EXT_QIITA_ENABLED=1
//	userItems(userId, limit?)   行: その人の記事(新しい順)。形は search と同じ
//
// q は 1〜100 文字・1 行(Qiita の検索の書き方 `tag:Go` などもそのまま通す)。
// userId は Qiita の id(英数字と _ -・1〜32 文字)。limit は 1〜20(既定 10)。
//
// 回数: 鍵なしは IP ごとに 1 時間 60 回(docs「Rate limit」)。同じ引数の答えは
// 10 分持ち、環境全体の 1 日の既定は qiitaGlobalDaily。1 時間の上限は
// EXT_LIMIT_QIITA_APP_HOURLY などで下げる(limits.go)。429 は 1 分止まる。
// 答えには記事の本文(body・rendered_body)が丸ごと入るので、本文の上限は
// qiitaMaxBody(行に使うのは題・URL・いいね・日付・user id だけ)。
//
// 規約(https://qiita.com/terms・2026-11-14 に読んだ): API 向けの条項は無い。
// 記事の権利は書いた人(8 条)・サービスの転用・売却・再販の禁止(11 条 5 項 3 号)。
// 行は題とリンクだけで本文は返さない。読みの結論は人の判断(external-api-terms.md)。
const Qiita = "qiita"

var qiitaHosts = []string{"https://qiita.com"}

const (
	qiitaMaxBody  = 4 << 20
	qiitaMaxQuery = 100
	qiitaMaxTitle = 120
	qiitaLimitDef = 10
	qiitaLimitMax = 20
	// qiitaGlobalDaily は環境全体の 1 日の既定: 1 時間 60 回 × 24 の 8 割。
	qiitaGlobalDaily = 1150
)

var qiitaUserRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

func init() {
	register(Provider{ID: Qiita, Billing: BillingFree, Hosts: qiitaHosts, GlobalDaily: qiitaGlobalDaily,
		Ops: []OpSpec{
			{Name: "search", Args: []string{"q", "limit"}, Check: checkQiitaSearch, CacheTTL: 10 * time.Minute},
			{Name: "userItems", Args: []string{"userId", "limit"}, Check: checkQiitaUser, CacheTTL: 10 * time.Minute},
		},
		New: func(Secrets) (map[string]Operation, error) {
			return newQiita(qiitaHosts[0], NewHTTPClient(qiitaHosts, DefaultTimeout)), nil
		}})
}

// checkQiitaSearch: q は 1〜100 文字・1 行。limit は 1〜20。
func checkQiitaSearch(a map[string]string) error {
	q := strings.TrimSpace(a["q"])
	if q == "" {
		return errors.New("q is required")
	}
	if len([]rune(q)) > qiitaMaxQuery {
		return fmt.Errorf("q takes at most %d characters", qiitaMaxQuery)
	}
	if strings.IndexFunc(q, unicode.IsControl) >= 0 {
		return errors.New("q must be one line")
	}
	_, err := argInt(a, "limit", qiitaLimitDef, 1, qiitaLimitMax)
	return err
}

// checkQiitaUser: userId は Qiita の id。limit は 1〜20。
func checkQiitaUser(a map[string]string) error {
	if !qiitaUserRe.MatchString(strings.TrimSpace(a["userId"])) {
		return errors.New("userId must be a Qiita user id like qiita")
	}
	_, err := argInt(a, "limit", qiitaLimitDef, 1, qiitaLimitMax)
	return err
}

// newQiita は base に向けた op(テストは httptest のサーバーを渡す)。
func newQiita(base string, hc *http.Client) map[string]Operation {
	q := &qiita{base: strings.TrimRight(base, "/"), hc: hc}
	return map[string]Operation{
		"search":    OperationFunc(q.search),
		"userItems": OperationFunc(q.userItems),
	}
}

type qiita struct {
	base string
	hc   *http.Client
}

// qiitaItem は API の記事(行になる所だけ。本文は読まない)。
type qiitaItem struct {
	Title      string `json:"title"`
	URL        string `json:"url"`
	LikesCount int64  `json:"likes_count"`
	CreatedAt  string `json:"created_at"`
	User       struct {
		ID string `json:"id"`
	} `json:"user"`
}

func (q *qiita) search(ctx context.Context, in Input) (Output, error) {
	if err := checkQiitaSearch(in.Args); err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrBadArgs, err)
	}
	limit, _ := argInt(in.Args, "limit", qiitaLimitDef, 1, qiitaLimitMax)
	v := url.Values{"query": {strings.TrimSpace(in.Args["q"])}, "page": {"1"}, "per_page": {strconv.Itoa(limit)}}
	return q.items(ctx, "/api/v2/items?"+v.Encode(), limit)
}

func (q *qiita) userItems(ctx context.Context, in Input) (Output, error) {
	if err := checkQiitaUser(in.Args); err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrBadArgs, err)
	}
	limit, _ := argInt(in.Args, "limit", qiitaLimitDef, 1, qiitaLimitMax)
	v := url.Values{"page": {"1"}, "per_page": {strconv.Itoa(limit)}}
	return q.items(ctx, "/api/v2/users/"+url.PathEscape(strings.TrimSpace(in.Args["userId"]))+"/items?"+v.Encode(), limit)
}

// items は path を読んで行にする。無い人(404)は 400 bad_request。
func (q *qiita) items(ctx context.Context, path string, limit int) (Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, q.base+path, nil)
	if err != nil {
		return Output{}, errors.New("qiita: building the request failed")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := q.hc.Do(req)
	if err != nil {
		return Output{}, transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return Output{}, fmt.Errorf("%w: qiita has no such user", ErrUnknown)
	case resp.StatusCode != http.StatusOK:
		return Output{}, statusError(resp.StatusCode)
	}
	body, err := ReadBody(io.LimitReader(resp.Body, qiitaMaxBody+1))
	if err == nil && len(body) > qiitaMaxBody {
		err = fmt.Errorf("body over %d bytes", qiitaMaxBody)
	}
	if err != nil {
		if ctx.Err() != nil {
			return Output{}, ctx.Err()
		}
		return Output{}, &ProviderError{Status: resp.StatusCode, Err: err}
	}
	var items []qiitaItem
	if err := json.Unmarshal(body, &items); err != nil {
		return Output{}, &ProviderError{Status: resp.StatusCode, Err: errors.New("qiita answered an unexpected shape")}
	}
	rows := []Row{}
	for _, it := range items {
		title := clip(it.Title, qiitaMaxTitle)
		link := qiitaLink(it.URL)
		if title == "" || link == "" {
			continue
		}
		rows = append(rows, Row{Title: title, Subtitle: joinDots(clip(it.User.ID, 32), qiitaDate(it.CreatedAt)),
			Value: strconv.FormatInt(max(it.LikesCount, 0), 10), URL: link})
		if len(rows) == limit {
			break
		}
	}
	return Output{Rows: rows}, nil
}

// qiitaDate は created_at(RFC 3339)を日本時間の 2006/01/02 に。読めなければ空。
func qiitaDate(s string) string {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return ""
	}
	return t.In(time.FixedZone("JST", 9*3600)).Format("2006/01/02")
}

// qiitaLink は qiita.com の https のリンクだけ通す(クエリは落とす)。
func qiitaLink(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || strings.ToLower(u.Hostname()) != "qiita.com" {
		return ""
	}
	return (&url.URL{Scheme: "https", Host: "qiita.com", Path: u.Path}).String()
}
