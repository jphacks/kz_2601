package extapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

const foursquareTestKey = "fsq3TestServiceKey_0123456789abcdef"

// foursquareFake is a fake Places API on httptest (no live calls).
type foursquareFake struct {
	status atomic.Int64
	calls  atomic.Int64
	path   atomic.Value // string
	query  atomic.Value // url.Values
}

func newFoursquareFake(t *testing.T) (*foursquareFake, map[string]Operation) {
	t.Helper()
	f := &foursquareFake{}
	f.status.Store(200)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+foursquareTestKey || r.Header.Get("X-Places-Api-Version") == "" || r.Method != http.MethodGet {
			w.WriteHeader(401)
			return
		}
		f.path.Store(r.URL.Path)
		f.query.Store(r.URL.Query())
		if s := f.status.Load(); s != 200 {
			w.WriteHeader(int(s))
			_, _ = w.Write([]byte(`{"message":"no"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/places/search":
			_, _ = w.Write([]byte(`{"results":[
				{"fsq_place_id":"4b5a1c2df964a520","name":"喫茶ひまわり","distance":120,"location":{"formatted_address":"石川県金沢市広坂1-1"},"categories":[{"name":"喫茶店"}]},
				{"fsq_place_id":"bad id!","name":"捨てる"},
				{"fsq_place_id":"5c6d7e8f9a0b1c2d","name":"","distance":3},
				{"fsq_place_id":"6d7e8f9a0b1c2d3e","name":"本屋","location":{}}]}`))
		case strings.HasPrefix(r.URL.Path, "/places/"):
			_, _ = w.Write([]byte(`{"fsq_place_id":"4b5a1c2df964a520","name":"喫茶ひまわり","location":{"formatted_address":"石川県金沢市広坂1-1"},"categories":[{"name":"喫茶店"}]}`))
		}
	}))
	t.Cleanup(srv.Close)
	ops, err := newFoursquare(foursquareTestKey, srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return f, ops
}

func TestFoursquareSearch(t *testing.T) {
	f, ops := newFoursquareFake(t)
	out, err := ops["search"].Do(t.Context(), Input{Args: map[string]string{"lat": "36.561325", "lng": "136.656205", "q": " 喫茶 ", "limit": "5"}})
	if err != nil {
		t.Fatal(err)
	}
	q := f.query.Load().(url.Values)
	if q.Get("ll") != "36.5613,136.6562" || q.Get("query") != "喫茶" || q.Get("radius") != "1000" || q.Get("limit") != "5" || q.Get("fields") != foursquareFields {
		t.Fatalf("query %v", q)
	}
	if len(out.Rows) != 2 {
		t.Fatalf("rows %+v", out.Rows)
	}
	want := Row{Title: "喫茶ひまわり", Subtitle: "喫茶店・石川県金沢市広坂1-1・120 m", Value: "4b5a1c2df964a520", URL: "https://foursquare.com/v/4b5a1c2df964a520"}
	if out.Rows[0] != want || out.Rows[1].Title != "本屋" || out.Rows[1].Subtitle != "" {
		t.Fatalf("rows %+v", out.Rows)
	}
	if out.Usage.Priced {
		t.Fatal("the API reports no cost: the foundation prices the call")
	}
}

func TestFoursquareDetails(t *testing.T) {
	f, ops := newFoursquareFake(t)
	out, err := ops["details"].Do(t.Context(), Input{Args: map[string]string{"placeId": "4b5a1c2df964a520"}})
	if err != nil {
		t.Fatal(err)
	}
	if f.path.Load() != "/places/4b5a1c2df964a520" || out.Item == nil || out.Item.Title != "喫茶ひまわり" || out.Item.Subtitle != "喫茶店・石川県金沢市広坂1-1" {
		t.Fatalf("path %v item %+v", f.path.Load(), out.Item)
	}
	f.status.Store(404)
	if _, err := ops["details"].Do(t.Context(), Input{Args: map[string]string{"placeId": "4b5a1c2df964a520"}}); Reason(err) != ReasonBadRequest {
		t.Fatalf("404: %v", err)
	}
}

func TestFoursquareBadArgs(t *testing.T) {
	f, ops := newFoursquareFake(t)
	p, _ := Lookup(Foursquare)
	for _, c := range []struct {
		op   string
		args map[string]string
	}{
		{"search", map[string]string{}},
		{"search", map[string]string{"lat": "36.5"}},
		{"search", map[string]string{"lat": "91", "lng": "136"}},
		{"search", map[string]string{"lat": "36.5", "lng": "136.6", "radius": "0"}},
		{"search", map[string]string{"lat": "36.5", "lng": "136.6", "radius": "100001"}},
		{"search", map[string]string{"lat": "36.5", "lng": "136.6", "limit": "51"}},
		{"search", map[string]string{"lat": "36.5", "lng": "136.6", "q": strings.Repeat("あ", 101)}},
		{"search", map[string]string{"lat": "36.5", "lng": "136.6", "q": "a\nb"}},
		{"details", map[string]string{"placeId": ""}},
		{"details", map[string]string{"placeId": "../x"}},
	} {
		if _, err := p.CheckArgs(c.op, c.args); !errors.Is(err, ErrUnknown) {
			t.Errorf("CheckArgs %s %v: %v", c.op, c.args, err)
		}
		if _, err := ops[c.op].Do(t.Context(), Input{Args: c.args}); !errors.Is(err, ErrBadArgs) {
			t.Errorf("Do %s %v: %v", c.op, c.args, err)
		}
	}
	if _, err := p.CheckArgs("search", map[string]string{"lat": "36.5", "lng": "136.6", "q": "https://evil.example/x"}); !errors.Is(err, ErrUnknown) {
		t.Error("a URL argument was accepted")
	}
	if f.calls.Load() != 0 {
		t.Fatalf("bad args reached the API %d times", f.calls.Load())
	}
}

func TestFoursquareUpstreamErrors(t *testing.T) {
	f, ops := newFoursquareFake(t)
	for _, s := range []int{400, 401, 403, 429, 500} {
		f.status.Store(int64(s))
		_, err := ops["search"].Do(t.Context(), Input{Args: map[string]string{"lat": "36.5", "lng": "136.6"}})
		if Reason(err) != ReasonUpstream || StatusOf(Reason(err)) != http.StatusBadGateway {
			t.Fatalf("%d: %v", s, err)
		}
	}
}

func TestFoursquareProvider(t *testing.T) {
	if _, err := newFoursquare("", "https://x", nil); err == nil {
		t.Fatal("empty key accepted")
	}
	p, ok := Lookup(Foursquare)
	if !ok || p.Billing != BillingPaid || p.NeedsOAuth || len(p.Hosts) != 1 || p.Hosts[0] != "https://places-api.foursquare.com" {
		t.Fatalf("provider %+v", p)
	}
	for _, o := range p.Ops {
		if o.CacheTTL != 0 {
			t.Fatalf("%s is cached: the terms are not read yet", o.Name)
		}
	}
}
