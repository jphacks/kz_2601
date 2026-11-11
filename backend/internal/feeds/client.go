package feeds

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"syscall"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

const (
	requestTimeout = 8 * time.Second
	userAgent      = "TailorIt-feeds/1 (+https://github.com/jphacks/kz_2601)"
)

// RSSFeed is one allowlisted feed. Adding one is a reviewed change here and
// in dsl.RSSFeeds (TestRSSAllowlistMatchesDSL keeps them equal).
type RSSFeed struct {
	Name      string
	URL       string // https, fixed
	Publisher string // attribution
	SiteURL   string
	License   string // shown with the source; "" = 公共データ利用規約(第1.0版)
}

// Endpoints are the only URLs the service fetches. Tests point them at an
// httptest server; production uses DefaultEndpoints.
type Endpoints struct {
	WeatherBase string // + "{region}.json"
	WarningBase string // + "{region}.json"
	Pressure    string // + "?lat=..&lon=.." (pressurePoints)
	Holidays    string
	RSS         map[string]RSSFeed
	Arxiv       string // + "?search_query=cat:{category}&..." (dsl.ArxivCategories)
	Diet        string // + "?any={q}&..." (国会会議録 speech search)
}

// DefaultEndpoints are the production sources (terms checked 2026-11-06,
// docs/backend/features/external-feeds.md).
func DefaultEndpoints() Endpoints {
	return Endpoints{
		WeatherBase: "https://www.jma.go.jp/bosai/forecast/data/forecast/",
		WarningBase: "https://www.jma.go.jp/bosai/warning/data/r8/",
		Pressure:    "https://api.met.no/weatherapi/locationforecast/2.0/compact",
		Holidays:    "https://www8.cao.go.jp/chosei/shukujitsu/syukujitsu.csv",
		RSS: map[string]RSSFeed{
			"digital-news":    {Name: "デジタル庁 新着・更新", URL: "https://www.digital.go.jp/rss/news.xml", Publisher: "デジタル庁ホームページ", SiteURL: "https://www.digital.go.jp/"},
			"fdma-disaster":   {Name: "消防庁 災害情報", URL: "https://www.fdma.go.jp/disaster/info/index.xml", Publisher: "消防庁ホームページ", SiteURL: "https://www.fdma.go.jp/"},
			"fdma-news":       {Name: "消防庁 新着情報", URL: "https://www.fdma.go.jp/index.xml", Publisher: "消防庁ホームページ", SiteURL: "https://www.fdma.go.jp/"},
			"go-blog":         {Name: "The Go Blog", URL: "https://go.dev/blog/feed.atom", Publisher: "The Go Blog(go.dev)", SiteURL: "https://go.dev/blog/", License: "CC BY 4.0"},
			"osaka-city-news": {Name: "大阪市 新着情報", URL: "https://www.city.osaka.lg.jp/main/rss/rss_new.xml", Publisher: "大阪市ホームページ", SiteURL: "https://www.city.osaka.lg.jp/", License: "RSS は CC BY"},
			"soumu-news":      {Name: "総務省 新着情報", URL: "https://www.soumu.go.jp/news.rdf", Publisher: "総務省ホームページ", SiteURL: "https://www.soumu.go.jp/"},
		},
		Arxiv: "https://export.arxiv.org/api/query",
		Diet:  "https://kokkai.ndl.go.jp/api/speech",
	}
}

const pdl = "公共データ利用規約(第1.0版)"

func (ep Endpoints) source(q Query) Source {
	switch q.Provider {
	case dsl.FeedWeather:
		return Source{Name: "気象庁ホームページ", URL: "https://www.jma.go.jp/", License: pdl}
	case dsl.FeedWarnings:
		return Source{Name: "気象庁ホームページ", URL: "https://www.jma.go.jp/bosai/warning/", License: pdl}
	case dsl.FeedPressure:
		return Source{Name: "MET Norway(ノルウェー気象研究所)", URL: "https://api.met.no/", License: "CC BY 4.0・6 時間ごとにまとめて加工"}
	case dsl.FeedHolidays:
		return Source{Name: "内閣府ホームページ", URL: "https://www8.cao.go.jp/chosei/shukujitsu/gaiyou.html", License: pdl}
	case dsl.FeedArxiv:
		return Source{Name: "arXiv.org", URL: "https://arxiv.org/list/" + q.FeedID + "/recent", License: "論文の情報は CC0 1.0・arXiv の承認や提携はありません"}
	case dsl.FeedDiet:
		return Source{Name: "国立国会図書館 国会会議録検索システム", URL: "https://kokkai.ndl.go.jp/", License: "検索用 API・発言の著作権は各発言者"}
	}
	f := ep.RSS[q.FeedID]
	license := f.License
	if license == "" {
		license = pdl
	}
	return Source{Name: f.Publisher, URL: f.SiteURL, License: license}
}

// weatherURL builds the forecast URL. region was checked against
// dsl.WeatherRegions (6 digits), so it cannot add a path or a query.
func (ep Endpoints) weatherURL(region string) string { return ep.WeatherBase + region + ".json" }

// warningURL builds the warning URL (region checked like weatherURL).
func (ep Endpoints) warningURL(region string) string { return ep.WarningBase + region + ".json" }

// pressureURL builds the forecast URL for the region's fixed point. MET
// Norway asks for at most 4 decimals; the points have 2.
func (ep Endpoints) pressureURL(region string) (string, bool) {
	p, ok := pressurePoints[region]
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%s?lat=%.2f&lon=%.2f", ep.Pressure, p[0], p[1]), true
}

// arxivURL asks the arXiv API for a category's latest submissions (the RSS
// feeds are empty on weekends). category was checked against
// dsl.ArxivCategories.
func (ep Endpoints) arxivURL(category string) string {
	v := url.Values{"search_query": {"cat:" + category}, "sortBy": {"submittedDate"}, "sortOrder": {"descending"},
		"max_results": {strconv.Itoa(dsl.FeedMaxLimit)}}
	return ep.Arxiv + "?" + v.Encode()
}

// dietURL searches speeches (newest meeting first). q was normalized by
// dsl.NormalizeFeedQuery and is only ever a query value.
func (ep Endpoints) dietURL(q string) string {
	v := url.Values{"any": {q}, "maximumRecords": {strconv.Itoa(dsl.FeedMaxLimit)}, "recordPacking": {"json"}}
	return ep.Diet + "?" + v.Encode()
}

// hosts are the scheme + host pairs the endpoints use.
func (ep Endpoints) hosts() map[string]bool {
	out := map[string]bool{}
	add := func(raw string) {
		if u, err := url.Parse(raw); err == nil && u.Host != "" {
			out[u.Scheme+"://"+u.Host] = true
		}
	}
	add(ep.WeatherBase)
	add(ep.WarningBase)
	add(ep.Pressure)
	add(ep.Holidays)
	for _, f := range ep.RSS {
		add(f.URL)
	}
	add(ep.Arxiv)
	add(ep.Diet)
	return out
}

func (ep Endpoints) allowedURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.User == nil && ep.hosts()[u.Scheme+"://"+u.Host]
}

// checkRedirect follows at most 3 redirects, each to an allowlisted host on
// the same scheme (no https -> http downgrade, no other site).
func (ep Endpoints) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 3 {
		return fmt.Errorf("%w: too many redirects", ErrRedirectBlocked)
	}
	if !ep.allowedURL(req.URL.String()) || (len(via) > 0 && req.URL.Scheme != via[0].URL.Scheme) {
		return fmt.Errorf("%w: %s", ErrRedirectBlocked, req.URL.Host)
	}
	return nil
}

// ErrBlockedAddress is returned when a name resolves to a non-public address.
var ErrBlockedAddress = errors.New("feeds: address is not public")

// NewClient is the production client: no proxy from the environment, every
// connection checked to go to a public unicast address (not loopback,
// private, link-local incl. the metadata server, CGNAT, multicast ...), and
// bounded handshake / header / total times.
func NewClient() *http.Client {
	d := &net.Dialer{Timeout: 5 * time.Second, Control: dialControl}
	t := &http.Transport{
		Proxy:                  nil,
		DialContext:            d.DialContext,
		ForceAttemptHTTP2:      true,
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  requestTimeout,
		MaxIdleConns:           10,
		IdleConnTimeout:        90 * time.Second,
		MaxResponseHeaderBytes: 32 << 10,
	}
	return &http.Client{Transport: t, Timeout: requestTimeout}
}

// dialControl runs after name resolution, on the exact address being dialled,
// so a name that resolves (or re-resolves) to an internal address is refused.
func dialControl(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrBlockedAddress, address)
	}
	if !PublicAddr(ap.Addr()) {
		return fmt.Errorf("%w: %s", ErrBlockedAddress, ap.Addr())
	}
	return nil
}

var blockedPrefixes = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24",
		"203.0.113.0/24", "240.0.0.0/4", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/23", "2001:db8::/32", "2002::/16",
	} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

// PublicAddr reports whether a is a globally routable unicast address.
func PublicAddr(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() ||
		a.IsInterfaceLocalMulticast() || a.IsMulticast() || a.IsUnspecified() {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(a) {
			return false
		}
	}
	return true
}
