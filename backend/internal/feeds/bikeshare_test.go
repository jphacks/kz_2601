package feeds

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Fixtures are real responses recorded on 2026-11-12 02:28 (JST), trimmed by
// hand to the stations nearest a point plus one about 3 km away:
//   - gbfs_docomo-cycle-tokyo_station_*.json: https://api-public.odpt.org/api/v4/gbfs/docomo-cycle-tokyo/
//     6 stations near 東京駅 (35.681, 139.767) + C3-36 (2.8 km)
//   - gbfs_hellocycling_station_*.json: https://api-public.odpt.org/api/v4/gbfs/hellocycling/
//     4 stations near 新御徒町 (35.707, 139.778) + リパーク東浅草 (2.8 km)

// gbfsSource is a fake ODPT GBFS API; body overrides a path.
type gbfsSource struct {
	srv   *httptest.Server
	mu    sync.Mutex
	hits  map[string]int
	query []string // raw queries seen (the point must never be sent)
	body  map[string][]byte
}

func newGBFSSource(t *testing.T) *gbfsSource {
	g := &gbfsSource{hits: map[string]int{}, body: map[string][]byte{}}
	for _, sys := range []string{"docomo-cycle-tokyo", "hellocycling"} {
		for _, f := range []string{"station_information", "station_status"} {
			g.body["/"+sys+"/"+f+".json"] = fixture(t, "gbfs_"+sys+"_"+f+".json")
		}
	}
	g.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.hits[r.URL.Path]++
		g.query = append(g.query, r.URL.RawQuery)
		b, ok := g.body[r.URL.Path]
		g.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	}))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *gbfsSource) set(path string, b []byte) {
	g.mu.Lock()
	g.body[path] = b
	g.mu.Unlock()
}

func (g *gbfsSource) count(path string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.hits[path]
}

func (g *gbfsSource) svc(c *clock, o Options) *Service {
	o.Client, o.Now = g.srv.Client(), c.now
	sys := func(id, name string) BikeshareSystem {
		b := odptGBFS(id, name, "事業者", "c_"+id, "CC BY 4.0")
		b.Info, b.Status = g.srv.URL+"/"+id+"/station_information.json", g.srv.URL+"/"+id+"/station_status.json"
		return b
	}
	o.Endpoints = &Endpoints{Bikeshare: map[string]BikeshareSystem{
		"docomo-cycle-tokyo": sys("docomo-cycle-tokyo", "ドコモ・バイクシェア(東京)"),
		"hellocycling":       sys("hellocycling", "HELLO CYCLING"),
	}}
	return New(o)
}

func bikeRows(items []Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Date+" | "+it.Title+" | "+it.Subtitle)
	}
	return out
}

func TestBikeshareNearest(t *testing.T) {
	g := newGBFSSource(t)
	c := &clock{t: testNow}
	s := g.svc(c, Options{})
	ctx := context.Background()
	q := Query{Provider: dsl.FeedBikeshare, FeedID: "docomo-cycle-tokyo", Q: "35.681,139.767"}
	res, err := s.Get(ctx, q, 5)
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "ドコモ・バイクシェア(東京)の近くのポート" || res.Source.Name != "コンテンツ等の提供者名: 事業者 / 公共交通オープンデータ協議会" ||
		res.Source.URL != "https://ckan.odpt.org/dataset/c_docomo-cycle-tokyo" || !strings.HasPrefix(res.Source.License, "CC BY 4.0") {
		t.Errorf("title/source = %q %+v", res.Title, res.Source)
	}
	const at = "2026-11-12T02:27:44+09:00"
	want := []string{
		at + " | B2-33.新槇町ビル | 自転車 0 台・返せる空き 109・約 250 m",
		at + " | A4-01.丸ビル（南側駐輪場） | 自転車 0 台・返せる空き 13・約 300 m",
		at + " | B2-21.八重洲二丁目北地区広場 | 自転車 0 台・返せる空き 22・約 350 m",
		at + " | A4-08.永代通り（大手町駅B2c出口前） | 自転車 0 台・返せる空き 0・約 350 m",
		at + " | A4-11.二重橋前駅(メトロ4番出口) | 自転車 0 台・返せる空き 0・約 450 m",
	}
	if got := bikeRows(res.Items); !slices.Equal(got, want) {
		t.Fatalf("rows =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// Another point in the same system is served from the one cached copy.
	res, err = s.Get(ctx, Query{Provider: dsl.FeedBikeshare, FeedID: "docomo-cycle-tokyo", Q: "35.6775, 139.7698"}, 1)
	if err != nil || len(res.Items) != 1 || res.Items[0].Title != "B2-11.京橋エドグラン" {
		t.Fatalf("second point = %+v, %v", res, err)
	}
	if res.Items[0].Subtitle != "自転車 0 台・返せる空き 12・約 50 m" {
		t.Errorf("subtitle = %q", res.Items[0].Subtitle)
	}
	info, status := "/docomo-cycle-tokyo/station_information.json", "/docomo-cycle-tokyo/station_status.json"
	if g.count(info) != 1 || g.count(status) != 1 {
		t.Errorf("hits info %d status %d, want 1 1", g.count(info), g.count(status))
	}
	// After the TTL only station_status is read again; after gbfsInfoTTL both.
	c.add(providerLimits[dsl.FeedBikeshare].ttl + time.Second)
	if _, err := s.Get(ctx, q, 5); err != nil {
		t.Fatal(err)
	}
	if g.count(info) != 1 || g.count(status) != 2 {
		t.Errorf("after ttl: info %d status %d, want 1 2", g.count(info), g.count(status))
	}
	c.add(gbfsInfoTTL)
	if _, err := s.Get(ctx, q, 5); err != nil {
		t.Fatal(err)
	}
	if g.count(info) != 2 || g.count(status) != 3 {
		t.Errorf("after info ttl: info %d status %d, want 2 3", g.count(info), g.count(status))
	}
	for _, raw := range g.query {
		if raw != "" {
			t.Errorf("a query was sent to the source: %q", raw)
		}
	}
}

func TestBikeshareStoppedAndFar(t *testing.T) {
	g := newGBFSSource(t)
	s := g.svc(&clock{t: testNow}, Options{})
	ctx := context.Background()
	res, err := s.Get(ctx, Query{Provider: dsl.FeedBikeshare, FeedID: "hellocycling", Q: "35.707,139.778"}, 20)
	if err != nil {
		t.Fatal(err)
	}
	// The station 2.8 km away is left out; one that stopped renting says so.
	want := []string{
		"2026-11-12T02:25:07+09:00 | ファミリーマート 台東四丁目店 | 自転車 3 台・返せる空き 2・約 50 m",
		"2026-11-12T02:25:03+09:00 | 新御徒町ステーション | 自転車 3 台・返せる空き 5・約 50 m",
		"2026-11-12T02:25:07+09:00 | 上野御徒町プレイス（旧山口ビル7・ヴィラルヴェロ） | 自転車 4 台・返せる空き 2・約 100 m",
		"2026-11-12T02:25:07+09:00 | 仲御徒町駅自転車駐車場 | 貸出停止中・約 200 m",
	}
	if got := bikeRows(res.Items); !slices.Equal(got, want) {
		t.Fatalf("rows =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// Nothing within the radius: an empty list, not an error.
	res, err = s.Get(ctx, Query{Provider: dsl.FeedBikeshare, FeedID: "hellocycling", Q: "43.062,141.354"}, 5)
	if err != nil || len(res.Items) != 0 || res.Items == nil {
		t.Errorf("far away = %+v, %v", res, err)
	}
}

func TestBikeshareOddValues(t *testing.T) {
	g := newGBFSSource(t)
	g.set("/hellocycling/station_information.json", []byte(`{"data":{"stations":[
 {"station_id":1,"name":"数の id","lat":35.0,"lon":139.0},
 {"station_id":"2","name":"撤去済み","lat":35.0001,"lon":139.0},
 {"station_id":"3","name":"返却だけ停止","lat":35.0002,"lon":139.0},
 {"station_id":"4","name":"位置なし"},
 {"station_id":"5","name":"状態なし","lat":35.0,"lon":139.0},
 {"station_id":"6","name":"`+"\\u202e"+`名前\n改行","lat":35.0003,"lon":139.0}]}}`))
	g.set("/hellocycling/station_status.json", []byte(`{"data":{"stations":[
 {"station_id":"1","num_bikes_available":2,"num_docks_available":1,"is_renting":true,"is_returning":true},
 {"station_id":"2","num_bikes_available":2,"is_installed":false},
 {"station_id":"3","num_bikes_available":-1,"num_docks_available":0,"is_returning":false,"last_reported":0},
 {"station_id":"4","num_bikes_available":5},
 {"station_id":"6","num_bikes_available":1},
 {"station_id":"99","num_bikes_available":9}]}}`))
	s := g.svc(&clock{t: testNow}, Options{})
	res, err := s.Get(context.Background(), Query{Provider: dsl.FeedBikeshare, FeedID: "hellocycling", Q: "35,139"}, 20)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		" | 数の id | 自転車 2 台・返せる空き 1・約 50 m",
		" | 返却だけ停止 | 自転車 0 台・返せる空き 0・返却停止中・約 50 m",
		" | 名前 改行 | 自転車 1 台・約 50 m",
	}
	if got := bikeRows(res.Items); !slices.Equal(got, want) {
		t.Fatalf("rows =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestBikeshareRejects(t *testing.T) {
	g := newGBFSSource(t)
	c := &clock{t: testNow}
	s := g.svc(c, Options{})
	ctx := context.Background()
	for _, q := range []Query{
		{Provider: dsl.FeedBikeshare, FeedID: "hellocycling"},
		{Provider: dsl.FeedBikeshare, FeedID: "hellocycling", Q: "新御徒町"},
		{Provider: dsl.FeedBikeshare, FeedID: "hellocycling", Q: "0,0"},
		{Provider: dsl.FeedBikeshare, FeedID: "hellocycling", Q: "139.778,35.707"},
		{Provider: dsl.FeedBikeshare, FeedID: "hellocycling", Q: "3.5e1,139.7"},
		{Provider: dsl.FeedBikeshare, FeedID: "hellocycling", Q: "35.7,139.7,1"},
		{Provider: dsl.FeedBikeshare, FeedID: "hellocycling", Q: "35.7,139.7", Region: "130000"},
		{Provider: dsl.FeedBikeshare, FeedID: "openstreet", Q: "35.7,139.7"},
		{Provider: dsl.FeedBikeshare, FeedID: "https://api-public.odpt.org/api/v4/gbfs/x/station_status.json", Q: "35.7,139.7"},
		{Provider: dsl.FeedBikeshare, Q: "35.7,139.7"},
	} {
		if _, err := s.Get(ctx, q, 5); !errors.Is(err, ErrUnknown) {
			t.Errorf("%+v: err = %v, want ErrUnknown", q, err)
		}
	}
	if n := g.count("/hellocycling/station_information.json"); n != 0 {
		t.Errorf("source hits = %d", n)
	}
	// A broken status fails, is remembered for FailTTL, and is retried after it.
	g.set("/docomo-cycle-tokyo/station_status.json", []byte(`<html>maintenance</html>`))
	q := Query{Provider: dsl.FeedBikeshare, FeedID: "docomo-cycle-tokyo", Q: "35.681,139.767"}
	if _, err := s.Get(ctx, q, 5); !errors.Is(err, ErrUpstream) {
		t.Errorf("html: err = %v", err)
	}
	if _, err := s.Get(ctx, q, 5); !errors.Is(err, ErrRecentFailure) {
		t.Errorf("cached failure: err = %v", err)
	}
	g.set("/docomo-cycle-tokyo/station_status.json", []byte(`{"data":{"stations":[]}}`))
	c.add(FailTTL + time.Second)
	if _, err := s.Get(ctx, q, 5); !errors.Is(err, ErrUpstream) {
		t.Errorf("empty: err = %v", err)
	}
	// station_information was read once and kept although status failed.
	if n := g.count("/docomo-cycle-tokyo/station_information.json"); n != 1 {
		t.Errorf("info hits = %d, want 1", n)
	}
	// Over the body cap.
	g.set("/hellocycling/station_information.json", []byte(`{"x":"`+strings.Repeat("a", int(providerLimits[dsl.FeedBikeshare].maxBody))+`"}`))
	if _, err := s.Get(ctx, Query{Provider: dsl.FeedBikeshare, FeedID: "hellocycling", Q: "35.7,139.7"}, 5); !errors.Is(err, ErrUpstream) {
		t.Errorf("too large: err = %v", err)
	}
}

func TestBikeshareDisabled(t *testing.T) {
	g := newGBFSSource(t)
	s := g.svc(&clock{t: testNow}, OptionsFromEnv(func(k string) string {
		return map[string]string{"FEEDS_DISABLED": "bikeshare"}[k]
	}))
	if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedBikeshare, FeedID: "hellocycling", Q: "35.7,139.7"}, 5); !errors.Is(err, ErrDisabled) {
		t.Errorf("err = %v, want ErrDisabled", err)
	}
}

// The point is not a search word: it neither counts against the caller's
// hourly source fetches nor splits the cache.
func TestBikesharePointNotCounted(t *testing.T) {
	g := newGBFSSource(t)
	s := g.svc(&clock{t: testNow}, Options{UserMissesPerHour: 1, IPMissesPerHour: 1})
	ctx := context.Background()
	for i, p := range []string{"35.681,139.767", "35.68,139.76", "35.677,139.77"} {
		q := Query{Provider: dsl.FeedBikeshare, FeedID: "docomo-cycle-tokyo", Q: p, UID: "u1", IP: "ip1"}
		if _, err := s.Get(ctx, q, 5); err != nil {
			t.Errorf("%d: %v", i, err)
		}
	}
	if n := g.count("/docomo-cycle-tokyo/station_status.json"); n != 1 {
		t.Errorf("status hits = %d, want 1", n)
	}
}

func TestRoundDistance(t *testing.T) {
	for m, want := range map[float64]string{0: "50 m", 24: "50 m", 76: "100 m", 974: "950 m", 975: "1.0 km", 1449: "1.4 km", 1950: "2.0 km"} {
		if got := roundDistance(m); got != want {
			t.Errorf("roundDistance(%v) = %q, want %q", m, got, want)
		}
	}
}

// The bikeshare allowlist matches the ids the DSL accepts, and every system
// is the fixed pair of files on ODPT's public GBFS API.
func TestBikeshareAllowlistMatchesDSL(t *testing.T) {
	ep := DefaultEndpoints()
	var ids []string
	for id, b := range ep.Bikeshare {
		ids = append(ids, id)
		if dsl.BikeshareSystems[id] != b.Name {
			t.Errorf("bikeshare %s name %q != dsl %q", id, b.Name, dsl.BikeshareSystems[id])
		}
		base := "https://api-public.odpt.org/api/v4/gbfs/" + id + "/"
		if b.Info != base+"station_information.json" || b.Status != base+"station_status.json" ||
			!ep.allowedURL(b.Info) || !strings.HasPrefix(b.DatasetURL, "https://ckan.odpt.org/dataset/") ||
			!strings.HasSuffix(b.Credit, " / 公共交通オープンデータ協議会") || !strings.HasPrefix(b.License, "CC BY 4.0") {
			t.Errorf("bikeshare %s: %+v", id, b)
		}
	}
	// The credit and change note are the form ODPT's FAQ asks for.
	for id, want := range map[string][2]string{
		"docomo-cycle-tokyo": {"コンテンツ等の提供者名: 株式会社ドコモ・バイクシェア / 公共交通オープンデータ協議会",
			"CC BY 4.0 (https://creativecommons.org/licenses/by/4.0/)・近くのポートを抜き出して近い順に並べ替えて表示"},
		"hellocycling": {"コンテンツ等の提供者名: OpenStreet株式会社 / 公共交通オープンデータ協議会",
			"CC BY 4.0 (https://creativecommons.org/licenses/by/4.0/)(3 つのライセンスから選択)・近くのポートを抜き出して近い順に並べ替えて表示"},
	} {
		if b := ep.Bikeshare[id]; b.Credit != want[0] || b.License != want[1] {
			t.Errorf("bikeshare %s credit/license = %q %q", id, b.Credit, b.License)
		}
	}
	slices.Sort(ids)
	if want := slices.Sorted(maps.Keys(dsl.BikeshareSystems)); !slices.Equal(ids, want) {
		t.Errorf("bikeshare ids %v != dsl %v", ids, want)
	}
}
