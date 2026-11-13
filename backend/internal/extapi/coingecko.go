package extapi

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// CoinGecko (#2139, a trial for #1688): crypto asset prices from the public
// API (api.coingecko.com/api/v3). Free, no OAuth: a call is one `runs`.
// Off by default (EXT_COINGECKO_ENABLED=1); a demo key, when set
// (EXT_COINGECKO_API_KEY), goes in the x-cg-demo-api-key header.
//
//	price(ids, vs?)          Rows: one per id — price / "24時間 +1.23%"
//	history(id, vs?, days?)  Rows: one per day (JST) — the day's last price
//
// ids are CoinGecko's coin ids (lower case, digits, "-"), at most
// coinGeckoMaxIDs joined by commas; vs is one quote currency (default jpy);
// days is 1-90 (default 7).
//
// Whether to ship it, the terms and the credit line ("data by CoinGecko" or
// the like) are #1688's decision: no attribution text is fixed here.
const CoinGecko = "coingecko"

var coinGeckoHosts = []string{"https://api.coingecko.com"}

const (
	coinGeckoMaxIDs      = 10
	coinGeckoDaysDefault = 7
	coinGeckoDaysMax     = 90
	coinGeckoDefaultVs   = "jpy"
)

var (
	coinIDRe  = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	coinVsRe  = regexp.MustCompile(`^[a-z]{3,5}$`)
	coinKeyRe = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)
)

func init() {
	register(Provider{ID: CoinGecko, Billing: BillingFree, Hosts: coinGeckoHosts,
		Ops: []OpSpec{
			{Name: "price", Args: []string{"ids", "vs"}, Check: checkCoinPrice, CacheTTL: time.Minute},
			{Name: "history", Args: []string{"id", "vs", "days"}, Check: checkCoinHistory, CacheTTL: time.Hour},
		},
		New: func(sec Secrets) (map[string]Operation, error) {
			return newCoinGecko(sec.APIKey, coinGeckoHosts[0], NewHTTPClient(coinGeckoHosts, DefaultTimeout))
		}})
}

func checkCoinID(id string) error {
	if len(id) > 64 || !coinIDRe.MatchString(id) {
		return errors.New("a coin id is lower case letters, digits and -, like bitcoin")
	}
	return nil
}

func checkCoinVs(a map[string]string) error {
	if v, ok := a["vs"]; ok && !coinVsRe.MatchString(v) {
		return errors.New("vs must be one currency like jpy or usd")
	}
	return nil
}

// coinIDs splits ids: 1-coinGeckoMaxIDs coin ids, no repeats.
func coinIDs(s string) ([]string, error) {
	if strings.TrimSpace(s) == "" {
		return nil, errors.New("ids is required")
	}
	ids := strings.Split(s, ",")
	if len(ids) > coinGeckoMaxIDs {
		return nil, fmt.Errorf("ids takes at most %d coins", coinGeckoMaxIDs)
	}
	for i, id := range ids {
		if err := checkCoinID(id); err != nil {
			return nil, err
		}
		if slices.Contains(ids[:i], id) {
			return nil, errors.New("ids has a repeat")
		}
	}
	return ids, nil
}

func checkCoinPrice(a map[string]string) error {
	if _, err := coinIDs(a["ids"]); err != nil {
		return err
	}
	return checkCoinVs(a)
}

func checkCoinHistory(a map[string]string) error {
	id, ok := a["id"]
	if !ok {
		return errors.New("id is required")
	}
	if err := checkCoinID(id); err != nil {
		return err
	}
	if _, err := argInt(a, "days", coinGeckoDaysDefault, 1, coinGeckoDaysMax); err != nil {
		return err
	}
	return checkCoinVs(a)
}

func coinVs(a map[string]string) string {
	if v := a["vs"]; v != "" {
		return v
	}
	return coinGeckoDefaultVs
}

// newCoinGecko builds the ops against base (tests pass an httptest server).
// key is optional (the demo key); a malformed one is an error.
func newCoinGecko(key, base string, hc *http.Client) (map[string]Operation, error) {
	if key != "" && !coinKeyRe.MatchString(key) {
		return nil, errors.New("EXT_COINGECKO_API_KEY is not a key")
	}
	c := &coinGecko{key: key, base: strings.TrimRight(base, "/"), hc: hc}
	return map[string]Operation{
		"price":   OperationFunc(c.price),
		"history": OperationFunc(c.history),
	}, nil
}

type coinGecko struct {
	key  string
	base string
	hc   *http.Client
}

func (c *coinGecko) get(ctx context.Context, path string, q url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path+"?"+q.Encode(), nil)
	if err != nil {
		return errors.New("coingecko: building the request failed")
	}
	req.Header.Set("Accept", "application/json")
	if c.key != "" {
		req.Header.Set("x-cg-demo-api-key", c.key)
	}
	return sendJSON(ctx, c.hc, req, out)
}

// price: GET /api/v3/simple/price. A coin the provider does not know is
// missing from the answer and gets no row; none known is 400 bad_request.
func (c *coinGecko) price(ctx context.Context, in Input) (Output, error) {
	ids, err := coinIDs(in.Args["ids"])
	if err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrBadArgs, err)
	}
	vs := coinVs(in.Args)
	var a map[string]map[string]*float64
	q := url.Values{"ids": {strings.Join(ids, ",")}, "vs_currencies": {vs}, "include_24hr_change": {"true"}}
	if err := c.get(ctx, "/api/v3/simple/price", q, &a); err != nil {
		return Output{}, err
	}
	rows := []Row{}
	for _, id := range ids {
		p := a[id][vs]
		if p == nil || !coinFinite(*p) {
			continue
		}
		rows = append(rows, Row{Title: id, Subtitle: coinChange(a[id][vs+"_24h_change"]), Value: coinPrice(*p, vs)})
	}
	if len(rows) == 0 {
		return Output{}, fmt.Errorf("%w: coingecko has none of these coins in %s", ErrUnknown, vs)
	}
	return Output{Rows: rows}, nil
}

// coinHistoryAnswer is /coins/{id}/market_chart: [unix ms, price] pairs.
type coinHistoryAnswer struct {
	Prices [][2]float64 `json:"prices"`
}

// history: GET /api/v3/coins/{id}/market_chart with interval=daily. A day
// can have two points (00:00 UTC and "now"); the row is the last of each
// JST day, newest first.
func (c *coinGecko) history(ctx context.Context, in Input) (Output, error) {
	id := in.Args["id"]
	if err := checkCoinID(id); err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrBadArgs, err)
	}
	days, err := argInt(in.Args, "days", coinGeckoDaysDefault, 1, coinGeckoDaysMax)
	if err != nil {
		return Output{}, err
	}
	vs := coinVs(in.Args)
	var a coinHistoryAnswer
	q := url.Values{"vs_currency": {vs}, "days": {strconv.Itoa(days)}, "interval": {"daily"}}
	if err := c.get(ctx, "/api/v3/coins/"+id+"/market_chart", q, &a); err != nil {
		var pe *ProviderError
		if errors.As(err, &pe) && pe.Status == http.StatusNotFound {
			return Output{}, fmt.Errorf("%w: coingecko has no coin %s", ErrUnknown, id)
		}
		return Output{}, err
	}
	rows := coinHistoryRows(a.Prices, vs, days+1)
	if len(rows) == 0 {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("coingecko: no prices")}
	}
	return Output{Rows: rows}, nil
}

// coinHistoryRows keeps the last point of each JST day, newest first, at
// most limit rows.
func coinHistoryRows(points [][2]float64, vs string, limit int) []Row {
	var rows []Row
	for i := len(points) - 1; i >= 0 && len(rows) < limit; i-- {
		ms, p := points[i][0], points[i][1]
		if ms <= 0 || !coinFinite(p) {
			continue
		}
		day := time.UnixMilli(int64(ms)).In(jst).Format("2006/1/2")
		if len(rows) > 0 && rows[len(rows)-1].Title == day {
			continue // an earlier point of a day already shown
		}
		rows = append(rows, Row{Title: day, Value: coinPrice(p, vs)})
	}
	return rows
}

func coinFinite(v float64) bool { return finite(v) && v >= 0 }

// coinPrice writes "1234567.89 JPY": 2 places from 1 up, 8 below (small
// coins).
func coinPrice(v float64, vs string) string {
	var s string
	if v >= 1 {
		s = strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
	} else {
		s = strconv.FormatFloat(math.Round(v*1e8)/1e8, 'f', -1, 64)
	}
	return s + " " + strings.ToUpper(vs)
}

// coinChange writes "24時間 +1.23%" ("" when unknown).
func coinChange(v *float64) string {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
		return ""
	}
	r := math.Round(*v*100)/100 + 0
	s := strconv.FormatFloat(r, 'f', 2, 64)
	if r >= 0 {
		s = "+" + s
	}
	return "24時間 " + s + "%"
}
