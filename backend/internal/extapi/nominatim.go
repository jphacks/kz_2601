package extapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Nominatim: 地名や住所から緯度経度を引く・緯度経度から住所を引く
// (OpenStreetMap Nominatim https://nominatim.org/release-docs/latest/api/)。
// 無料・鍵なし・OAuth なし: 1 回 = `runs` 1。届く先は nominatim.openstreetmap.org だけ。
//
//	geocode(q, limit?)   行: 候補ごと(見出し = display_name・値 = "lat,lng"・補足 = type・category・リンク = osm.org の地図)   EXT_NOMINATIM_ENABLED=1
//	reverse(lat, lng)    1 件(形は geocode と同じ)。その場所に住所が無ければ 400 bad_request
//
// q は 1〜200 文字・1 行。limit は 1〜10(既定 5)。lat / lng は小数の度(4 桁 ≈ 10 m に丸める)。
//
// 利用方針(https://operations.osmfoundation.org/policies/nominatim/): 1 秒 1 回まで・
// アプリを名乗る User-Agent(nominatimUserAgent)・出典の表示(NominatimAttribution)・
// 大量の一括の問い合わせ(bulk geocoding)の禁止。1 秒 1 回は台の中で間を空けて守り
// (nominatimPace)、環境全体の 1 日の既定は nominatimGlobalDaily に控える。
// 同じ引数の答えは 1 日持つ(nominatimCacheTTL)。データは ODbL(© OpenStreetMap contributors)。
// 住所の検索は Google Places(有料・緯度経度 30 日)と別物で、ここは鍵も原価もない。
const (
	Nominatim = "nominatim"
	// NominatimAttribution は行の下に出す出典(ODbL の決まり)。
	NominatimAttribution = "© OpenStreetMap contributors"
)

var nominatimHosts = []string{"https://nominatim.openstreetmap.org"}

const (
	nominatimUserAgent = "TailorIt/1.0 (https://tailor-it.app)"
	nominatimMaxQuery  = 200
	nominatimMaxTitle  = 200
	nominatimLimitDef  = 5
	nominatimLimitMax  = 10
	nominatimCacheTTL  = 24 * time.Hour
	// nominatimInterval は 1 秒 1 回の決まり(台ごと)。
	nominatimInterval = time.Second
	// nominatimGlobalDaily は環境全体の 1 日の既定(方針の「重い利用」に当たらないよう控えめに)。
	nominatimGlobalDaily = 1000
	nominatimCoordDigits = 4
)

func init() {
	register(Provider{ID: Nominatim, Billing: BillingFree, Hosts: nominatimHosts, GlobalDaily: nominatimGlobalDaily,
		Ops: []OpSpec{
			{Name: "geocode", Args: []string{"q", "limit"}, Check: checkNominatimGeocode, CacheTTL: nominatimCacheTTL},
			{Name: "reverse", Args: []string{"lat", "lng"}, Check: checkNominatimReverse, CacheTTL: nominatimCacheTTL},
		},
		New: func(Secrets) (map[string]Operation, error) {
			return newNominatim(nominatimHosts[0], NewHTTPClient(nominatimHosts, DefaultTimeout), nominatimInterval), nil
		}})
}

// checkNominatimGeocode: q は 1〜200 文字・1 行。limit は 1〜10。
func checkNominatimGeocode(a map[string]string) error {
	q := strings.TrimSpace(a["q"])
	if q == "" {
		return errors.New("q is required")
	}
	if len([]rune(q)) > nominatimMaxQuery {
		return fmt.Errorf("q takes at most %d characters", nominatimMaxQuery)
	}
	if strings.IndexFunc(q, unicode.IsControl) >= 0 {
		return errors.New("q must be one line")
	}
	_, err := argInt(a, "limit", nominatimLimitDef, 1, nominatimLimitMax)
	return err
}

// checkNominatimReverse: lat / lng は両方要る。
func checkNominatimReverse(a map[string]string) error {
	_, _, ok, err := argLatLng(a, nominatimCoordDigits)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("lat and lng are required")
	}
	return nil
}

// newNominatim は base に向けた op(テストは httptest のサーバーと短い interval を渡す)。
func newNominatim(base string, hc *http.Client, interval time.Duration) map[string]Operation {
	n := &nominatim{base: strings.TrimRight(base, "/"), hc: hc, pace: &nominatimPace{interval: interval}}
	return map[string]Operation{
		"geocode": OperationFunc(n.geocode),
		"reverse": OperationFunc(n.reverse),
	}
}

type nominatim struct {
	base string
	hc   *http.Client
	pace *nominatimPace
}

// nominatimPace は台の中で問い合わせの間を interval 空ける(1 秒 1 回の決まり)。
type nominatimPace struct {
	mu       sync.Mutex
	next     time.Time
	interval time.Duration
}

// wait は次に送ってよい時刻まで待つ(ctx が切れたらその error)。
func (p *nominatimPace) wait(ctx context.Context) error {
	p.mu.Lock()
	now := time.Now()
	at := p.next
	if at.Before(now) {
		at = now
	}
	p.next = at.Add(p.interval)
	p.mu.Unlock()
	d := time.Until(at)
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// nominatimPlace は API の 1 件(行になる所だけ。format=jsonv2)。
type nominatimPlace struct {
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
	Category    string `json:"category"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	Error       string `json:"error"`
}

func (n *nominatim) geocode(ctx context.Context, in Input) (Output, error) {
	if err := checkNominatimGeocode(in.Args); err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrBadArgs, err)
	}
	limit, _ := argInt(in.Args, "limit", nominatimLimitDef, 1, nominatimLimitMax)
	v := url.Values{"q": {strings.TrimSpace(in.Args["q"])}, "format": {"jsonv2"}, "limit": {strconv.Itoa(limit)}}
	var places []nominatimPlace
	if err := n.get(ctx, "/search?"+v.Encode(), &places); err != nil {
		return Output{}, err
	}
	rows := []Row{}
	for _, p := range places {
		if r, ok := nominatimRow(p); ok {
			rows = append(rows, r)
		}
		if len(rows) == limit {
			break
		}
	}
	return Output{Rows: rows}, nil
}

func (n *nominatim) reverse(ctx context.Context, in Input) (Output, error) {
	lat, lng, ok, err := argLatLng(in.Args, nominatimCoordDigits)
	if err != nil || !ok {
		return Output{}, fmt.Errorf("%w: lat and lng are required", ErrBadArgs)
	}
	v := url.Values{"lat": {nominatimCoord(lat)}, "lon": {nominatimCoord(lng)}, "format": {"jsonv2"}, "zoom": {"18"}}
	var p nominatimPlace
	if err := n.get(ctx, "/reverse?"+v.Encode(), &p); err != nil {
		return Output{}, err
	}
	// 海の上などは 200 で {"error": "Unable to geocode"}
	r, ok := nominatimRow(p)
	if p.Error != "" || !ok {
		return Output{}, fmt.Errorf("%w: nominatim has no address there", ErrUnknown)
	}
	return Output{Item: &r}, nil
}

// get は間を空けてから path を読み、JSON を out にする。
func (n *nominatim) get(ctx context.Context, path string, out any) error {
	if err := n.pace.wait(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.base+path, nil)
	if err != nil {
		return errors.New("nominatim: building the request failed")
	}
	req.Header.Set("User-Agent", nominatimUserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "ja,en")
	return sendJSON(ctx, n.hc, req, out)
}

// nominatimRow は 1 件を行にする(名前か座標が読めなければ捨てる)。
func nominatimRow(p nominatimPlace) (Row, bool) {
	title := clip(p.DisplayName, nominatimMaxTitle)
	lat, err1 := parseCoord(strings.TrimSpace(p.Lat), 90)
	lng, err2 := parseCoord(strings.TrimSpace(p.Lon), 180)
	if title == "" || err1 != nil || err2 != nil {
		return Row{}, false
	}
	ls, gs := nominatimCoord(lat), nominatimCoord(lng)
	return Row{Title: title, Subtitle: joinDots(clip(p.Type, 40), clip(p.Category, 40)), Value: ls + "," + gs,
		URL: "https://www.openstreetmap.org/?mlat=" + ls + "&mlon=" + gs + "#map=16/" + ls + "/" + gs}, true
}

// nominatimCoord は度を小数 5 桁までの文字に。
func nominatimCoord(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
