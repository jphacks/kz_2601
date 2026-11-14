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
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// connpass: 勉強会・イベントを探す(connpass API v2 https://connpass.com/about/api/v2/)。
// 無料(API 自体の原価は無い)・運営の API キー(`runs` だけ)。届く先は connpass.com だけ。
//
//	events(keyword?, nickname?, prefecture?, ym?, limit?)   行: イベントごと   EXT_CONNPASS_ENABLED=1・EXT_CONNPASS_API_KEY
//
// 行: 見出し = 題・補足 = 「11/20(木) 19:00〜21:00・場所」(日本時間)・値 = 「参加/定員」
// (定員が無ければ参加の数だけ)・リンク = connpass のイベントのページ。
//
// keyword は 1〜100 文字・1 行。nickname は connpass の id(英数字と _ -・32 文字まで:
// その人が参加するイベント)。prefecture は v2 の都道府県の id(小文字のローマ字・
// `tokyo,online` のようにカンマ区切りで 5 つまで)。ym は YYYYMM。ym が無ければ
// 今月(日本時間)の、まだ終わっていないイベントだけを開催の順に出す。limit は 1〜20
// (既定 10)。API には 50 件を頼み(order=2 開催日時順)、こちらで絞って切る。
//
// v1(鍵なし)は 2025-12-31 で終わり、v2 は申請した API キーが要る(X-API-Key)。
// フリープランは 12 秒に 1 回・1 日 100 回(2026-11-04 から)で、「業務目的では
// 使えない」と help にある: 本番は有料のプラン(2026-11-15 オーナー・external-api-terms-modes.md)。
// 同じ引数の答えは 10 分持ち、環境全体の 1 日の既定は connpassGlobalDaily。
// 429 は 1 分止まる(limits.go)。キーはヘッダーだけでエラーに URL は出ない。
const Connpass = "connpass"

var connpassHosts = []string{"https://connpass.com"}

const (
	connpassPath       = "/api/v2/events/"
	connpassMaxBody    = 4 << 20
	connpassMaxKeyword = 100
	connpassMaxTitle   = 120
	connpassMaxPlace   = 60
	connpassLimitDef   = 10
	connpassLimitMax   = 20
	connpassMaxPrefs   = 5
	connpassCount      = 50
	// connpassGlobalDaily は環境全体の 1 日の既定: フリープランの 100 回の 8 割。
	connpassGlobalDaily = 80
	// connpassUserMinute は 1 人の 1 分の既定(ボタンの連打で全体の 1 日を使い切らない)。
	connpassUserMinute = 3
)

var (
	connpassKeyRe  = regexp.MustCompile(`^[!-~]{8,256}$`)
	connpassNickRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
	connpassYmRe   = regexp.MustCompile(`^[0-9]{6}$`)
	// connpassPrefectures は v2 の prefecture の id(都道府県 47 とオンライン)。
	connpassPrefectures = []string{"online", "hokkaido", "aomori", "iwate", "miyagi", "akita", "yamagata", "fukushima",
		"ibaraki", "tochigi", "gunma", "saitama", "chiba", "tokyo", "kanagawa", "niigata", "toyama", "ishikawa", "fukui",
		"yamanashi", "nagano", "gifu", "shizuoka", "aichi", "mie", "shiga", "kyoto", "osaka", "hyogo", "nara", "wakayama",
		"tottori", "shimane", "okayama", "hiroshima", "yamaguchi", "tokushima", "kagawa", "ehime", "kochi", "fukuoka",
		"saga", "nagasaki", "kumamoto", "oita", "miyazaki", "kagoshima", "okinawa"}
	connpassJST = time.FixedZone("JST", 9*3600)
)

func init() {
	register(Provider{ID: Connpass, Billing: BillingFree, Hosts: connpassHosts, GlobalDaily: connpassGlobalDaily,
		// 有料のプランで使う(2026-11-15 オーナー): 鍵が無ければ off のまま(newConnpass)。
		// 第三者向けに出すには connpass の事前の承諾が要る(人の作業)。止まらない使い方を防ぐ控えめの上限:
		// 環境全体 1 日 connpassGlobalDaily・1 人 1 分 connpassUserMinute(env で変えられる)
		UserMinute: connpassUserMinute,
		Terms:      Terms{Attribution: "イベント情報: connpass"},
		Ops: []OpSpec{
			{Name: "events", Args: []string{"keyword", "nickname", "prefecture", "ym", "limit"}, Check: checkConnpassEvents, CacheTTL: 10 * time.Minute},
		},
		New: func(sec Secrets) (map[string]Operation, error) {
			return newConnpass(sec.APIKey, connpassHosts[0], NewHTTPClient(connpassHosts, DefaultTimeout), time.Now)
		}})
}

// checkConnpassEvents は引数の値を確かめる(全部任意)。
func checkConnpassEvents(a map[string]string) error {
	if v, ok := a["keyword"]; ok {
		k := strings.TrimSpace(v)
		if k == "" || len([]rune(k)) > connpassMaxKeyword {
			return fmt.Errorf("keyword must be 1-%d characters", connpassMaxKeyword)
		}
		if strings.IndexFunc(k, unicode.IsControl) >= 0 {
			return errors.New("keyword must be one line")
		}
	}
	if v, ok := a["nickname"]; ok && !connpassNickRe.MatchString(strings.TrimSpace(v)) {
		return errors.New("nickname must be a connpass id")
	}
	if v, ok := a["prefecture"]; ok {
		ps := strings.Split(v, ",")
		if len(ps) > connpassMaxPrefs {
			return fmt.Errorf("prefecture takes at most %d ids", connpassMaxPrefs)
		}
		for i, p := range ps {
			if !slices.Contains(connpassPrefectures, p) || slices.Contains(ps[:i], p) {
				return errors.New("prefecture must be ids like tokyo,online (no repeats)")
			}
		}
	}
	if v, ok := a["ym"]; ok {
		if _, err := time.Parse("200601", v); !connpassYmRe.MatchString(v) || err != nil {
			return errors.New("ym must be YYYYMM")
		}
	}
	_, err := argInt(a, "limit", connpassLimitDef, 1, connpassLimitMax)
	return err
}

// newConnpass は base に向けた op(テストは httptest のサーバーと固定の時計を渡す)。
func newConnpass(key, base string, hc *http.Client, now func() time.Time) (map[string]Operation, error) {
	if !connpassKeyRe.MatchString(key) {
		return nil, errors.New("EXT_CONNPASS_API_KEY is not set or not an API key")
	}
	c := &connpass{key: key, base: strings.TrimRight(base, "/"), hc: hc, now: now}
	return map[string]Operation{"events": OperationFunc(c.events)}, nil
}

type connpass struct {
	key  string
	base string
	hc   *http.Client
	now  func() time.Time
}

// connpassAnswer は API の JSON(行になる所だけ)。
type connpassAnswer struct {
	Events []struct {
		Title     string `json:"title"`
		URL       string `json:"url"`
		StartedAt string `json:"started_at"`
		EndedAt   string `json:"ended_at"`
		Place     string `json:"place"`
		Accepted  int64  `json:"accepted"`
		Limit     *int64 `json:"limit"`
	} `json:"events"`
}

func (c *connpass) events(ctx context.Context, in Input) (Output, error) {
	if err := checkConnpassEvents(in.Args); err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrBadArgs, err)
	}
	limit, _ := argInt(in.Args, "limit", connpassLimitDef, 1, connpassLimitMax)
	now := c.now().In(connpassJST)
	q := url.Values{"order": {"2"}, "count": {strconv.Itoa(connpassCount)}}
	for _, k := range []string{"keyword", "nickname", "prefecture"} {
		if v, ok := in.Args[k]; ok {
			q.Set(k, strings.TrimSpace(v))
		}
	}
	ym, thisMonth := in.Args["ym"]
	if !thisMonth {
		ym = now.Format("200601")
	}
	q.Set("ym", ym)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+connpassPath+"?"+q.Encode(), nil)
	if err != nil {
		return Output{}, errors.New("connpass: building the request failed")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-API-Key", c.key)
	resp, err := c.hc.Do(req)
	if err != nil {
		return Output{}, transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// 401 / 403 はキーの誤り(再試行しない)・429 / 5xx は再試行
		return Output{}, statusError(resp.StatusCode)
	}
	body, err := ReadBody(io.LimitReader(resp.Body, connpassMaxBody+1))
	if err == nil && len(body) > connpassMaxBody {
		err = fmt.Errorf("body over %d bytes", connpassMaxBody)
	}
	if err != nil {
		if ctx.Err() != nil {
			return Output{}, ctx.Err()
		}
		return Output{}, &ProviderError{Status: resp.StatusCode, Err: err}
	}
	var a connpassAnswer
	if err := json.Unmarshal(body, &a); err != nil {
		return Output{}, &ProviderError{Status: resp.StatusCode, Err: errors.New("connpass answered an unexpected shape")}
	}
	type ev struct {
		start time.Time
		row   Row
	}
	var evs []ev
	for _, e := range a.Events {
		title := clip(e.Title, connpassMaxTitle)
		link := connpassLink(e.URL)
		start, err := time.Parse(time.RFC3339, strings.TrimSpace(e.StartedAt))
		if title == "" || link == "" || err != nil {
			continue
		}
		end, errEnd := time.Parse(time.RFC3339, strings.TrimSpace(e.EndedAt))
		if errEnd != nil {
			end = start
		}
		// ym が無い時は今月の、まだ終わっていないものだけ
		if !thisMonth && end.Before(now) {
			continue
		}
		value := strconv.FormatInt(max(e.Accepted, 0), 10)
		if e.Limit != nil && *e.Limit > 0 {
			value += "/" + strconv.FormatInt(*e.Limit, 10)
		}
		evs = append(evs, ev{start: start, row: Row{Title: title, Subtitle: joinDots(connpassWhen(start, end, errEnd == nil), clip(e.Place, connpassMaxPlace)), Value: value, URL: link}})
	}
	slices.SortStableFunc(evs, func(x, y ev) int { return x.start.Compare(y.start) })
	rows := make([]Row, 0, min(len(evs), limit))
	for _, e := range evs[:min(len(evs), limit)] {
		rows = append(rows, e.row)
	}
	return Output{Rows: rows}, nil
}

// connpassWhen は「11/20(木) 19:00〜21:00」(日本時間)。日をまたぐ時は終わりにも日を付ける。
func connpassWhen(start, end time.Time, hasEnd bool) string {
	start, end = start.In(connpassJST), end.In(connpassJST)
	wd := []string{"日", "月", "火", "水", "木", "金", "土"}
	s := start.Format("1/2") + "(" + wd[start.Weekday()] + ") " + start.Format("15:04")
	if !hasEnd || !end.After(start) {
		return s
	}
	if end.Format("20060102") != start.Format("20060102") {
		return s + "〜" + end.Format("1/2 15:04")
	}
	return s + "〜" + end.Format("15:04")
}

// connpassLink は connpass.com(とそのサブドメイン)の https のリンクだけ通す(クエリは落とす)。
func connpassLink(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return ""
	}
	h := strings.ToLower(u.Hostname())
	if h != "connpass.com" && !strings.HasSuffix(h, ".connpass.com") {
		return ""
	}
	return (&url.URL{Scheme: "https", Host: h, Path: u.Path}).String()
}
