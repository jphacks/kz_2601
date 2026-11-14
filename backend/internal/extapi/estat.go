package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// e-Stat (政府統計の総合窓口) API v3: the government statistics tables and
// their cells, read with the operator's appId
// (api.e-stat.go.jp/rest/3.0/app/json). Free: a call is one `runs`.
//
//	searchStats(q, limit?)              getStatsList: Rows (table title / statistics name · survey month / table id / e-Stat page)
//	statsData(statsDataId, limit?)      getStatsData: Rows (class names / value + unit), the first N cells
//
// Terms (e-Stat API 利用規約・クレジット表示): a service using the API shows
// EStatAttribution where the person can see it, and the source
// 「政府統計の総合窓口(e-Stat)」 is named with the numbers. Statistics do
// not change once published, so answers are cached (search 1 hour, a
// table's cells 1 day).
//
// The appId (EXT_ESTAT_API_KEY, Secret Manager) goes in the query as the API
// wants; errors never carry the URL (keyed.go). The API answers 200 with
// RESULT.STATUS for its own errors: 0-2 are fine (1 = no matching data),
// 101-105 and 300 are the request's fault (bad_request after the call),
// 100 (appId refused) and 2xx are upstream_unavailable.
const (
	EStat = "estat"
	// EStatAttribution is the credit e-Stat asks a service to show.
	EStatAttribution = "このサービスは、政府統計総合窓口(e-Stat)のAPI機能を使用していますが、サービスの内容は国によって保証されたものではありません。"
	// EStatSource is the source named with the numbers.
	EStatSource = "政府統計の総合窓口(e-Stat)"

	estatHost     = "https://api.e-stat.go.jp"
	estatBasePath = "/rest/3.0/app/json"
	estatPageURL  = "https://www.e-stat.go.jp/dbview?sid="

	estatQueryMax      = 100
	estatSearchDefault = 10
	estatSearchMax     = 50
	estatDataDefault   = 20
	estatDataMax       = 100
	estatMaxRunes      = 120
)

var (
	estatKeyRe = regexp.MustCompile(`^[A-Za-z0-9]{20,64}$`)
	estatIDRe  = regexp.MustCompile(`^[0-9]{8,12}$`)
)

func init() {
	register(Provider{ID: EStat, Billing: BillingFree, Hosts: []string{estatHost}, Terms: Terms{Attribution: EStatAttribution},
		Ops: []OpSpec{
			{Name: "searchStats", Args: []string{"q", "limit"}, Check: checkEStatSearch, CacheTTL: time.Hour},
			{Name: "statsData", Args: []string{"statsDataId", "limit"}, Check: checkEStatData, CacheTTL: 24 * time.Hour},
		},
		New: func(sec Secrets) (map[string]Operation, error) {
			return newEStat(sec.APIKey, estatHost, NewHTTPClient([]string{estatHost}, DefaultTimeout))
		}})
}

// argEStatQuery reads q: 1-100 runes, no control characters.
func argEStatQuery(args map[string]string) (string, error) {
	q := strings.TrimSpace(args["q"])
	if q == "" || !utf8.ValidString(q) || utf8.RuneCountInString(q) > estatQueryMax ||
		strings.ContainsFunc(q, unicode.IsControl) {
		return "", fmt.Errorf("%w: q must be 1-%d characters", ErrBadArgs, estatQueryMax)
	}
	return q, nil
}

// argEStatID reads statsDataId: the digits of a table id.
func argEStatID(args map[string]string) (string, error) {
	id := strings.TrimSpace(args["statsDataId"])
	if !estatIDRe.MatchString(id) {
		return "", fmt.Errorf("%w: statsDataId must be a table id (digits)", ErrBadArgs)
	}
	return id, nil
}

func checkEStatSearch(args map[string]string) error {
	if _, err := argEStatQuery(args); err != nil {
		return err
	}
	_, err := argInt(args, "limit", estatSearchDefault, 1, estatSearchMax)
	return err
}

func checkEStatData(args map[string]string) error {
	if _, err := argEStatID(args); err != nil {
		return err
	}
	_, err := argInt(args, "limit", estatDataDefault, 1, estatDataMax)
	return err
}

// newEStat builds the ops against base (the API, or a test server).
func newEStat(key, base string, hc *http.Client) (map[string]Operation, error) {
	if !estatKeyRe.MatchString(key) {
		return nil, errors.New("EXT_ESTAT_API_KEY is not set or not an appId")
	}
	e := &estat{key: key, base: base, hc: hc}
	return map[string]Operation{
		"searchStats": OperationFunc(e.searchStats),
		"statsData":   OperationFunc(e.statsData),
	}, nil
}

type estat struct {
	key  string
	base string
	hc   *http.Client
}

// estatList is a list the API writes as an array, or as one object when it
// has one element.
type estatList[T any] []T

func (l *estatList[T]) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '[' {
		return json.Unmarshal(b, (*[]T)(l))
	}
	var one T
	if err := json.Unmarshal(b, &one); err != nil {
		return err
	}
	*l = estatList[T]{one}
	return nil
}

// estatText is a value the API writes as "text" or as {"@attr": ..., "$": "text"}.
type estatText string

func (t *estatText) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '{' {
		var o struct {
			S string `json:"$"`
		}
		if err := json.Unmarshal(b, &o); err != nil {
			return err
		}
		*t = estatText(o.S)
		return nil
	}
	var s any
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch v := s.(type) {
	case string:
		*t = estatText(v)
	case float64:
		*t = estatText(strconv.FormatFloat(v, 'f', -1, 64))
	default:
		*t = ""
	}
	return nil
}

type estatResult struct {
	Status   int    `json:"STATUS"`
	ErrorMsg string `json:"ERROR_MSG"`
}

type estatTable struct {
	ID         string    `json:"@id"`
	StatName   estatText `json:"STAT_NAME"`
	Title      estatText `json:"TITLE"`
	SurveyDate estatText `json:"SURVEY_DATE"`
}

type estatListAnswer struct {
	Root struct {
		Result   estatResult `json:"RESULT"`
		DataList struct {
			Tables estatList[estatTable] `json:"TABLE_INF"`
		} `json:"DATALIST_INF"`
	} `json:"GET_STATS_LIST"`
}

type estatClass struct {
	Code string `json:"@code"`
	Name string `json:"@name"`
}

type estatClassObj struct {
	ID      string                `json:"@id"`
	Name    string                `json:"@name"`
	Classes estatList[estatClass] `json:"CLASS"`
}

// estatValue is one cell: its class codes (@tab, @cat01, @area, @time ...)
// and the value.
type estatValue map[string]string

type estatDataAnswer struct {
	Root struct {
		Result estatResult `json:"RESULT"`
		Data   struct {
			ClassInf struct {
				Objs estatList[estatClassObj] `json:"CLASS_OBJ"`
			} `json:"CLASS_INF"`
			DataInf struct {
				Values estatList[estatValue] `json:"VALUE"`
			} `json:"DATA_INF"`
		} `json:"STATISTICAL_DATA"`
	} `json:"GET_STATS_DATA"`
}

// get calls path with the appId and q.
func (e *estat) get(ctx context.Context, path string, q url.Values, out any) error {
	q.Set("appId", e.key)
	q.Set("lang", "J")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.base+estatBasePath+path+"?"+q.Encode(), nil)
	if err != nil {
		return errors.New("estat: building the request failed")
	}
	req.Header.Set("Accept", "application/json")
	return sendJSON(ctx, e.hc, req, out)
}

// checkResult turns RESULT.STATUS into the foundation's errors.
func checkResult(r estatResult) error {
	switch {
	case r.Status <= 2:
		return nil
	case r.Status >= 101 && r.Status <= 105, r.Status == 300:
		return afterCall(fmt.Errorf("%w: e-stat refused the request (status %d)", ErrBadArgs, r.Status))
	}
	return &ProviderError{Status: http.StatusOK, Retryable: r.Status >= 200 && r.Status < 300,
		Err: fmt.Errorf("estat: status %d", r.Status)}
}

func (e *estat) searchStats(ctx context.Context, in Input) (Output, error) {
	q, err := argEStatQuery(in.Args)
	if err != nil {
		return Output{}, err
	}
	limit, err := argInt(in.Args, "limit", estatSearchDefault, 1, estatSearchMax)
	if err != nil {
		return Output{}, err
	}
	var a estatListAnswer
	if err := e.get(ctx, "/getStatsList", url.Values{"searchWord": {q}, "limit": {strconv.Itoa(limit)}}, &a); err != nil {
		return Output{}, err
	}
	if err := checkResult(a.Root.Result); err != nil {
		return Output{}, err
	}
	rows := make([]Row, 0, min(limit, len(a.Root.DataList.Tables)))
	for _, t := range a.Root.DataList.Tables {
		if len(rows) == limit {
			break
		}
		if t.ID == "" || t.Title == "" {
			continue
		}
		rows = append(rows, Row{
			Title:    clipRunes(string(t.Title), estatMaxRunes),
			Subtitle: joinDots(clipRunes(string(t.StatName), estatMaxRunes), estatSurveyDate(string(t.SurveyDate))),
			Value:    t.ID,
			URL:      estatPageURL + url.QueryEscape(t.ID),
		})
	}
	return Output{Rows: rows}, nil
}

// estatSurveyDate turns "202001" / "202001-202012" into "2020年1月" /
// "2020年1月〜2020年12月"; "0" (no survey month) is empty.
func estatSurveyDate(s string) string {
	parts := strings.SplitN(s, "-", 2)
	out := make([]string, 0, 2)
	for _, p := range parts {
		if len(p) != 6 {
			return ""
		}
		y, err1 := strconv.Atoi(p[:4])
		m, err2 := strconv.Atoi(p[4:])
		if err1 != nil || err2 != nil || m < 1 || m > 12 {
			return ""
		}
		out = append(out, strconv.Itoa(y)+"年"+strconv.Itoa(m)+"月")
	}
	return strings.Join(out, "〜")
}

func (e *estat) statsData(ctx context.Context, in Input) (Output, error) {
	id, err := argEStatID(in.Args)
	if err != nil {
		return Output{}, err
	}
	limit, err := argInt(in.Args, "limit", estatDataDefault, 1, estatDataMax)
	if err != nil {
		return Output{}, err
	}
	var a estatDataAnswer
	if err := e.get(ctx, "/getStatsData", url.Values{"statsDataId": {id}, "limit": {strconv.Itoa(limit)}, "metaGetFlg": {"Y"}}, &a); err != nil {
		return Output{}, err
	}
	if err := checkResult(a.Root.Result); err != nil {
		return Output{}, err
	}
	// class id → code → name, in the order the table lists its classes
	type classMap struct {
		id    string
		names map[string]string
	}
	classes := make([]classMap, 0, len(a.Root.Data.ClassInf.Objs))
	for _, o := range a.Root.Data.ClassInf.Objs {
		m := classMap{id: o.ID, names: map[string]string{}}
		for _, c := range o.Classes {
			m.names[c.Code] = c.Name
		}
		classes = append(classes, m)
	}
	rows := make([]Row, 0, min(limit, len(a.Root.Data.DataInf.Values)))
	for _, v := range a.Root.Data.DataInf.Values {
		if len(rows) == limit {
			break
		}
		val := v["$"]
		if val == "" {
			continue
		}
		names := make([]string, 0, len(classes))
		for _, c := range classes {
			if n := c.names[v["@"+c.id]]; n != "" {
				names = append(names, n)
			}
		}
		rows = append(rows, Row{
			Title: clipRunes(strings.Join(names, "・"), estatMaxRunes),
			Value: strings.TrimSpace(val + " " + v["@unit"]),
		})
	}
	if len(rows) == 0 {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("estat: no cells")}
	}
	return Output{Rows: rows}, nil
}

// clipRunes keeps at most n runes of s (one space between words), with
// "…" when it was cut.
func clipRunes(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}
