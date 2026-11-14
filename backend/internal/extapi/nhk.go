package extapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// NHK 番組表 API v2: the programs of a day and what is on air now, read with
// the operator's key (api.nhk.or.jp/v2/pg). Free: a call is one `runs`.
//
//	programs(area?, service?, date?, limit?)  /pg/list/{area}/{service}/{date}.json: Rows (title / HH:MM–HH:MM · subtitle / service name)
//	now(area?, service?)                       /pg/now/{area}/{service}.json: Item = the present program, Rows = previous / present / following
//
// area is the 3-digit area code (default 130 = 東京), service one of
// nhkServices (default g1 = 総合), date YYYY-MM-DD in JST from yesterday to
// 7 days ahead (default today). Times are JST (the API's own zone).
//
// Terms (NHK 番組表 API 利用規約): the key is allowed 300 requests a day,
// so the provider's environment-wide default is that many
// (nhkGlobalDaily; EXT_GLOBAL_DAILY_NHK wins) and an answer is kept 10
// minutes. The program information is shown as it is, with
// NHKAttribution. The list answer carries no program page URL, so rows have
// no link.
//
// The key (EXT_NHK_API_KEY, Secret Manager) goes in the query as the API
// wants; errors never carry the URL (keyed.go).
const (
	NHK = "nhk"
	// NHKAttribution is the source shown with the rows.
	NHKAttribution = "番組情報: NHK番組表API"

	nhkHost        = "https://api.nhk.or.jp"
	nhkGlobalDaily = 300
	nhkCacheTTL    = 10 * time.Minute

	nhkDefaultArea    = "130"
	nhkDefaultService = "g1"
	nhkListDefault    = 20
	nhkListMax        = 100
	nhkMaxRunes       = 120
	nhkDaysAhead      = 7
)

// nhkServices are the service ids of the API (TV, radio and the bundles).
var nhkServices = []string{"g1", "g2", "e1", "e3", "s1", "s3", "s5", "s6", "r1", "r2", "r3", "tv", "radio", "netradio"}

var (
	nhkKeyRe  = regexp.MustCompile(`^[A-Za-z0-9]{16,64}$`)
	nhkAreaRe = regexp.MustCompile(`^[0-9]{3}$`)
	nhkDateRe = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
)

func init() {
	register(Provider{ID: NHK, Billing: BillingFree, Hosts: []string{nhkHost}, GlobalDaily: nhkGlobalDaily,
		Ops: []OpSpec{
			{Name: "programs", Args: []string{"area", "service", "date", "limit"}, Check: checkNHKPrograms, CacheTTL: nhkCacheTTL},
			{Name: "now", Args: []string{"area", "service"}, Check: checkNHKNow, CacheTTL: nhkCacheTTL},
		},
		New: func(sec Secrets) (map[string]Operation, error) {
			return newNHK(sec.APIKey, nhkHost, NewHTTPClient([]string{nhkHost}, DefaultTimeout), time.Now)
		}})
}

// argNHKAreaService reads area / service (their defaults when empty).
func argNHKAreaService(args map[string]string) (area, service string, err error) {
	area = strings.TrimSpace(args["area"])
	if area == "" {
		area = nhkDefaultArea
	}
	if !nhkAreaRe.MatchString(area) {
		return "", "", fmt.Errorf("%w: area must be a 3-digit area code", ErrBadArgs)
	}
	service = strings.ToLower(strings.TrimSpace(args["service"]))
	if service == "" {
		service = nhkDefaultService
	}
	if !slices.Contains(nhkServices, service) {
		return "", "", fmt.Errorf("%w: service must be one of %s", ErrBadArgs, strings.Join(nhkServices, ", "))
	}
	return area, service, nil
}

// argNHKDate reads date (today in JST when empty): yesterday to 7 days ahead.
func argNHKDate(args map[string]string, now time.Time) (string, error) {
	today := now.In(jst).Truncate(24 * time.Hour)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, jst)
	s := strings.TrimSpace(args["date"])
	if s == "" {
		return today.Format("2006-01-02"), nil
	}
	d, err := time.ParseInLocation("2006-01-02", s, jst)
	if !nhkDateRe.MatchString(s) || err != nil || d.Before(today.AddDate(0, 0, -1)) || d.After(today.AddDate(0, 0, nhkDaysAhead)) {
		return "", fmt.Errorf("%w: date must be YYYY-MM-DD from yesterday to %d days ahead", ErrBadArgs, nhkDaysAhead)
	}
	return s, nil
}

func checkNHKPrograms(args map[string]string) error {
	if _, _, err := argNHKAreaService(args); err != nil {
		return err
	}
	if _, err := argNHKDate(args, time.Now()); err != nil {
		return err
	}
	_, err := argInt(args, "limit", nhkListDefault, 1, nhkListMax)
	return err
}

func checkNHKNow(args map[string]string) error {
	_, _, err := argNHKAreaService(args)
	return err
}

// newNHK builds the ops against base (the API, or a test server); now is
// the clock of the date's default and range.
func newNHK(key, base string, hc *http.Client, now func() time.Time) (map[string]Operation, error) {
	if !nhkKeyRe.MatchString(key) {
		return nil, errors.New("EXT_NHK_API_KEY is not set or not a key")
	}
	n := &nhk{key: key, base: base, hc: hc, now: now}
	return map[string]Operation{
		"programs": OperationFunc(n.programs),
		"now":      OperationFunc(n.nowOnAir),
	}, nil
}

type nhk struct {
	key  string
	base string
	hc   *http.Client
	now  func() time.Time
}

// nhkProgram is one program of the list and the now answers.
type nhkProgram struct {
	ID        string `json:"id"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Title     string `json:"title"`
	Subtitle  string `json:"subtitle"`
	Service   struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"service"`
}

type nhkListAnswer struct {
	// List is service id → programs (several services for tv / radio).
	List map[string][]nhkProgram `json:"list"`
}

type nhkNowAnswer struct {
	List map[string]struct {
		Previous  *nhkProgram `json:"previous"`
		Present   *nhkProgram `json:"present"`
		Following *nhkProgram `json:"following"`
	} `json:"nowonair_list"`
}

// get calls path with the key.
func (n *nhk) get(ctx context.Context, path string, out any) error {
	q := url.Values{"key": {n.key}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.base+path+"?"+q.Encode(), nil)
	if err != nil {
		return errors.New("nhk: building the request failed")
	}
	req.Header.Set("Accept", "application/json")
	return sendJSON(ctx, n.hc, req, out)
}

func (n *nhk) programs(ctx context.Context, in Input) (Output, error) {
	area, service, err := argNHKAreaService(in.Args)
	if err != nil {
		return Output{}, err
	}
	date, err := argNHKDate(in.Args, n.now())
	if err != nil {
		return Output{}, err
	}
	limit, err := argInt(in.Args, "limit", nhkListDefault, 1, nhkListMax)
	if err != nil {
		return Output{}, err
	}
	var a nhkListAnswer
	if err := n.get(ctx, "/v2/pg/list/"+area+"/"+service+"/"+date+".json", &a); err != nil {
		return Output{}, err
	}
	var all []nhkProgram
	for _, ps := range a.List {
		all = append(all, ps...)
	}
	// by start time (the services of a bundle come as separate lists)
	slices.SortStableFunc(all, func(x, y nhkProgram) int { return strings.Compare(x.StartTime, y.StartTime) })
	rows := make([]Row, 0, min(limit, len(all)))
	for _, p := range all {
		if len(rows) == limit {
			break
		}
		if r, ok := nhkRow(p, ""); ok {
			rows = append(rows, r)
		}
	}
	if len(rows) == 0 {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("nhk: no programs")}
	}
	return Output{Rows: rows}, nil
}

// nowOnAir is the program on air now (Item) with the previous and the
// following one (Rows), per service.
func (n *nhk) nowOnAir(ctx context.Context, in Input) (Output, error) {
	area, service, err := argNHKAreaService(in.Args)
	if err != nil {
		return Output{}, err
	}
	var a nhkNowAnswer
	if err := n.get(ctx, "/v2/pg/now/"+area+"/"+service+".json", &a); err != nil {
		return Output{}, err
	}
	ids := make([]string, 0, len(a.List))
	for id := range a.List {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var out Output
	for _, id := range ids {
		s := a.List[id]
		for _, it := range []struct {
			p     *nhkProgram
			label string
		}{{s.Previous, "前"}, {s.Present, "今"}, {s.Following, "次"}} {
			if it.p == nil {
				continue
			}
			r, ok := nhkRow(*it.p, it.label)
			if !ok {
				continue
			}
			if it.label == "今" && out.Item == nil {
				item := r
				out.Item = &item
			}
			out.Rows = append(out.Rows, r)
		}
	}
	if len(out.Rows) == 0 {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("nhk: nothing on air")}
	}
	return out, nil
}

// nhkRow turns a program into a row: title / "[label ]HH:MM–HH:MM · subtitle"
// / service name. A program without a title or a start time is skipped.
func nhkRow(p nhkProgram, label string) (Row, bool) {
	title := clipNHK(p.Title)
	start, err := time.Parse(time.RFC3339, p.StartTime)
	if title == "" || err != nil {
		return Row{}, false
	}
	span := start.In(jst).Format("15:04")
	if end, err := time.Parse(time.RFC3339, p.EndTime); err == nil {
		span += "–" + end.In(jst).Format("15:04")
	}
	if label != "" {
		span = label + " " + span
	}
	sub := span
	if s := clipNHK(p.Subtitle); s != "" {
		sub += " · " + s
	}
	return Row{Title: title, Subtitle: sub, Value: strings.Join(strings.Fields(p.Service.Name), " ")}, true
}

// clipNHK keeps one line of at most nhkMaxRunes runes.
func clipNHK(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= nhkMaxRunes {
		return s
	}
	return string([]rune(s)[:nhkMaxRunes-1]) + "…"
}
