package extapi

import (
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/jphacks/kz_2601/backend/internal/gtfsstatic"
)

// gtfsNow is the fake clock: 2026-11-12 08:20 JST.
var gtfsNow = time.Date(2026, 11, 12, 8, 20, 0, 0, gtfsJST)

// pb builds a protobuf message for the fake feeds.
type pb []byte

func (b pb) str(n protowire.Number, s string) pb {
	return protowire.AppendString(protowire.AppendTag(b, n, protowire.BytesType), s)
}
func (b pb) msg(n protowire.Number, m pb) pb {
	return protowire.AppendBytes(protowire.AppendTag(b, n, protowire.BytesType), m)
}
func (b pb) varint(n protowire.Number, v uint64) pb {
	return protowire.AppendVarint(protowire.AppendTag(b, n, protowire.VarintType), v)
}
func (b pb) f32(n protowire.Number, f float32) pb {
	return protowire.AppendFixed32(protowire.AppendTag(b, n, protowire.Fixed32Type), math.Float32bits(f))
}

func gtfsAt(min int) uint64 { return uint64(gtfsNow.Add(time.Duration(min) * time.Minute).Unix()) }

func gtfsHeader(ts uint64) pb { return pb{}.msg(1, pb{}.str(1, "2.0").varint(3, ts)) }

// gtfsNoDelay marks a StopTimeUpdate without a delay.
const gtfsNoDelay = math.MinInt32

// gtfsStop is a StopTimeUpdate: arrival at min from now, delay in seconds
// (an int32 varint: a negative one is sign-extended).
func gtfsStop(stop string, min int, delay int32) pb {
	ev := pb{}.varint(2, gtfsAt(min))
	if delay != gtfsNoDelay {
		ev = ev.varint(1, uint64(int64(delay)))
	}
	return pb{}.str(4, stop).msg(2, ev)
}

func gtfsTripEntity(id, route string, canceled bool, stops ...pb) pb {
	trip := pb{}.str(1, "trip-"+id).str(5, route)
	if canceled {
		trip = trip.varint(4, 3)
	}
	tu := pb{}.msg(1, trip)
	for _, s := range stops {
		tu = tu.msg(2, s)
	}
	return pb{}.msg(2, pb{}.str(1, id).msg(3, tu))
}

func gtfsVehicleEntity(id, label, route string, lat, lng float32, ts uint64) pb {
	vp := pb{}.msg(1, pb{}.str(5, route)).msg(2, pb{}.f32(1, lat).f32(2, lng)).varint(5, ts).msg(8, pb{}.str(1, id).str(2, label))
	return pb{}.msg(2, pb{}.str(1, id).msg(4, vp))
}

// gtfsTripsFeed: three buses at stop S1 (one late, one canceled, one past),
// one bus at S2, an early bus on another route, a deleted entity.
func gtfsTripsFeed(ts uint64) pb {
	f := gtfsHeader(ts)
	f = append(f, gtfsTripEntity("a", "R1", false, gtfsStop("S0", 2, 0), gtfsStop("S1", 12, 180))...)
	f = append(f, gtfsTripEntity("b", "R1", false, gtfsStop("S1", 5, gtfsNoDelay))...)
	f = append(f, gtfsTripEntity("c", "R1", true, gtfsStop("S1", 7, 0))...)   // canceled
	f = append(f, gtfsTripEntity("d", "R1", false, gtfsStop("S1", -5, 0))...) // already gone
	f = append(f, gtfsTripEntity("e", "R2", false, gtfsStop("S1", 20, 0))...)
	f = append(f, gtfsTripEntity("f", "R1", false, gtfsStop("S2", 3, 0))...)
	f = append(f, gtfsTripEntity("g", "R3", false, gtfsStop("S1", 30, -120))...)
	f = append(f, gtfsTripEntity("h", "R1", false, gtfsStop("S1", 300, 0))...) // too far ahead
	// a deleted entity is skipped
	f = f.msg(2, pb{}.str(1, "x").varint(2, 1).msg(3, pb{}.msg(1, pb{}.str(5, "R1")).msg(2, gtfsStop("S1", 1, 0))))
	return f
}

func gtfsVehiclesFeed(ts uint64) pb {
	f := gtfsHeader(ts)
	f = append(f, gtfsVehicleEntity("v2", "品97", "R2", 35.68951, 139.69170, gtfsAt(-1))...)
	f = append(f, gtfsVehicleEntity("v1", "都01", "R1", 35.66500, 139.71200, gtfsAt(0))...)
	f = append(f, gtfsVehicleEntity("v3", "", "R1", 35.70000, 139.70000, gtfsAt(-30))...) // stale
	f = append(f, gtfsVehicleEntity("v4", "", "R1", 0, 0, gtfsAt(0))...)                  // no position
	return f
}

type gtfsFake struct {
	status atomic.Int64
	hits   atomic.Int64
	body   atomic.Value // pb
}

// gtfsTestFeeds puts a test feed with both kinds on the list.
func gtfsTestFeeds(t *testing.T) {
	t.Helper()
	old := gtfsFeeds
	gtfsFeeds = append(slices.Clone(old), gtfsFeed{id: "testBus", name: "試験バス", host: "https://gtfs.example.jp",
		vehicles: "/rt/vehicles.pb", trips: "/rt/trips.pb", credit: "試験"})
	t.Cleanup(func() { gtfsFeeds = old })
}

func gtfsOps(t *testing.T, fake *gtfsFake) map[string]Operation {
	t.Helper()
	return gtfsOpsWith(t, fake, nil)
}

// gtfsOpsWith also gives testBus a static table.
func gtfsOpsWith(t *testing.T, fake *gtfsFake, tbl *gtfsstatic.Table) map[string]Operation {
	t.Helper()
	fake.status.Store(200)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.hits.Add(1)
		if s := fake.status.Load(); s != 200 {
			w.WriteHeader(int(s))
			return
		}
		if r.URL.Path != "/rt/vehicles.pb" && r.URL.Path != "/rt/trips.pb" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(fake.body.Load().(pb))
	}))
	t.Cleanup(srv.Close)
	return newGTFSRealtime(func(string) string { return srv.URL }, srv.Client(), func() time.Time { return gtfsNow },
		func(feed string) *gtfsstatic.Table {
			if feed == "testBus" {
				return tbl
			}
			return nil
		})
}

func gtfsRows(t *testing.T, got, want []Row) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("rows %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestGTFSNextBuses(t *testing.T) {
	gtfsTestFeeds(t)
	var fake gtfsFake
	ops := gtfsOps(t, &fake)
	fake.body.Store(gtfsTripsFeed(gtfsAt(0)))
	out, err := ops["nextBuses"].Do(t.Context(), Input{Args: map[string]string{"feed": "testBus", "stop": "S1"}})
	if err != nil {
		t.Fatal(err)
	}
	// time order; canceled, gone, too far, deleted and other stops left out; 3 by default
	gtfsRows(t, out.Rows, []Row{
		{Title: "R1", Value: "08:25(あと 5 分)"},
		{Title: "R1", Subtitle: "遅れ 3 分", Value: "08:32(あと 12 分)"},
		{Title: "R2", Subtitle: "定刻", Value: "08:40(あと 20 分)"},
	})

	out, err = ops["nextBuses"].Do(t.Context(), Input{Args: map[string]string{"feed": "testBus", "stop": "S1", "limit": "10"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 4 || out.Rows[3] != (Row{Title: "R3", Subtitle: "2 分早い", Value: "08:50(あと 30 分)"}) {
		t.Fatalf("rows %+v", out.Rows)
	}

	out, err = ops["nextBuses"].Do(t.Context(), Input{Args: map[string]string{"feed": "testBus", "stop": "S1", "route": "R2"}})
	if err != nil || len(out.Rows) != 1 || out.Rows[0].Title != "R2" {
		t.Fatalf("route filter: %+v %v", out.Rows, err)
	}

	// an old feed is not shown
	fake.body.Store(gtfsTripsFeed(gtfsAt(-30)))
	out, err = ops["nextBuses"].Do(t.Context(), Input{Args: map[string]string{"feed": "testBus", "stop": "S1"}})
	if err != nil || len(out.Rows) != 0 {
		t.Fatalf("stale feed: %+v %v", out.Rows, err)
	}
}

func TestGTFSBusPositions(t *testing.T) {
	gtfsTestFeeds(t)
	var fake gtfsFake
	ops := gtfsOps(t, &fake)
	fake.body.Store(gtfsVehiclesFeed(gtfsAt(0)))
	out, err := ops["busPositions"].Do(t.Context(), Input{Args: map[string]string{"feed": "testBus"}})
	if err != nil {
		t.Fatal(err)
	}
	// route order; the stale bus and the one without a position left out
	gtfsRows(t, out.Rows, []Row{
		{Title: "都01", Subtitle: "R1・08:20 更新", Value: "35.665,139.712"},
		{Title: "品97", Subtitle: "R2・08:19 更新", Value: "35.68951,139.6917"},
	})
	out, err = ops["busPositions"].Do(t.Context(), Input{Args: map[string]string{"feed": "testBus", "route": "R2", "limit": "1"}})
	if err != nil || len(out.Rows) != 1 || out.Rows[0].Title != "品97" {
		t.Fatalf("route filter: %+v %v", out.Rows, err)
	}
}

// gtfsTestTable: names for S1 / R1 / R2, trip-k (route only in the
// timetable) at S1 at 08:30, trip-m at 08:50, trip-n past midnight.
func gtfsTestTable() *gtfsstatic.Table {
	return &gtfsstatic.Table{
		Stops:     map[string]string{"S1": "香林坊"},
		Routes:    map[string]string{"R1": "10", "R2": "兼六園シャトル"},
		TripRoute: map[string]string{"trip-k": "R2", "trip-m": "R1"},
		Times: map[gtfsstatic.Key]int{
			{Trip: "trip-k", Stop: "S1"}: 8*3600 + 30*60,
			{Trip: "trip-m", Stop: "S1"}: 8*3600 + 50*60,
		},
	}
}

// gtfsDelayOnlyEntity is a trip whose stop update has only a delay (stop
// delay, else the trip's), with no route_id in the feed.
func gtfsDelayOnlyEntity(id, startDate string, stopDelay, tripDelay int32) pb {
	trip := pb{}.str(1, id)
	if startDate != "" {
		trip = trip.str(3, startDate)
	}
	ev := pb{}
	if stopDelay != gtfsNoDelay {
		ev = ev.varint(1, uint64(int64(stopDelay)))
	}
	tu := pb{}.msg(1, trip).msg(2, pb{}.str(4, "S1").msg(2, ev))
	if tripDelay != gtfsNoDelay {
		tu = tu.varint(5, uint64(int64(tripDelay)))
	}
	return pb{}.msg(2, pb{}.str(1, id).msg(3, tu))
}

func TestGTFSNextBusesWithStatic(t *testing.T) {
	gtfsTestFeeds(t)
	var fake gtfsFake
	ops := gtfsOpsWith(t, &fake, gtfsTestTable())
	f := gtfsHeader(gtfsAt(0))
	f = append(f, gtfsTripEntity("a", "R1", false, gtfsStop("S1", 12, 180))...)
	f = append(f, gtfsDelayOnlyEntity("trip-k", "20261112", 120, gtfsNoDelay)...)
	f = append(f, gtfsDelayOnlyEntity("trip-m", "", gtfsNoDelay, 60)...)
	f = append(f, gtfsDelayOnlyEntity("trip-x", "", 60, gtfsNoDelay)...)        // not in the timetable
	f = append(f, gtfsDelayOnlyEntity("trip-k", "20261111", 0, gtfsNoDelay)...) // yesterday's run: gone
	fake.body.Store(f)
	out, err := ops["nextBuses"].Do(t.Context(), Input{Args: map[string]string{"feed": "testBus", "stop": "S1", "limit": "10"}})
	if err != nil {
		t.Fatal(err)
	}
	gtfsRows(t, out.Rows, []Row{
		{Title: "10", Subtitle: "香林坊・遅れ 3 分", Value: "08:32(あと 12 分)"},
		{Title: "兼六園シャトル", Subtitle: "香林坊・遅れ 2 分(時刻表 08:30)", Value: "08:32(あと 12 分)"},
		{Title: "10", Subtitle: "香林坊・遅れ 1 分(時刻表 08:50)", Value: "08:51(あと 31 分)"},
	})
	// the route filter uses the timetable's route when the feed has none
	out, err = ops["nextBuses"].Do(t.Context(), Input{Args: map[string]string{"feed": "testBus", "stop": "S1", "route": "R2"}})
	if err != nil || len(out.Rows) != 1 || out.Rows[0].Title != "兼六園シャトル" {
		t.Fatalf("route filter: %+v %v", out.Rows, err)
	}
}

func TestGTFSBusPositionsWithStatic(t *testing.T) {
	gtfsTestFeeds(t)
	var fake gtfsFake
	ops := gtfsOpsWith(t, &fake, gtfsTestTable())
	fake.body.Store(gtfsVehiclesFeed(gtfsAt(0)))
	out, err := ops["busPositions"].Do(t.Context(), Input{Args: map[string]string{"feed": "testBus"}})
	if err != nil {
		t.Fatal(err)
	}
	gtfsRows(t, out.Rows, []Row{
		{Title: "都01", Subtitle: "10・08:20 更新", Value: "35.665,139.712"},
		{Title: "品97", Subtitle: "兼六園シャトル・08:19 更新", Value: "35.68951,139.6917"},
	})
}

func TestGTFSScheduled(t *testing.T) {
	for _, c := range []struct {
		date string
		sec  int
		now  time.Time
		want time.Time
	}{
		{"20261112", 8*3600 + 30*60, gtfsNow, time.Date(2026, 11, 12, 8, 30, 0, 0, gtfsJST)},
		{"", 8*3600 + 30*60, gtfsNow, time.Date(2026, 11, 12, 8, 30, 0, 0, gtfsJST)},
		// past midnight: 24:30 of the 11th, not of the 12th
		{"", 24*3600 + 30*60, time.Date(2026, 11, 12, 0, 20, 0, 0, gtfsJST), time.Date(2026, 11, 12, 0, 30, 0, 0, gtfsJST)},
		{"20261111", 24*3600 + 30*60, gtfsNow, time.Date(2026, 11, 12, 0, 30, 0, 0, gtfsJST)},
		{"bad", 8 * 3600, gtfsNow, time.Date(2026, 11, 12, 8, 0, 0, 0, gtfsJST)},
	} {
		if got := gtfsScheduled(c.date, c.sec, c.now); !got.Equal(c.want) {
			t.Fatalf("%q %d: %v, want %v", c.date, c.sec, got, c.want)
		}
	}
}

func TestGTFSRefusesBadArgs(t *testing.T) {
	gtfsTestFeeds(t)
	var fake gtfsFake
	ops := gtfsOps(t, &fake)
	p, _ := Lookup(GTFSRealtime)
	for _, c := range []struct {
		op   string
		args map[string]string
	}{
		{"nextBuses", map[string]string{"stop": "S1"}},                                        // no feed
		{"nextBuses", map[string]string{"feed": "nope", "stop": "S1"}},                        // not listed
		{"nextBuses", map[string]string{"feed": "testBus"}},                                   // no stop
		{"nextBuses", map[string]string{"feed": "testBus", "stop": "S1/../x"}},                // not an id
		{"nextBuses", map[string]string{"feed": "testBus", "stop": "S1", "limit": "11"}},      // over the limit
		{"nextBuses", map[string]string{"feed": "testBus", "stop": "S1", "route": "a b"}},     // not an id
		{"nextBuses", map[string]string{"feed": "toeiBus", "stop": "S1"}},                     // no TripUpdate feed
		{"busPositions", map[string]string{"feed": "testBus", "limit": "0"}},                  // under the limit
		{"busPositions", map[string]string{"feed": "https://gtfs.example.jp/rt/vehicles.pb"}}, // a URL
	} {
		if _, err := ops[c.op].Do(t.Context(), Input{Args: c.args}); Reason(err) != ReasonBadRequest {
			t.Fatalf("%s %v: %s (%v)", c.op, c.args, Reason(err), err)
		}
		if _, err := p.CheckArgs(c.op, c.args); err == nil {
			t.Fatalf("CheckArgs accepted %s %v", c.op, c.args)
		}
	}
	if _, err := p.CheckArgs("busPositions", map[string]string{"feed": "testBus", "stop": "S1"}); err == nil {
		t.Fatal("unknown argument accepted")
	}
	if fake.hits.Load() != 0 {
		t.Fatal("a bad argument reached the feed")
	}
}

func TestGTFSUpstreamErrors(t *testing.T) {
	gtfsTestFeeds(t)
	var fake gtfsFake
	ops := gtfsOps(t, &fake)
	args := map[string]string{"feed": "testBus"}
	for _, s := range []int64{403, 404, 429, 503} {
		fake.status.Store(s)
		if _, err := ops["busPositions"].Do(t.Context(), Input{Args: args}); Reason(err) != ReasonUpstream {
			t.Fatalf("%d: %s %v", s, Reason(err), err)
		}
	}
	fake.status.Store(200)
	for _, body := range []pb{pb("<html>"), {0x0a, 0xff}} {
		fake.body.Store(body)
		if _, err := ops["busPositions"].Do(t.Context(), Input{Args: args}); Reason(err) != ReasonUpstream {
			t.Fatalf("%q: %s %v", body, Reason(err), err)
		}
	}
}

func TestGTFSText(t *testing.T) {
	if got := gtfsText([]byte(" a\xffb ")); got != "ab" {
		t.Fatalf("%q", got)
	}
	if got := gtfsText([]byte(strings.Repeat("あ", gtfsMaxRunes+3))); len([]rune(got)) != gtfsMaxRunes {
		t.Fatalf("%q", got)
	}
}

// The listed feeds are well formed and their hosts are the egress list.
func TestGTFSFeedList(t *testing.T) {
	p, ok := Lookup(GTFSRealtime)
	if !ok {
		t.Fatal("not registered")
	}
	seen := map[string]bool{}
	for _, f := range gtfsFeeds {
		if !gtfsIDRe.MatchString(f.id) || seen[f.id] || f.name == "" || f.credit == "" || (f.vehicles == "" && f.trips == "") {
			t.Fatalf("feed %+v", f)
		}
		if !strings.HasPrefix(f.host, "https://") || strings.Count(f.host, "/") != 2 {
			t.Fatalf("feed %s host %q", f.id, f.host)
		}
		if !strings.HasPrefix(f.vehicles+f.trips, "/") {
			t.Fatalf("feed %s paths", f.id)
		}
		seen[f.id] = true
	}
	if len(p.Hosts) != len(gtfsHosts()) {
		t.Fatalf("hosts %v", p.Hosts)
	}
}
