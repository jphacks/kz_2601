package extapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const placesTestKey = "test-places-key-00000000000000"

// placesFake is a fake Places API (New): it records the request and answers.
type placesFake struct {
	status atomic.Int64
	calls  atomic.Int64
	path   atomic.Value // string
	body   atomic.Value // map[string]any
	header atomic.Value // http.Header
}

func newPlacesFake(t *testing.T) (*placesFake, *httptest.Server) {
	t.Helper()
	f := &placesFake{}
	f.status.Store(200)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		f.path.Store(r.URL.Path + "?" + r.URL.RawQuery)
		f.header.Store(r.Header.Clone())
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		f.body.Store(m)
		if s := f.status.Load(); s != 200 {
			w.WriteHeader(int(s))
			_, _ = w.Write([]byte(`{"error":{"code":` + "400" + `,"message":"API key not valid"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"places":[
			{"id":"ChIJ1","displayName":{"text":"兼六園","languageCode":"ja"},"shortFormattedAddress":"金沢市兼六町1","googleMapsUri":"https://maps.google.com/?cid=1"},
			{"id":"","displayName":{"text":"no id"}},
			{"id":"ChIJ2","displayName":{"text":"金沢城公園"},"googleMapsUri":"javascript:alert(1)"},
			{"id":"ChIJ3","displayName":{"text":"三つ目"}}]}`))
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func placesOps(t *testing.T, srv *httptest.Server) map[string]Operation {
	t.Helper()
	ops, err := newGooglePlaces(placesTestKey, srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

func TestGooglePlacesSearchText(t *testing.T) {
	f, srv := newPlacesFake(t)
	ops := placesOps(t, srv)
	out, err := ops["searchText"].Do(t.Context(), Input{Args: map[string]string{"q": "庭園", "lat": "36.562123", "lng": "136.662456", "limit": "2"}})
	if err != nil {
		t.Fatal(err)
	}
	h := f.header.Load().(http.Header)
	if f.path.Load() != "/v1/places:searchText?" || h.Get("X-Goog-Api-Key") != placesTestKey || h.Get("X-Goog-FieldMask") != placesFieldMask {
		t.Fatalf("path %v header %v", f.path.Load(), h)
	}
	// the cheap mask: no rating, hours, phone, reviews or photos
	for _, field := range []string{"rating", "Hours", "phone", "reviews", "photos", "*"} {
		if strings.Contains(placesFieldMask, field) {
			t.Fatalf("field mask asks for %s", field)
		}
	}
	b := f.body.Load().(map[string]any)
	c := b["locationBias"].(map[string]any)["circle"].(map[string]any)
	center := c["center"].(map[string]any)
	if b["textQuery"] != "庭園" || b["pageSize"] != 2.0 || b["languageCode"] != "ja" || c["radius"] != 5000.0 ||
		center["latitude"] != 36.5621 || center["longitude"] != 136.6625 {
		t.Fatalf("body %v", b)
	}
	// rows: no id skipped, a non-https link dropped, cut at limit
	want := []Row{
		{Title: "兼六園", Subtitle: "金沢市兼六町1", Value: "ChIJ1", URL: "https://maps.google.com/?cid=1"},
		{Title: "金沢城公園", Value: "ChIJ2"},
	}
	if len(out.Rows) != 2 || out.Rows[0] != want[0] || out.Rows[1] != want[1] || out.Usage.Priced {
		t.Fatalf("output %+v", out)
	}
	// no point: no bias
	if _, err := ops["searchText"].Do(t.Context(), Input{Args: map[string]string{"q": "金沢 カフェ"}}); err != nil {
		t.Fatal(err)
	}
	if b := f.body.Load().(map[string]any); b["locationBias"] != nil || b["pageSize"] != 10.0 {
		t.Fatalf("body %v", b)
	}
}

func TestGooglePlacesSearchNearby(t *testing.T) {
	f, srv := newPlacesFake(t)
	ops := placesOps(t, srv)
	out, err := ops["searchNearby"].Do(t.Context(), Input{Args: map[string]string{"type": "cafe", "lat": "36.56", "lng": "136.65", "radius": "300"}})
	if err != nil || len(out.Rows) != 3 {
		t.Fatalf("%+v %v", out, err)
	}
	b := f.body.Load().(map[string]any)
	c := b["locationRestriction"].(map[string]any)["circle"].(map[string]any)
	if f.path.Load() != "/v1/places:searchNearby?" || c["radius"] != 300.0 || b["maxResultCount"] != 10.0 ||
		len(b["includedTypes"].([]any)) != 1 || b["includedTypes"].([]any)[0] != "cafe" {
		t.Fatalf("body %v", b)
	}
	if _, err := ops["searchNearby"].Do(t.Context(), Input{Args: map[string]string{"lat": "36.56", "lng": "136.65"}}); err != nil {
		t.Fatal(err)
	}
	if b := f.body.Load().(map[string]any); b["includedTypes"] != nil || b["locationRestriction"].(map[string]any)["circle"].(map[string]any)["radius"] != 1000.0 {
		t.Fatalf("defaults %v", b)
	}
}

func TestGooglePlacesRefusesBadArgs(t *testing.T) {
	f, srv := newPlacesFake(t)
	ops := placesOps(t, srv)
	for _, c := range []struct {
		op   string
		args map[string]string
	}{
		{"searchText", map[string]string{}},
		{"searchText", map[string]string{"q": "  "}},
		{"searchText", map[string]string{"q": strings.Repeat("あ", placesMaxQuery+1)}},
		{"searchText", map[string]string{"q": "x", "lat": "36"}},
		{"searchText", map[string]string{"q": "x", "radius": "100"}},
		{"searchText", map[string]string{"q": "x", "lat": "36", "lng": "136", "radius": "50001"}},
		{"searchText", map[string]string{"q": "x", "limit": "21"}},
		{"searchNearby", map[string]string{"type": "cafe"}},
		{"searchNearby", map[string]string{"type": "Cafe", "lat": "1", "lng": "1"}},
		{"searchNearby", map[string]string{"type": "cafe,bar", "lat": "1", "lng": "1"}},
		{"searchNearby", map[string]string{"lat": "1", "lng": "1", "radius": "0"}},
		{"searchNearby", map[string]string{"lat": "-91", "lng": "1"}},
	} {
		if _, err := ops[c.op].Do(t.Context(), Input{Args: c.args}); Reason(err) != ReasonBadRequest {
			t.Fatalf("%s %v: %s (%v)", c.op, c.args, Reason(err), err)
		}
	}
	if f.calls.Load() != 0 {
		t.Fatal("a bad argument reached the API")
	}
}

func TestGooglePlacesDefinition(t *testing.T) {
	p, ok := Lookup(GooglePlaces)
	if !ok || p.Billing != BillingPaid || p.NeedsOAuth || p.Fake || p.EnvName() != "GOOGLE_PLACES" ||
		len(p.Hosts) != 1 || p.Hosts[0] != googlePlacesHost {
		t.Fatalf("%+v", p)
	}
	for _, k := range []string{"", "short", "has space " + placesTestKey} {
		if _, err := p.New(Secrets{APIKey: k}); err == nil {
			t.Fatalf("key %q accepted", k)
		}
	}
	if _, err := p.New(Secrets{APIKey: placesTestKey}); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"EXT_GOOGLE_PLACES_API_KEY":                placesTestKey,
		"EXT_COST_USD_GOOGLE_PLACES_SEARCH_TEXT":   "0.5",
		"EXT_COST_USD_GOOGLE_PLACES_SEARCH_NEARBY": "0.25",
	}
	c, err := ConfigFromEnv(func(k string) string { return env[k] })
	if err != nil || c.Enabled[GooglePlaces] || c.CostUSD[GooglePlaces+".searchText"] != 0.5 || c.CostUSD[GooglePlaces+".searchNearby"] != 0.25 {
		t.Fatalf("%+v %v", c.CostUSD, err)
	}
}

// Through Service.Call: per-request cost from env, the rest of the hold
// back; failures give everything back; the key is in no log line.
func TestGooglePlacesThroughService(t *testing.T) {
	f, srv := newPlacesFake(t)
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[GooglePlaces] = true
		c.Secrets = map[string]Secrets{GooglePlaces: {APIKey: placesTestKey}}
		c.Reserve[GooglePlaces] = 50
		c.CostUSD[GooglePlaces+".searchText"] = 0.2 // a test number, not a price
	}})
	if !e.svc.Enabled(GooglePlaces) {
		t.Fatal("not enabled")
	}
	e.svc.ops[GooglePlaces] = placesOps(t, srv)
	e.fund(user, 100)
	res, err := e.call(e.req(GooglePlaces, "searchText", map[string]string{"q": "庭園"}))
	if err != nil || len(res.Output.Rows) != 3 || res.Charge.Coins != 20 || res.Charge.Balance != 80 || e.runs() != 1 {
		t.Fatalf("%+v %v runs=%d", res, err, e.runs())
	}
	for _, s := range []int64{500, 503, 429, 400, 403} {
		f.status.Store(s)
		_, err := e.call(e.req(GooglePlaces, "searchText", map[string]string{"q": "庭園"}))
		if Reason(err) != ReasonUpstream {
			t.Fatalf("status %d: %v", s, err)
		}
		if w := e.wallet(user); w.Reserved != 0 || w.Available() != 80 || e.runs() != 1 {
			t.Fatalf("status %d: not given back: %+v runs=%d", s, w, e.runs())
		}
	}
	if strings.Contains(e.logs.String(), placesTestKey) {
		t.Fatal("the key is in the logs")
	}
}
