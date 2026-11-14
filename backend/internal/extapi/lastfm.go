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
)

// Last.fm: 人の最近聴いた曲・よく聴くアーティスト(Last.fm API 2.0 https://www.last.fm/api)。
// 無料・運営の鍵(EXT_LASTFM_API_KEY・クエリの api_key)・OAuth なし: 1 回 = `runs` 1。
// 利用者の名前は op の引数で、公開のデータを読むだけ(本人の認証は要らない)。
// 届く先は ws.audioscrobbler.com だけ。
//
//	recentTracks(user, limit?)          行: 曲ごと(見出し = 曲・補足 = アーティスト・アルバム・値 = 「再生中」か日時(日本時間)・リンク = 曲)   EXT_LASTFM_ENABLED=1
//	topArtists(user, period?, limit?)   行: アーティストごと(見出し = 名前・補足 = 順位・値 = 再生回数・リンク = アーティスト)
//
// user は Last.fm の名前(英字で始まる英数字と _ -・2〜15 文字)。limit は 1〜20(既定 10)。
// period は overall(既定)・7day・1month・3month・6month・12month。
//
// Last.fm は 1 曲だけの時に配列でなく 1 個のオブジェクトを返すことがあるので、
// 配列でも 1 個でも読む(lastfmList)。エラーは HTTP の状態と本文の error で分ける:
// 無い人(404・error 6)は 400 bad_request、鍵の不備(403・error 10 / 26)は 502、
// 回数超え(429・error 29)は 1 分止まる。
//
// 規約(https://www.last.fm/api/tos): 非商用の利用が前提で、商用は別の許可が要る。
// 出典の表示(LastfmAttribution)。入れるか・規約の読みは人の判断(external-api-terms.md)。既定 off。
const (
	Lastfm = "lastfm"
	// LastfmAttribution は行の下に出す出典。
	LastfmAttribution = "Powered by Last.fm"
)

var lastfmHosts = []string{"https://ws.audioscrobbler.com"}

const (
	lastfmLimitDef   = 10
	lastfmLimitMax   = 20
	lastfmMaxTitle   = 120
	lastfmPeriodDef  = "overall"
	lastfmTTLRecent  = time.Minute
	lastfmTTLArtists = time.Hour
)

var (
	lastfmUserRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{1,14}$`)
	lastfmKeyRe  = regexp.MustCompile(`^[a-f0-9]{32}$`)
	lastfmPeriod = []string{"overall", "7day", "1month", "3month", "6month", "12month"}
)

func init() {
	register(Provider{ID: Lastfm, Billing: BillingFree, Hosts: lastfmHosts,
		Ops: []OpSpec{
			{Name: "recentTracks", Args: []string{"user", "limit"}, Check: checkLastfmRecent, CacheTTL: lastfmTTLRecent},
			{Name: "topArtists", Args: []string{"user", "period", "limit"}, Check: checkLastfmTop, CacheTTL: lastfmTTLArtists},
		},
		New: func(sec Secrets) (map[string]Operation, error) {
			return newLastfm(sec.APIKey, lastfmHosts[0], NewHTTPClient(lastfmHosts, DefaultTimeout))
		}})
}

func checkLastfmUser(a map[string]string) error {
	if !lastfmUserRe.MatchString(strings.TrimSpace(a["user"])) {
		return errors.New("user must be a Last.fm user name like rj")
	}
	_, err := argInt(a, "limit", lastfmLimitDef, 1, lastfmLimitMax)
	return err
}

func checkLastfmRecent(a map[string]string) error { return checkLastfmUser(a) }

// checkLastfmTop: period は決まった値だけ。
func checkLastfmTop(a map[string]string) error {
	if err := checkLastfmUser(a); err != nil {
		return err
	}
	if p := strings.TrimSpace(a["period"]); p != "" && !slices.Contains(lastfmPeriod, p) {
		return errors.New("period must be one of " + strings.Join(lastfmPeriod, ", "))
	}
	return nil
}

// newLastfm は base に向けた op(テストは httptest のサーバーを渡す)。鍵が無ければ provider は動かない。
func newLastfm(key, base string, hc *http.Client) (map[string]Operation, error) {
	if !lastfmKeyRe.MatchString(key) {
		return nil, errors.New("EXT_LASTFM_API_KEY is not set or not a key")
	}
	l := &lastfm{key: key, base: strings.TrimRight(base, "/"), hc: hc}
	return map[string]Operation{
		"recentTracks": OperationFunc(l.recentTracks),
		"topArtists":   OperationFunc(l.topArtists),
	}, nil
}

type lastfm struct {
	key  string
	base string
	hc   *http.Client
}

// lastfmText は {"#text": "..."} の形。
type lastfmText struct {
	Text string `json:"#text"`
}

type lastfmTrack struct {
	Name   string     `json:"name"`
	URL    string     `json:"url"`
	Artist lastfmText `json:"artist"`
	Album  lastfmText `json:"album"`
	Date   struct {
		UTS string `json:"uts"`
	} `json:"date"`
	Attr struct {
		NowPlaying string `json:"nowplaying"`
	} `json:"@attr"`
}

type lastfmArtist struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	PlayCount string `json:"playcount"`
	Attr      struct {
		Rank string `json:"rank"`
	} `json:"@attr"`
}

// lastfmList は配列でも 1 個でも読む。
type lastfmList[T any] []T

func (l *lastfmList[T]) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '[' {
		return json.Unmarshal(b, (*[]T)(l))
	}
	var one T
	if err := json.Unmarshal(b, &one); err != nil {
		return err
	}
	*l = lastfmList[T]{one}
	return nil
}

// lastfmAnswer は API の答え(行になる所だけ)。
type lastfmAnswer struct {
	Error        int    `json:"error"`
	Message      string `json:"message"`
	RecentTracks struct {
		Track lastfmList[lastfmTrack] `json:"track"`
	} `json:"recenttracks"`
	TopArtists struct {
		Artist lastfmList[lastfmArtist] `json:"artist"`
	} `json:"topartists"`
}

func (l *lastfm) recentTracks(ctx context.Context, in Input) (Output, error) {
	if err := checkLastfmRecent(in.Args); err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrBadArgs, err)
	}
	limit, _ := argInt(in.Args, "limit", lastfmLimitDef, 1, lastfmLimitMax)
	a, err := l.get(ctx, url.Values{"method": {"user.getrecenttracks"}, "user": {strings.TrimSpace(in.Args["user"])}, "limit": {strconv.Itoa(limit)}})
	if err != nil {
		return Output{}, err
	}
	rows := []Row{}
	for _, t := range a.RecentTracks.Track {
		title := clip(t.Name, lastfmMaxTitle)
		if title == "" {
			continue
		}
		value := lastfmTime(t.Date.UTS)
		if t.Attr.NowPlaying == "true" {
			value = "再生中"
		}
		rows = append(rows, Row{Title: title, Subtitle: joinDots(clip(t.Artist.Text, 80), clip(t.Album.Text, 80)), Value: value, URL: lastfmLink(t.URL)})
		if len(rows) == limit {
			break
		}
	}
	return Output{Rows: rows}, nil
}

func (l *lastfm) topArtists(ctx context.Context, in Input) (Output, error) {
	if err := checkLastfmTop(in.Args); err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrBadArgs, err)
	}
	limit, _ := argInt(in.Args, "limit", lastfmLimitDef, 1, lastfmLimitMax)
	period := strings.TrimSpace(in.Args["period"])
	if period == "" {
		period = lastfmPeriodDef
	}
	a, err := l.get(ctx, url.Values{"method": {"user.gettopartists"}, "user": {strings.TrimSpace(in.Args["user"])}, "period": {period}, "limit": {strconv.Itoa(limit)}})
	if err != nil {
		return Output{}, err
	}
	rows := []Row{}
	for _, ar := range a.TopArtists.Artist {
		title := clip(ar.Name, lastfmMaxTitle)
		if title == "" {
			continue
		}
		sub := ""
		if n, err := strconv.Atoi(ar.Attr.Rank); err == nil && n > 0 {
			sub = "#" + strconv.Itoa(n)
		}
		value := ""
		if n, err := strconv.ParseInt(ar.PlayCount, 10, 64); err == nil && n >= 0 {
			value = strconv.FormatInt(n, 10) + " 回"
		}
		rows = append(rows, Row{Title: title, Subtitle: sub, Value: value, URL: lastfmLink(ar.URL)})
		if len(rows) == limit {
			break
		}
	}
	return Output{Rows: rows}, nil
}

// get は /2.0/ を読む。鍵はクエリ(API の決まり)で、エラーに URL は載せない(keyed.go)。
// 本文の error は HTTP の状態より優先して分ける。
func (l *lastfm) get(ctx context.Context, q url.Values) (*lastfmAnswer, error) {
	q.Set("api_key", l.key)
	q.Set("format", "json")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.base+"/2.0/?"+q.Encode(), nil)
	if err != nil {
		return nil, errors.New("lastfm: building the request failed")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := l.hc.Do(req)
	if err != nil {
		return nil, transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := ReadBody(resp.Body)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &ProviderError{Status: resp.StatusCode, Retryable: true, Err: errors.New("reading the answer failed")}
	}
	var a lastfmAnswer
	if jsonErr := json.Unmarshal(body, &a); jsonErr != nil {
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, statusError(resp.StatusCode)
		}
		return nil, &ProviderError{Status: resp.StatusCode, Err: errors.New("lastfm answered an unexpected shape")}
	}
	switch {
	case a.Error == 6 || resp.StatusCode == http.StatusNotFound:
		// Invalid parameters(無い人)
		return nil, fmt.Errorf("%w: lastfm has no such user", ErrUnknown)
	case a.Error == 29:
		// Rate limit exceeded
		return nil, statusError(http.StatusTooManyRequests)
	case a.Error != 0:
		st := resp.StatusCode
		if st >= 200 && st <= 299 {
			st = http.StatusBadGateway
		}
		return nil, &ProviderError{Status: st, Retryable: st == http.StatusTooManyRequests || st >= 500, Err: fmt.Errorf("lastfm error %d", a.Error)}
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return nil, statusError(resp.StatusCode)
	}
	return &a, nil
}

// lastfmTime は uts(秒)を日本時間の 2006/01/02 15:04 に。読めなければ空。
func lastfmTime(uts string) string {
	n, err := strconv.ParseInt(strings.TrimSpace(uts), 10, 64)
	if err != nil || n <= 0 {
		return ""
	}
	return time.Unix(n, 0).In(time.FixedZone("JST", 9*3600)).Format("2006/01/02 15:04")
}

// lastfmLink は last.fm の https のリンクだけ通す(クエリは落とす)。
func lastfmLink(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return ""
	}
	h := strings.ToLower(u.Hostname())
	if h != "www.last.fm" && h != "last.fm" {
		return ""
	}
	return (&url.URL{Scheme: "https", Host: "www.last.fm", Path: u.Path}).String()
}
