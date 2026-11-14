package extapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// YOLP: Yahoo! JAPAN のローカルサーチ(店・施設を言葉で探す)。無料・運営のアプリ ID
// (EXT_YOLP_API_KEY・クエリの appid)・OAuth なし(1 回 = `runs` 1)。届くのは
// map.yahooapis.jp だけ。アプリ ID はクエリに入るので、失敗はキュー(URL)を持たない
// 形で返す(keyed.go)。
//
//	localSearch(query, lat?, lng?, dist?, limit?)   行: 名前・住所・ジャンル
//
// lat / lng を渡すとその点の近い順(dist は km・1〜20・既定 3)。lat / lng は小数
// 4 桁(約 10 m)に丸めてから出す。同じ引数の答えは 10 分キャッシュ。規約: クレジット
// 「Web Services by Yahoo! JAPAN」(YOLPAttribution)を出す。電話番号は行に出さない。
// 商用の可否・回数の上限は人が規約を読む(external-api-terms.md)。
const (
	YOLP = "yolp"
	// YOLPAttribution は行と一緒に出すクレジット(Yahoo! JAPAN のガイドライン)。
	YOLPAttribution = "Web Services by Yahoo! JAPAN"

	yolpHost       = "https://map.yahooapis.jp"
	yolpSearchPath = "/search/local/V1/localSearch"
	yolpMaxQuery   = 100
	yolpCacheTTL   = 10 * time.Minute
)

var (
	yolpHosts = []string{yolpHost}
	yolpKeyRe = regexp.MustCompile(`^[0-9A-Za-z._-]{16,128}$`)
)

func init() {
	register(Provider{ID: YOLP, Billing: BillingFree, Hosts: yolpHosts,
		Ops: []OpSpec{
			{Name: "localSearch", Args: []string{"query", "lat", "lng", "dist", "limit"}, Check: checkYOLPSearch, CacheTTL: yolpCacheTTL},
		},
		New: func(sec Secrets) (map[string]Operation, error) {
			return newYOLP(sec.APIKey, yolpHost, NewHTTPClient(yolpHosts, DefaultTimeout))
		}})
}

// yolpQuery は query(1 行・1〜100 文字)。
func yolpQuery(a map[string]string) (string, error) {
	q := strings.TrimSpace(a["query"])
	if q == "" {
		return "", fmt.Errorf("%w: query is required", ErrBadArgs)
	}
	if len([]rune(q)) > yolpMaxQuery {
		return "", fmt.Errorf("%w: query takes at most %d characters", ErrBadArgs, yolpMaxQuery)
	}
	if strings.IndexFunc(q, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("%w: query must be one line", ErrBadArgs)
	}
	return q, nil
}

// yolpArgs は引数をまとめて確かめる(dist は lat / lng がある時だけ)。
type yolpArgs struct {
	query    string
	lat, lng float64
	point    bool
	dist     int
	limit    int
}

func parseYOLPArgs(a map[string]string) (yolpArgs, error) {
	var p yolpArgs
	var err error
	if p.query, err = yolpQuery(a); err != nil {
		return p, err
	}
	if p.lat, p.lng, p.point, err = argLatLng(a, 4); err != nil {
		return p, err
	}
	if p.dist, err = argInt(a, "dist", 3, 1, 20); err != nil {
		return p, err
	}
	if _, has := a["dist"]; has && !p.point {
		return p, fmt.Errorf("%w: dist needs lat and lng", ErrBadArgs)
	}
	p.limit, err = argInt(a, "limit", 10, 1, 20)
	return p, err
}

func checkYOLPSearch(a map[string]string) error {
	_, err := parseYOLPArgs(a)
	return err
}

// newYOLP は base に対する op(テストは httptest を渡す)。
func newYOLP(key, base string, hc *http.Client) (map[string]Operation, error) {
	if !yolpKeyRe.MatchString(key) {
		return nil, errors.New("EXT_YOLP_API_KEY is not set or not an application id")
	}
	y := &yolp{key: key, base: strings.TrimRight(base, "/"), hc: hc}
	return map[string]Operation{"localSearch": OperationFunc(y.localSearch)}, nil
}

type yolp struct {
	key  string
	base string
	hc   *http.Client
}

// yolpAnswer は ローカルサーチの JSON のうち行にする分。
type yolpAnswer struct {
	ResultInfo struct {
		Count int `json:"Count"`
		Total int `json:"Total"`
	} `json:"ResultInfo"`
	Feature []struct {
		Name     string `json:"Name"`
		Geometry struct {
			Coordinates string `json:"Coordinates"` // "lon,lat"
		} `json:"Geometry"`
		Property struct {
			Address string `json:"Address"`
			Genre   []struct {
				Name string `json:"Name"`
			} `json:"Genre"`
		} `json:"Property"`
	} `json:"Feature"`
	Error *struct {
		Message string `json:"Message"`
	} `json:"Error"`
}

// localSearch は 1 回の GET。行 = 見出し: 名前・補足: 住所・値: ジャンル。
// Cells(サーバーの中だけ)は 名前・住所・ジャンル・緯度・経度。
func (y *yolp) localSearch(ctx context.Context, in Input) (Output, error) {
	p, err := parseYOLPArgs(in.Args)
	if err != nil {
		return Output{}, err
	}
	q := url.Values{"appid": {y.key}, "query": {p.query}, "output": {"json"}, "results": {strconv.Itoa(p.limit)}}
	if p.point {
		q.Set("lat", strconv.FormatFloat(p.lat, 'f', 4, 64))
		q.Set("lon", strconv.FormatFloat(p.lng, 'f', 4, 64))
		q.Set("dist", strconv.Itoa(p.dist))
		q.Set("sort", "dist")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, y.base+yolpSearchPath+"?"+q.Encode(), nil)
	if err != nil {
		return Output{}, errors.New("yolp: building the request failed")
	}
	req.Header.Set("Accept", "application/json")
	var a yolpAnswer
	if err := y.send(ctx, req, &a); err != nil {
		return Output{}, err
	}
	if a.Error != nil {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("yolp answered an error")}
	}
	rows := make([]Row, 0, len(a.Feature))
	cells := make([][]string, 0, len(a.Feature))
	for _, f := range a.Feature {
		name := clip(f.Name, 80)
		if name == "" {
			continue
		}
		genre := ""
		if len(f.Property.Genre) > 0 {
			genre = clip(f.Property.Genre[0].Name, 30)
		}
		addr := clip(f.Property.Address, 100)
		lat, lng := yolpPoint(f.Geometry.Coordinates)
		rows = append(rows, Row{Title: name, Subtitle: addr, Value: genre})
		cells = append(cells, []string{name, addr, genre, lat, lng})
		if len(rows) == p.limit {
			break
		}
	}
	return Output{Rows: rows, Cells: cells}, nil
}

// yolpPoint は "lon,lat" を緯度・経度の文字に分ける(読めなければ空)。
func yolpPoint(s string) (lat, lng string) {
	lon, la, ok := strings.Cut(strings.TrimSpace(s), ",")
	if !ok {
		return "", ""
	}
	if _, err := strconv.ParseFloat(la, 64); err != nil {
		return "", ""
	}
	if _, err := strconv.ParseFloat(lon, 64); err != nil {
		return "", ""
	}
	return la, lon
}

// send は 1 回の要求。400 は引数の問題(400)、401 / 403 は運営のアプリ ID の問題(502・
// やり直し不可)、429 / 5xx は 502(やり直し可)。失敗に URL は載せない(appid が入る)。
func (y *yolp) send(ctx context.Context, req *http.Request, out any) error {
	err := sendJSON(ctx, y.hc, req, out)
	var pe *ProviderError
	if !errors.As(err, &pe) {
		return err
	}
	switch pe.Status {
	case http.StatusBadRequest:
		return fmt.Errorf("%w: yolp refused the query", ErrUnknown)
	case http.StatusUnauthorized, http.StatusForbidden:
		return &ProviderError{Status: pe.Status, Err: errors.New("yolp refused the application id")}
	}
	return err
}
