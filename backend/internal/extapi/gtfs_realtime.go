package extapi

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/jphacks/kz_2601/backend/internal/gtfsstatic"
)

// GTFS Realtime (#1692): bus arrival predictions and live positions from
// the GTFS-RT feeds of a closed list of operators (gtfsFeeds). Free, no key:
// one call is one `runs`.
//
//	nextBuses(feed, stop, route?, limit?)   Rows: the next buses at a stop (route / delay / "8:32(あと 12 分)")
//	busPositions(feed, route?, limit?)      Rows: one per bus (label / "route・HH:MM 更新" / "lat,lng")
//
// feed is an id of gtfsFeeds; stop and route are the feed's GTFS stop_id /
// route_id. Times come from the TripUpdate's absolute times. When a static
// GTFS-JP table (gtfsstatic) is given for the feed, stop and route names
// replace the ids and a prediction given only as a delay is shown as the
// timetable's time plus the delay; without one, ids stay and such a
// prediction is left out. Fetching and refreshing the static zip is not
// wired yet (production passes no table).
//
// The feed is protobuf (gtfs-realtime.proto); only the fields used here are
// read with protowire (google.golang.org/protobuf, already a dependency),
// so no generated bindings are added.
const (
	GTFSRealtime = "gtfsRealtime"

	gtfsMaxStops     = 10
	gtfsMaxPositions = 50
	gtfsMaxEntities  = 20000
	gtfsMaxRunes     = 64
	// gtfsStale: a feed or vehicle older than this is not shown (old
	// predictions mislead more than no answer).
	gtfsStale = 10 * time.Minute
	// gtfsAhead: predictions further ahead than this are left out.
	gtfsAhead = 3 * time.Hour
)

// gtfsFeed is one operator's realtime feed. A path is "" when the operator
// does not publish that kind.
type gtfsFeed struct {
	id, name string
	host     string // scheme://host, also the egress allow-list
	vehicles string // VehiclePosition path
	trips    string // TripUpdate path
	// credit is shown with the rows (CC BY: the provider and the license).
	credit string
}

// gtfsFeeds is the closed list (license from the data catalog; the URL and
// license must be checked once by a person before the provider is turned
// on: docs/backend/features/external-apis.md).
var gtfsFeeds = []gtfsFeed{
	{id: "toeiBus", name: "都営バス", host: "https://api-public.odpt.org",
		vehicles: "/api/v4/gtfs/realtime/ToeiBus",
		credit:   "東京都交通局・公共交通オープンデータ協議会、都営バス GTFS-RT、CC BY 4.0(https://creativecommons.org/licenses/by/4.0/deed.ja)"},
}

// GTFSAttribution is shown with the rows of every feed (with the feed's
// credit).
const GTFSAttribution = "事業者が公開しているデータです。正確・完全とは限りません。運行の情報は事業者の公式の案内を確かめてください"

var gtfsIDRe = regexp.MustCompile(`^[A-Za-z0-9_.:\-]{1,64}$`)

func init() {
	register(Provider{ID: GTFSRealtime, Billing: BillingFree, Hosts: gtfsHosts(),
		Ops: []OpSpec{
			{Name: "nextBuses", Args: []string{"feed", "stop", "route", "limit"}, Check: checkGTFSNext, CacheTTL: 30 * time.Second},
			{Name: "busPositions", Args: []string{"feed", "route", "limit"}, Check: checkGTFSPositions, CacheTTL: 30 * time.Second},
		},
		New: func(Secrets) (map[string]Operation, error) {
			return newGTFSRealtime(func(h string) string { return h }, NewHTTPClient(gtfsHosts(), DefaultTimeout), time.Now, nil), nil
		}})
}

// gtfsHosts are the feeds' hosts, each once.
func gtfsHosts() []string {
	var hs []string
	for _, f := range gtfsFeeds {
		if !slices.Contains(hs, f.host) {
			hs = append(hs, f.host)
		}
	}
	return hs
}

func gtfsFeedOf(id string) (gtfsFeed, bool) {
	i := slices.IndexFunc(gtfsFeeds, func(f gtfsFeed) bool { return f.id == id })
	if i < 0 {
		return gtfsFeed{}, false
	}
	return gtfsFeeds[i], true
}

// gtfsArgs reads feed / route (and stop when wantStop) and the limit.
func gtfsArgs(args map[string]string, trips bool, def, hi int) (f gtfsFeed, stop, route string, limit int, err error) {
	f, ok := gtfsFeedOf(strings.TrimSpace(args["feed"]))
	if !ok {
		return f, "", "", 0, fmt.Errorf("%w: feed must be a listed feed id", ErrUnknown)
	}
	if (trips && f.trips == "") || (!trips && f.vehicles == "") {
		return f, "", "", 0, fmt.Errorf("%w: feed %s has no data for this op", ErrUnknown, f.id)
	}
	if trips {
		if stop = strings.TrimSpace(args["stop"]); !gtfsIDRe.MatchString(stop) {
			return f, "", "", 0, fmt.Errorf("%w: stop must be a GTFS stop_id", ErrUnknown)
		}
	}
	if route = strings.TrimSpace(args["route"]); route != "" && !gtfsIDRe.MatchString(route) {
		return f, "", "", 0, fmt.Errorf("%w: route must be a GTFS route_id", ErrUnknown)
	}
	limit, err = argInt(args, "limit", def, 1, hi)
	return f, stop, route, limit, err
}

func checkGTFSNext(args map[string]string) error {
	_, _, _, _, err := gtfsArgs(args, true, 3, gtfsMaxStops)
	return err
}

func checkGTFSPositions(args map[string]string) error {
	_, _, _, _, err := gtfsArgs(args, false, 20, gtfsMaxPositions)
	return err
}

// newGTFSRealtime builds the ops; base maps a feed's host to the one to
// reach (itself, or a test server). static gives a feed's static table
// (static or its answer nil: ids only).
func newGTFSRealtime(base func(host string) string, hc *http.Client, now func() time.Time, static func(feed string) *gtfsstatic.Table) map[string]Operation {
	g := &gtfsRT{base: base, hc: hc, now: now, static: static}
	return map[string]Operation{
		"nextBuses":    OperationFunc(g.nextBuses),
		"busPositions": OperationFunc(g.busPositions),
	}
}

type gtfsRT struct {
	base   func(string) string
	hc     *http.Client
	now    func() time.Time
	static func(string) *gtfsstatic.Table
}

// table is the feed's static table (nil when there is none; its methods
// take nil).
func (g *gtfsRT) table(feed string) *gtfsstatic.Table {
	if g.static == nil {
		return nil
	}
	return g.static(feed)
}

// gtfsName is the static name, else the id.
func gtfsName(name, id string) string {
	if name != "" {
		return name
	}
	return id
}

// gtfsScheduled is a timetable time (seconds from the service day's start)
// as a moment. startDate is the TripDescriptor's YYYYMMDD; without a valid
// one the service day is today or yesterday in JST, whichever puts the
// time nearer to now (a trip past midnight runs on yesterday's day).
func gtfsScheduled(startDate string, sec int, now time.Time) time.Time {
	at := func(d time.Time) time.Time {
		y, m, dd := d.Date()
		// Japan has no daylight saving: "noon minus 12h" is midnight.
		return time.Date(y, m, dd, 0, 0, 0, 0, gtfsJST).Add(time.Duration(sec) * time.Second)
	}
	if d, err := time.ParseInLocation("20060102", startDate, gtfsJST); err == nil {
		return at(d)
	}
	today := now.In(gtfsJST)
	a, b := at(today), at(today.AddDate(0, 0, -1))
	if b.Sub(now).Abs() < a.Sub(now).Abs() {
		return b
	}
	return a
}

var gtfsJST = time.FixedZone("JST", 9*60*60)

// fetch reads and decodes one feed.
func (g *gtfsRT) fetch(ctx context.Context, f gtfsFeed, path string) (*gtfsMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.base(f.host)+path, nil)
	if err != nil {
		return nil, errors.New("gtfsRealtime: building the request failed")
	}
	req.Header.Set("Accept", "application/x-protobuf, application/octet-stream")
	resp, err := g.hc.Do(req)
	if err != nil {
		return nil, transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := ReadBody(resp.Body)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &ProviderError{Status: resp.StatusCode, Retryable: true, Err: errors.New("reading the answer failed")}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, statusError(resp.StatusCode)
	}
	m, err := parseGTFSFeed(body)
	if err != nil {
		return nil, &ProviderError{Status: resp.StatusCode, Err: err}
	}
	return m, nil
}

// fresh says whether a time (unix seconds; 0 = unknown, then the feed's)
// is recent enough to show.
func (g *gtfsRT) fresh(ts, feedTS uint64) bool {
	if ts == 0 {
		ts = feedTS
	}
	if ts == 0 || ts > math.MaxInt64 {
		return false
	}
	return g.now().Sub(time.Unix(int64(ts), 0)) <= gtfsStale
}

func (g *gtfsRT) nextBuses(ctx context.Context, in Input) (Output, error) {
	f, stop, route, limit, err := gtfsArgs(in.Args, true, 3, gtfsMaxStops)
	if err != nil {
		return Output{}, err
	}
	m, err := g.fetch(ctx, f, f.trips)
	if err != nil {
		return Output{}, err
	}
	if !g.fresh(0, m.timestamp) {
		return Output{Rows: []Row{}}, nil
	}
	now := g.now()
	tbl := g.table(f.id)
	stopName := tbl.StopName(stop)
	type next struct {
		at  time.Time
		row Row
	}
	var ns []next
	for _, tu := range m.trips {
		rid := tu.trip.routeID
		if rid == "" {
			rid = tbl.RouteOf(tu.trip.tripID)
		}
		if tu.trip.canceled || (route != "" && rid != route) {
			continue
		}
		for _, st := range tu.stops {
			if st.stopID != stop || st.skipped {
				continue
			}
			ev := st.arrival
			if ev.time == 0 && (st.departure.time != 0 || !ev.hasDelay) {
				ev = st.departure
			}
			delay, ok := ev.delay, ev.hasDelay
			if !ok {
				delay, ok = tu.delay, tu.hasDelay
			}
			var at time.Time
			sub := gtfsDelayText(delay, ok)
			switch sched, has := tbl.Scheduled(tu.trip.tripID, stop); {
			case ev.time != 0:
				at = time.Unix(ev.time, 0)
			case ok && has:
				// only a delay: the timetable's time plus the delay
				s := gtfsScheduled(tu.trip.startDate, sched, now)
				at = s.Add(time.Duration(delay) * time.Second)
				sub += "(時刻表 " + s.In(gtfsJST).Format("15:04") + ")"
			default:
				continue // a delay without a time or a timetable
			}
			if at.Before(now.Add(-time.Minute)) || at.After(now.Add(gtfsAhead)) {
				continue
			}
			title := gtfsName(tbl.RouteName(rid), rid)
			if title == "" {
				title = "系統不明"
			}
			if stopName != "" {
				sub = strings.TrimSuffix(stopName+"・"+sub, "・")
			}
			mins := max(0, int(math.Ceil(at.Sub(now).Minutes())))
			ns = append(ns, next{at, Row{Title: title, Subtitle: sub,
				Value: fmt.Sprintf("%s(あと %d 分)", at.In(gtfsJST).Format("15:04"), mins)}})
			break
		}
	}
	slices.SortStableFunc(ns, func(a, b next) int { return a.at.Compare(b.at) })
	rows := make([]Row, 0, min(len(ns), limit))
	for _, n := range ns[:min(len(ns), limit)] {
		rows = append(rows, n.row)
	}
	return Output{Rows: rows}, nil
}

// gtfsDelayText is the delay in whole minutes ("" when unknown).
func gtfsDelayText(sec int32, ok bool) string {
	if !ok {
		return ""
	}
	m := int(math.Round(float64(sec) / 60))
	switch {
	case m > 0:
		return fmt.Sprintf("遅れ %d 分", m)
	case m < 0:
		return fmt.Sprintf("%d 分早い", -m)
	}
	return "定刻"
}

func (g *gtfsRT) busPositions(ctx context.Context, in Input) (Output, error) {
	f, _, route, limit, err := gtfsArgs(in.Args, false, 20, gtfsMaxPositions)
	if err != nil {
		return Output{}, err
	}
	m, err := g.fetch(ctx, f, f.vehicles)
	if err != nil {
		return Output{}, err
	}
	tbl := g.table(f.id)
	type pos struct {
		route string
		row   Row
	}
	var ps []pos
	for _, v := range m.vehicles {
		rid := v.trip.routeID
		if rid == "" {
			rid = tbl.RouteOf(v.trip.tripID)
		}
		if !v.hasPos || (route != "" && rid != route) || !g.fresh(v.timestamp, m.timestamp) {
			continue
		}
		lat, lng := float64(v.lat), float64(v.lng)
		if math.IsNaN(lat) || math.IsNaN(lng) || lat < -90 || lat > 90 || lng < -180 || lng > 180 || (lat == 0 && lng == 0) {
			continue
		}
		title := v.label
		if title == "" {
			title = v.id
		}
		if title == "" {
			title = "バス"
		}
		ts := v.timestamp
		if ts == 0 {
			ts = m.timestamp
		}
		sub := time.Unix(int64(ts), 0).In(gtfsJST).Format("15:04") + " 更新"
		if rid != "" {
			sub = gtfsName(tbl.RouteName(rid), rid) + "・" + sub
		}
		ps = append(ps, pos{rid, Row{Title: title, Subtitle: sub, Value: gtfsCoord(v.lat) + "," + gtfsCoord(v.lng)}})
	}
	slices.SortStableFunc(ps, func(a, b pos) int {
		if c := strings.Compare(a.route, b.route); c != 0 {
			return c
		}
		return strings.Compare(a.row.Title, b.row.Title)
	})
	rows := make([]Row, 0, min(len(ps), limit))
	for _, p := range ps[:min(len(ps), limit)] {
		rows = append(rows, p.row)
	}
	return Output{Rows: rows}, nil
}

// gtfsCoord is a float32 degree in its shortest form (the feed has no more
// precision than float32).
func gtfsCoord(f float32) string { return strconv.FormatFloat(float64(f), 'f', -1, 32) }

// The parts of gtfs-realtime.proto read here (field numbers from the spec).
type gtfsMessage struct {
	timestamp uint64 // FeedHeader.timestamp
	trips     []gtfsTripUpdate
	vehicles  []gtfsVehicle
}

type gtfsTrip struct {
	tripID, routeID string
	startDate       string // YYYYMMDD, the service day ("" when not given)
	canceled        bool   // TripDescriptor.schedule_relationship CANCELED
}

type gtfsEvent struct {
	time     int64
	delay    int32
	hasDelay bool
}

type gtfsStopTime struct {
	stopID             string
	arrival, departure gtfsEvent
	skipped            bool // SKIPPED or NO_DATA
}

type gtfsTripUpdate struct {
	trip     gtfsTrip
	stops    []gtfsStopTime
	delay    int32
	hasDelay bool
}

type gtfsVehicle struct {
	trip      gtfsTrip
	id, label string
	lat, lng  float32
	hasPos    bool
	timestamp uint64
}

// gtfsText is a feed's string made valid UTF-8 and at most gtfsMaxRunes
// runes (ids and labels are short; a longer one is cut).
func gtfsText(b []byte) string {
	s := strings.TrimSpace(strings.ToValidUTF8(string(b), ""))
	if r := []rune(s); len(r) > gtfsMaxRunes {
		return string(r[:gtfsMaxRunes])
	}
	return s
}

var errGTFSBad = errors.New("the answer is not a GTFS-RT feed")

// pbFields walks one message's fields: v is a varint / fixed value, sub
// the bytes of a length-delimited field. Unknown fields are skipped.
func pbFields(b []byte, f func(num protowire.Number, typ protowire.Type, v uint64, sub []byte) error) error {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return errGTFSBad
		}
		b = b[n:]
		var v uint64
		var sub []byte
		switch typ {
		case protowire.VarintType:
			v, n = protowire.ConsumeVarint(b)
		case protowire.Fixed32Type:
			var x uint32
			x, n = protowire.ConsumeFixed32(b)
			v = uint64(x)
		case protowire.Fixed64Type:
			v, n = protowire.ConsumeFixed64(b)
		case protowire.BytesType:
			sub, n = protowire.ConsumeBytes(b)
		default:
			n = protowire.ConsumeFieldValue(num, typ, b)
		}
		if n < 0 {
			return errGTFSBad
		}
		b = b[n:]
		if err := f(num, typ, v, sub); err != nil {
			return err
		}
	}
	return nil
}

func parseGTFSFeed(b []byte) (*gtfsMessage, error) {
	m := &gtfsMessage{}
	entities := 0
	err := pbFields(b, func(num protowire.Number, typ protowire.Type, _ uint64, sub []byte) error {
		if typ != protowire.BytesType {
			return nil
		}
		switch num {
		case 1: // header
			return pbFields(sub, func(num protowire.Number, typ protowire.Type, v uint64, _ []byte) error {
				if num == 3 && typ == protowire.VarintType {
					m.timestamp = v
				}
				return nil
			})
		case 2: // entity
			if entities++; entities > gtfsMaxEntities {
				return errors.New("the feed has too many entities")
			}
			return parseGTFSEntity(sub, m)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return m, nil
}

func parseGTFSEntity(b []byte, m *gtfsMessage) error {
	var deleted bool
	var tu, vp []byte
	err := pbFields(b, func(num protowire.Number, typ protowire.Type, v uint64, sub []byte) error {
		switch {
		case num == 2 && typ == protowire.VarintType:
			deleted = v != 0
		case num == 3 && typ == protowire.BytesType:
			tu = sub
		case num == 4 && typ == protowire.BytesType:
			vp = sub
		}
		return nil
	})
	if err != nil || deleted {
		return err
	}
	if tu != nil {
		t, err := parseGTFSTripUpdate(tu)
		if err != nil {
			return err
		}
		m.trips = append(m.trips, t)
	}
	if vp != nil {
		v, err := parseGTFSVehicle(vp)
		if err != nil {
			return err
		}
		m.vehicles = append(m.vehicles, v)
	}
	return nil
}

func parseGTFSTrip(b []byte) (gtfsTrip, error) {
	var t gtfsTrip
	err := pbFields(b, func(num protowire.Number, typ protowire.Type, v uint64, sub []byte) error {
		switch {
		case num == 1 && typ == protowire.BytesType:
			t.tripID = gtfsText(sub)
		case num == 3 && typ == protowire.BytesType:
			t.startDate = gtfsText(sub)
		case num == 4 && typ == protowire.VarintType:
			t.canceled = v == 3 // CANCELED
		case num == 5 && typ == protowire.BytesType:
			t.routeID = gtfsText(sub)
		}
		return nil
	})
	return t, err
}

func parseGTFSEvent(b []byte) (gtfsEvent, error) {
	var e gtfsEvent
	err := pbFields(b, func(num protowire.Number, typ protowire.Type, v uint64, _ []byte) error {
		if typ != protowire.VarintType {
			return nil
		}
		switch num {
		case 1:
			e.delay, e.hasDelay = int32(v), true
		case 2:
			e.time = int64(v)
		}
		return nil
	})
	return e, err
}

func parseGTFSTripUpdate(b []byte) (gtfsTripUpdate, error) {
	var tu gtfsTripUpdate
	err := pbFields(b, func(num protowire.Number, typ protowire.Type, v uint64, sub []byte) error {
		var err error
		switch {
		case num == 1 && typ == protowire.BytesType:
			tu.trip, err = parseGTFSTrip(sub)
		case num == 2 && typ == protowire.BytesType:
			var st gtfsStopTime
			st, err = parseGTFSStopTime(sub)
			tu.stops = append(tu.stops, st)
		case num == 5 && typ == protowire.VarintType:
			tu.delay, tu.hasDelay = int32(v), true
		}
		return err
	})
	return tu, err
}

func parseGTFSStopTime(b []byte) (gtfsStopTime, error) {
	var st gtfsStopTime
	err := pbFields(b, func(num protowire.Number, typ protowire.Type, v uint64, sub []byte) error {
		var err error
		switch {
		case num == 2 && typ == protowire.BytesType:
			st.arrival, err = parseGTFSEvent(sub)
		case num == 3 && typ == protowire.BytesType:
			st.departure, err = parseGTFSEvent(sub)
		case num == 4 && typ == protowire.BytesType:
			st.stopID = gtfsText(sub)
		case num == 5 && typ == protowire.VarintType:
			st.skipped = v == 1 || v == 2 // SKIPPED / NO_DATA
		}
		return err
	})
	return st, err
}

func parseGTFSVehicle(b []byte) (gtfsVehicle, error) {
	var vp gtfsVehicle
	err := pbFields(b, func(num protowire.Number, typ protowire.Type, v uint64, sub []byte) error {
		var err error
		switch {
		case num == 1 && typ == protowire.BytesType:
			vp.trip, err = parseGTFSTrip(sub)
		case num == 2 && typ == protowire.BytesType:
			err = pbFields(sub, func(num protowire.Number, typ protowire.Type, v uint64, _ []byte) error {
				if typ != protowire.Fixed32Type {
					return nil
				}
				switch num {
				case 1:
					vp.lat = math.Float32frombits(uint32(v))
				case 2:
					vp.lng, vp.hasPos = math.Float32frombits(uint32(v)), true
				}
				return nil
			})
		case num == 5 && typ == protowire.VarintType:
			vp.timestamp = v
		case num == 8 && typ == protowire.BytesType:
			err = pbFields(sub, func(num protowire.Number, typ protowire.Type, _ uint64, s []byte) error {
				if typ == protowire.BytesType {
					switch num {
					case 1:
						vp.id = gtfsText(s)
					case 2:
						vp.label = gtfsText(s)
					}
				}
				return nil
			})
		}
		return err
	})
	return vp, err
}
