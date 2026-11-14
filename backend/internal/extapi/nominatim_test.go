package extapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// nominatimServer は nominatim.openstreetmap.org の代わり(本物は呼ばない)。
type nominatimServer struct {
	*httptest.Server
	hits   atomic.Int64
	last   atomic.Value
	ua     atomic.Value
	status int
	body   string
}

const nominatimSearchBody = `[
{"place_id":1,"lat":"36.5613","lon":"136.6562","category":"place","type":"city","display_name":" 金沢市, 石川県, 日本 ","importance":0.7},
{"place_id":2,"lat":"36.57","lon":"136.65","category":"railway","type":"station","display_name":"金沢駅, 金沢市, 石川県, 日本"},
{"place_id":3,"lat":"abc","lon":"136.65","category":"place","type":"town","display_name":"壊れた座標"},
{"place_id":4,"lat":"36.5","lon":"136.6","category":"place","type":"hamlet","display_name":""}
]`

const nominatimReverseBody = `{"place_id":5,"lat":"36.56131","lon":"136.65622","category":"amenity","type":"townhall","display_name":"金沢市役所, 広坂, 金沢市, 石川県, 920-8577, 日本"}`

func newNominatimServer(t *testing.T) *nominatimServer {
	t.Helper()
	s := &nominatimServer{status: http.StatusOK, body: nominatimSearchBody}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		s.last.Store(r.URL.RequestURI())
		s.ua.Store(r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(s.body))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *nominatimServer) uri() string { v, _ := s.last.Load().(string); return v }

func (s *nominatimServer) ops() map[string]Operation { return newNominatim(s.URL, s.Client(), 0) }

func TestNominatimRegistered(t *testing.T) {
	p, ok := Lookup(Nominatim)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || p.GlobalDaily != nominatimGlobalDaily || !slices.Equal(p.Hosts, []string{"https://nominatim.openstreetmap.org"}) {
		t.Fatalf("nominatim %+v", p)
	}
	if !slices.Contains(ReviewBuildFeatures(), "externalCall.nominatim") {
		t.Fatal("a provider without OAuth belongs to the review build")
	}
	if len(p.Ops) != 2 || p.Ops[0].Name != "geocode" || p.Ops[1].Name != "reverse" {
		t.Fatalf("ops %+v", p.Ops)
	}
	for _, o := range p.Ops {
		if o.Write || o.CacheTTL != 24*time.Hour {
			t.Fatalf("op %+v", o)
		}
	}
	cfg, err := ConfigFromEnv(func(string) string { return "" })
	if err != nil || cfg.Enabled[Nominatim] || cfg.Limits[Nominatim].GlobalDaily != nominatimGlobalDaily {
		t.Fatalf("default: %v %v %+v", cfg.Enabled, err, cfg.Limits[Nominatim])
	}
	cfg, err = ConfigFromEnv(func(k string) string { return map[string]string{"EXT_NOMINATIM_ENABLED": "1"}[k] })
	if err != nil || !cfg.Enabled[Nominatim] {
		t.Fatalf("EXT_NOMINATIM_ENABLED: %v %v", cfg.Enabled, err)
	}
	if NominatimAttribution != "© OpenStreetMap contributors" {
		t.Fatalf("attribution %q", NominatimAttribution)
	}
}

var nominatimWant = []Row{
	{Title: "金沢市, 石川県, 日本", Subtitle: "city・place", Value: "36.5613,136.6562", URL: "https://www.openstreetmap.org/?mlat=36.5613&mlon=136.6562#map=16/36.5613/136.6562"},
	{Title: "金沢駅, 金沢市, 石川県, 日本", Subtitle: "station・railway", Value: "36.57,136.65", URL: "https://www.openstreetmap.org/?mlat=36.57&mlon=136.65#map=16/36.57/136.65"},
}

func TestNominatimGeocode(t *testing.T) {
	s := newNominatimServer(t)
	out, err := s.ops()["geocode"].Do(context.Background(), Input{Args: map[string]string{"q": " 金沢 ", "limit": "3"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.uri() != "/search?format=jsonv2&limit=3&q=%E9%87%91%E6%B2%A2" {
		t.Fatalf("request %s", s.uri())
	}
	if ua, _ := s.ua.Load().(string); ua != "TailorIt/1.0 (https://tailor-it.app)" {
		t.Fatalf("User-Agent %q", ua)
	}
	if !slices.Equal(out.Rows, nominatimWant) {
		t.Fatalf("rows %+v", out.Rows)
	}
	// limit は行を切る・既定は 5
	out, err = s.ops()["geocode"].Do(context.Background(), Input{Args: map[string]string{"q": "a", "limit": "1"}})
	if err != nil || len(out.Rows) != 1 {
		t.Fatalf("limit 1: %+v %v", out.Rows, err)
	}
	if _, err := s.ops()["geocode"].Do(context.Background(), Input{Args: map[string]string{"q": "a"}}); err != nil || !strings.Contains(s.uri(), "limit=5") {
		t.Fatalf("default limit: %s %v", s.uri(), err)
	}
	// 無い所は行 0(エラーでない)
	s.body = `[]`
	out, err = s.ops()["geocode"].Do(context.Background(), Input{Args: map[string]string{"q": "a"}})
	if err != nil || out.Rows == nil || len(out.Rows) != 0 {
		t.Fatalf("empty: %+v %v", out.Rows, err)
	}
	// 429 / 5xx は ProviderError(やり直せる)
	for _, st := range []int{http.StatusTooManyRequests, http.StatusBadGateway} {
		s.status, s.body = st, `{"error":"x"}`
		var pe *ProviderError
		if _, err := s.ops()["geocode"].Do(context.Background(), Input{Args: map[string]string{"q": "a"}}); !errors.As(err, &pe) || !pe.Retryable || pe.Status != st {
			t.Fatalf("%d: %v", st, err)
		}
	}
}

func TestNominatimReverse(t *testing.T) {
	s := newNominatimServer(t)
	s.body = nominatimReverseBody
	out, err := s.ops()["reverse"].Do(context.Background(), Input{Args: map[string]string{"lat": "36.561312", "lng": "136.656219"}})
	if err != nil {
		t.Fatal(err)
	}
	// 4 桁に丸めて送る
	if s.uri() != "/reverse?format=jsonv2&lat=36.5613&lon=136.6562&zoom=18" {
		t.Fatalf("request %s", s.uri())
	}
	want := Row{Title: "金沢市役所, 広坂, 金沢市, 石川県, 920-8577, 日本", Subtitle: "townhall・amenity", Value: "36.56131,136.65622",
		URL: "https://www.openstreetmap.org/?mlat=36.56131&mlon=136.65622#map=16/36.56131/136.65622"}
	if out.Item == nil || *out.Item != want || len(out.Rows) != 0 {
		t.Fatalf("item %+v", out.Item)
	}
	// 住所が無い所は 200 で error → 400 bad_request(呼んだ後)
	s.body = `{"error":"Unable to geocode"}`
	if _, err := s.ops()["reverse"].Do(context.Background(), Input{Args: map[string]string{"lat": "0", "lng": "0"}}); !errors.Is(err, ErrUnknown) || Reason(err) != ReasonBadRequest {
		t.Fatalf("no address: %v", err)
	}
	// 引数が無ければ呼ばない
	hits := s.hits.Load()
	if _, err := s.ops()["reverse"].Do(context.Background(), Input{Args: map[string]string{"lat": "1"}}); !errors.Is(err, ErrBadArgs) || s.hits.Load() != hits {
		t.Fatalf("missing lng: %v", err)
	}
}

// 1 秒 1 回の決まり: 2 回目は interval の後に送る。ctx が切れたら待たない。
func TestNominatimPace(t *testing.T) {
	s := newNominatimServer(t)
	ops := newNominatim(s.URL, s.Client(), 150*time.Millisecond)
	start := time.Now()
	for range 2 {
		if _, err := ops["geocode"].Do(context.Background(), Input{Args: map[string]string{"q": "a"}}); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d < 150*time.Millisecond {
		t.Fatalf("second call after %v", d)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	hits := s.hits.Load()
	if _, err := ops["geocode"].Do(ctx, Input{Args: map[string]string{"q": "a"}}); !errors.Is(err, context.DeadlineExceeded) || s.hits.Load() != hits {
		t.Fatalf("cancelled wait: %v hits %d", err, s.hits.Load()-hits)
	}
}

func TestNominatimCheck(t *testing.T) {
	p, _ := Lookup(Nominatim)
	for _, c := range []struct {
		op   string
		args map[string]string
		ok   bool
	}{
		{"geocode", map[string]string{"q": "金沢駅"}, true},
		{"geocode", map[string]string{"q": "1 Infinite Loop, Cupertino", "limit": "10"}, true},
		{"geocode", map[string]string{"q": strings.Repeat("あ", 200)}, true},
		{"geocode", nil, false},
		{"geocode", map[string]string{"q": "  "}, false},
		{"geocode", map[string]string{"q": strings.Repeat("あ", 201)}, false},
		{"geocode", map[string]string{"q": "a\nb"}, false},
		{"geocode", map[string]string{"q": "a", "limit": "0"}, false},
		{"geocode", map[string]string{"q": "a", "limit": "11"}, false},
		{"geocode", map[string]string{"q": "https://example.com/x"}, false},
		{"geocode", map[string]string{"q": "a", "lat": "1"}, false},
		{"reverse", map[string]string{"lat": "36.5", "lng": "136.6"}, true},
		{"reverse", map[string]string{"lat": "-90", "lng": "180"}, true},
		{"reverse", map[string]string{"lat": "36.5"}, false},
		{"reverse", nil, false},
		{"reverse", map[string]string{"lat": "91", "lng": "0"}, false},
		{"reverse", map[string]string{"lat": "0x1", "lng": "0"}, false},
	} {
		_, err := p.CheckArgs(c.op, c.args)
		if (err == nil) != c.ok {
			t.Errorf("%s %v: err %v want ok=%v", c.op, c.args, err, c.ok)
		}
		if err != nil && !errors.Is(err, ErrUnknown) {
			t.Errorf("%s %v: %v is not ErrUnknown", c.op, c.args, err)
		}
	}
}
