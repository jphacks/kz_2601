package appconfig

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/feeds"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// bikeshare's q is a point checked here; the source is asked for fixed files
// only (the point never leaves the server).
func TestFeedBikeshareQuery(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.URL.String())
		mu.Unlock()
		switch r.URL.Path {
		case "/x/station_information.json":
			_, _ = w.Write([]byte(`{"data":{"stations":[{"station_id":"1","name":"駅前","lat":35.681,"lon":139.767}]}}`))
		case "/x/station_status.json":
			_, _ = w.Write([]byte(`{"data":{"stations":[{"station_id":"1","num_bikes_available":3,"num_docks_available":4,"is_renting":true,"is_returning":true}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	now := func() time.Time { return time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC) }
	svc := feeds.New(feeds.Options{Client: srv.Client(), Now: now, Endpoints: &feeds.Endpoints{Bikeshare: map[string]feeds.BikeshareSystem{
		"hellocycling": {Name: "HELLO CYCLING", Info: srv.URL + "/x/station_information.json", Status: srv.URL + "/x/station_status.json",
			Credit: "OpenStreet株式会社 / 公共交通オープンデータ協議会", DatasetURL: "https://ckan.odpt.org/dataset/x", License: "CC BY 4.0"},
	}}})
	e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory(), WithFeeds(svc, 100, now)))}

	for _, body := range []map[string]any{
		{"feedId": "hellocycling"},
		{"feedId": "hellocycling", "q": "駅前"},
		{"feedId": "hellocycling", "q": "0,0"},
		{"feedId": "hellocycling", "q": "35.681,139.767,1"},
		{"feedId": "docomo-cycle", "q": "35.681,139.767"},
		{"feedId": "hellocycling", "q": "35.681,139.767", "region": "130000"},
	} {
		if r := e.do("u1", http.MethodPost, "/v1/feeds/bikeshare", body, nil); r.code != http.StatusBadRequest {
			t.Errorf("%v: %d %s, want 400", body, r.code, r.raw)
		}
	}
	if len(got) != 0 {
		t.Fatalf("rejected requests reached the source: %v", got)
	}
	r := e.do("u1", http.MethodPost, "/v1/feeds/bikeshare", map[string]any{"feedId": "hellocycling", "q": "35.6812,139.7671", "limit": 3}, nil)
	items, _ := r.body["items"].([]any)
	if r.code != http.StatusOK || len(items) != 1 || r.body["title"] != "HELLO CYCLINGの近くのポート" {
		t.Fatalf("%d %s", r.code, r.raw)
	}
	if it, _ := items[0].(map[string]any); it["subtitle"] != "自転車 3 台・返せる空き 4・約 50 m" {
		t.Errorf("item = %v", it)
	}
	if cc := r.hdr.Get("Cache-Control"); cc != "private, max-age=60" {
		t.Errorf("Cache-Control = %q", cc)
	}
	for _, u := range got {
		if u != "/x/station_information.json" && u != "/x/station_status.json" {
			t.Errorf("sent %q", u)
		}
	}
}
