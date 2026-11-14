package extapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

// detailsFake answers GET /v1/places/{id} with testdata/googleplaces/<file>.
func detailsFake(t *testing.T, file string) (*atomic.Value, *httptest.Server) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "googleplaces", file))
	if err != nil {
		t.Fatal(err)
	}
	var seen atomic.Value // *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Store(r.Clone(r.Context()))
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return &seen, srv
}

func TestGooglePlacesDetails_2140(t *testing.T) {
	cases := []struct {
		file, lang, id string
		title, sub     string
		url            string
		hours          int
	}{
		{"details_open.json", "", "ChIJopen000001", "兼六園", "営業中 · ★4.5(21,345件)", "https://maps.google.com/?cid=1", 7},
		{"details_closed.json", "en", "ChIJclosed0001", "Kanazawa Castle Park", "Closed now · ★4.3 (987 reviews)", "", 2},
		{"details_24h.json", "ja", "ChIJ24hours001", "コンビニ 金沢駅前店", "営業中 · ★3.2(12件)", "https://maps.google.com/?cid=3", 7},
		{"details_norating.json", "ja", "ChIJnorating01", "新しいカフェ", "評価なし", "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			seen, srv := detailsFake(t, tc.file)
			args := map[string]string{"placeId": tc.id}
			if tc.lang != "" {
				args["lang"] = tc.lang
			}
			out, err := placesOps(t, srv)["details"].Do(t.Context(), Input{Args: args})
			if err != nil {
				t.Fatal(err)
			}
			r := seen.Load().(*http.Request)
			wantLang := tc.lang
			if wantLang == "" {
				wantLang = "ja"
			}
			if r.Method != http.MethodGet || r.URL.Path != "/v1/places/"+tc.id || r.URL.Query().Get("languageCode") != wantLang ||
				r.Header.Get("X-Goog-Api-Key") != placesTestKey || r.Header.Get("X-Goog-FieldMask") != placesDetailsFieldMask {
				t.Fatalf("request %s %s %v", r.Method, r.URL, r.Header)
			}
			if out.Item == nil || out.Item.Title != tc.title || out.Item.Subtitle != tc.sub || out.Item.Value != tc.id || out.Item.URL != tc.url {
				t.Fatalf("item %+v", out.Item)
			}
			if len(out.Rows) != tc.hours {
				t.Fatalf("rows %+v", out.Rows)
			}
			if tc.file == "details_24h.json" && !strings.Contains(out.Rows[0].Title, "24 時間") {
				t.Fatalf("24h row %+v", out.Rows[0])
			}
		})
	}
}

func TestGooglePlacesDetailsBadArgs_2140(t *testing.T) {
	_, srv := detailsFake(t, "details_open.json")
	ops := placesOps(t, srv)
	for _, args := range []map[string]string{
		{},
		{"placeId": "short"},
		{"placeId": "ChIJ/../../x0000"},
		{"placeId": "ChIJopen000001", "lang": "fr"},
	} {
		if _, err := ops["details"].Do(t.Context(), Input{Args: args}); !errors.Is(err, ErrBadArgs) {
			t.Fatalf("%v: err %v", args, err)
		}
	}
}

// Off by default: the search ops and their mask stay as they were; details
// is in the registry (the doc table) but the provider builds it only with
// EXT_GOOGLE_PLACES_DETAILS=1, and a call without it is unavailable.
func TestGooglePlacesDetailsOffByDefault_2140(t *testing.T) {
	p, ok := Lookup(GooglePlaces)
	if !ok {
		t.Fatal("googlePlaces not registered")
	}
	if _, ok := p.Op("details"); !ok {
		t.Fatal("details not in the registry")
	}
	if placesFieldMask != "places.id,places.displayName,places.shortFormattedAddress,places.googleMapsUri" {
		t.Fatalf("search mask changed: %s", placesFieldMask)
	}
	for _, v := range []string{"", "0", "1"} {
		t.Setenv(placesDetailsEnv, v)
		ops, err := p.New(Secrets{APIKey: placesTestKey})
		if err != nil {
			t.Fatal(err)
		}
		names := []string{}
		for n := range ops {
			names = append(names, n)
		}
		slices.Sort(names)
		want := []string{"searchNearby", "searchText"}
		if v == "1" {
			want = []string{"details", "searchNearby", "searchText"}
		}
		if !slices.Equal(names, want) {
			t.Fatalf("%s=%q: ops %v", placesDetailsEnv, v, names)
		}
	}

	t.Setenv(placesDetailsEnv, "")
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[GooglePlaces] = true
		c.Reserve[GooglePlaces] = 50
		if c.Secrets == nil {
			c.Secrets = map[string]Secrets{}
		}
		c.Secrets[GooglePlaces] = Secrets{APIKey: placesTestKey}
	}})
	e.fund(user, 100)
	if !e.svc.Enabled(GooglePlaces) {
		t.Fatal("googlePlaces should be on")
	}
	_, err := e.call(e.req(GooglePlaces, "details", map[string]string{"placeId": "ChIJopen000001"}))
	if Reason(err) != ReasonUnavailable {
		t.Fatalf("reason %q (%v)", Reason(err), err)
	}
	if e.runs() != 0 || e.wallet(user).Available() != 100 {
		t.Fatal("counted")
	}
}
