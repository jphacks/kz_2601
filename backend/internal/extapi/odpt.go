package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

// ODPT (#1691): train service status (運行情報) from the Public
// Transportation Open Data Center (api.odpt.org/api/v4, odpt:TrainInformation),
// read with the operator's consumer key.
//
//	trainStatus(operator?, railway?)   Rows: one per line (line name / status・text / "M/D HH:MM 更新")
//
// operator and railway are ODPT ids from the closed allow-list below
// (odptOperators). Only operators whose data is under the ODPT Basic License
// or CC BY 4.0 are listed; the "Challenge Limited License" operators (JR
// East, Tokyu, Tobu, Keio, Seibu, Keikyu) are only for the yearly contest
// entries and are left out (docs/backend/features/external-apis.md).
//
// Terms (センター利用規約・基本ライセンス・開発者ガイドライン):
//   - the app shows ODPTAttribution (data from the center, not guaranteed,
//     no direct questions to the operators); Toei rows also need
//     ODPTToeiCredit (CC BY 4.0).
//   - the time of each row (dc:date) is shown, and a row past its dct:valid
//     is dropped; the shared cache keeps an answer 1 minute at most, and
//     never past the first dct:valid of its rows (Output.ValidUntil).
//   - the key (EXT_ODPT_API_KEY, Secret Manager) goes in the query
//     (acl:consumerKey) as the API wants; errors never carry the URL
//     (keyed.go).
const (
	ODPT = "odpt"
	// ODPTAttribution is the notice shown with the rows (開発者ガイドライン
	// 「利用者への通知」).
	ODPTAttribution = "公共交通オープンデータセンターのデータです。正確・完全とは限りません。遅れの情報は事業者の公式の案内を確かめてください(事業者への問い合わせはしないでください)"
	// ODPTToeiCredit is the CC BY 4.0 credit of the Toei rows.
	ODPTToeiCredit = "東京都交通局・公共交通オープンデータ協議会、運行情報、CC BY 4.0(https://creativecommons.org/licenses/by/4.0/deed.ja)"

	odptHost     = "https://api.odpt.org"
	odptPath     = "/api/v4/odpt:TrainInformation"
	odptMaxRows  = 60
	odptMaxRunes = 200
)

var odptKeyRe = regexp.MustCompile(`^[A-Za-z0-9]{16,128}$`)

// odptLine is one railway of the allow-list.
type odptLine struct{ id, name string }

// odptOperator is one operator of the allow-list and its railways.
type odptOperator struct {
	id, name string
	lines    []odptLine
}

// odptOperators is the closed allow-list (license checked 2026-11-12 on
// ckan.odpt.org, the dataset "<operator> 運行情報").
var odptOperators = []odptOperator{
	{"odpt.Operator:TokyoMetro", "東京メトロ", []odptLine{ // 基本ライセンス
		{"odpt.Railway:TokyoMetro.Ginza", "銀座線"},
		{"odpt.Railway:TokyoMetro.Marunouchi", "丸ノ内線"},
		{"odpt.Railway:TokyoMetro.MarunouchiBranch", "丸ノ内線(方南町支線)"},
		{"odpt.Railway:TokyoMetro.Hibiya", "日比谷線"},
		{"odpt.Railway:TokyoMetro.Tozai", "東西線"},
		{"odpt.Railway:TokyoMetro.Chiyoda", "千代田線"},
		{"odpt.Railway:TokyoMetro.Yurakucho", "有楽町線"},
		{"odpt.Railway:TokyoMetro.Hanzomon", "半蔵門線"},
		{"odpt.Railway:TokyoMetro.Namboku", "南北線"},
		{"odpt.Railway:TokyoMetro.Fukutoshin", "副都心線"},
	}},
	{"odpt.Operator:Toei", "都営", []odptLine{ // CC BY 4.0
		{"odpt.Railway:Toei.Asakusa", "浅草線"},
		{"odpt.Railway:Toei.Mita", "三田線"},
		{"odpt.Railway:Toei.Shinjuku", "新宿線"},
		{"odpt.Railway:Toei.Oedo", "大江戸線"},
		{"odpt.Railway:Toei.Arakawa", "東京さくらトラム(都電荒川線)"},
		{"odpt.Railway:Toei.NipporiToneri", "日暮里・舎人ライナー"},
	}},
	{"odpt.Operator:TWR", "東京臨海高速鉄道", []odptLine{ // 基本ライセンス
		{"odpt.Railway:TWR.Rinkai", "りんかい線"},
	}},
	{"odpt.Operator:MIR", "首都圏新都市鉄道", []odptLine{ // 基本ライセンス
		{"odpt.Railway:MIR.TsukubaExpress", "つくばエクスプレス"},
	}},
	{"odpt.Operator:TamaMonorail", "多摩都市モノレール", []odptLine{ // 基本ライセンス
		{"odpt.Railway:TamaMonorail.TamaMonorail", "多摩モノレール"},
	}},
	{"odpt.Operator:YokohamaMunicipal", "横浜市交通局", []odptLine{ // 基本ライセンス
		{"odpt.Railway:YokohamaMunicipal.Blue", "ブルーライン"},
		{"odpt.Railway:YokohamaMunicipal.Green", "グリーンライン"},
	}},
}

func init() {
	register(Provider{ID: ODPT, Billing: BillingFree,
		Ops: []OpSpec{
			{Name: "trainStatus", Args: []string{"operator", "railway"}, Check: checkODPTStatus, CacheTTL: time.Minute},
		},
		Hosts: []string{odptHost},
		New: func(sec Secrets) (map[string]Operation, error) {
			return newODPT(sec.APIKey, odptHost, NewHTTPClient([]string{odptHost}, DefaultTimeout), time.Now)
		}})
}

// odptOperatorIndex is the allow-list position of an operator id (-1: not
// listed).
func odptOperatorIndex(id string) int {
	return slices.IndexFunc(odptOperators, func(o odptOperator) bool { return o.id == id })
}

// odptLineIndex finds a railway id: its operator and line positions.
func odptLineIndex(id string) (op, line int) {
	for i, o := range odptOperators {
		if j := slices.IndexFunc(o.lines, func(l odptLine) bool { return l.id == id }); j >= 0 {
			return i, j
		}
	}
	return -1, -1
}

// odptFilter reads operator / railway: the query to send. Both empty is no
// filter (every listed operator). A railway must belong to the operator
// when both are given.
func odptFilter(args map[string]string) (url.Values, error) {
	op, rw := strings.TrimSpace(args["operator"]), strings.TrimSpace(args["railway"])
	oi := -1
	if op != "" {
		if oi = odptOperatorIndex(op); oi < 0 {
			return nil, fmt.Errorf("%w: operator must be a listed ODPT operator id", ErrUnknown)
		}
	}
	if rw == "" {
		if oi < 0 {
			return url.Values{}, nil
		}
		return url.Values{"odpt:operator": {op}}, nil
	}
	ri, _ := odptLineIndex(rw)
	if ri < 0 {
		return nil, fmt.Errorf("%w: railway must be a listed ODPT railway id", ErrUnknown)
	}
	if oi >= 0 && oi != ri {
		return nil, fmt.Errorf("%w: railway is not a line of operator", ErrUnknown)
	}
	return url.Values{"odpt:railway": {rw}}, nil
}

func checkODPTStatus(args map[string]string) error {
	_, err := odptFilter(args)
	return err
}

// newODPT builds the op against base (the API, or a test server).
func newODPT(key, base string, hc *http.Client, now func() time.Time) (map[string]Operation, error) {
	if !odptKeyRe.MatchString(key) {
		return nil, errors.New("EXT_ODPT_API_KEY is not set or not a key")
	}
	o := &odpt{key: key, base: base, hc: hc, now: now}
	return map[string]Operation{"trainStatus": OperationFunc(o.trainStatus)}, nil
}

type odpt struct {
	key  string
	base string
	hc   *http.Client
	now  func() time.Time
}

// odptText is a language map ({"ja": …, "en": …}); a few operators send a
// plain string.
type odptText string

func (t *odptText) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*t = odptText(s)
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	*t = odptText(m["ja"])
	return nil
}

type odptTrainInformation struct {
	Date     string   `json:"dc:date"`
	Valid    string   `json:"dct:valid"`
	Operator string   `json:"odpt:operator"`
	Railway  string   `json:"odpt:railway"`
	Status   odptText `json:"odpt:trainInformationStatus"`
	Text     odptText `json:"odpt:trainInformationText"`
}

var odptJST = time.FixedZone("JST", 9*60*60)

func (o *odpt) trainStatus(ctx context.Context, in Input) (Output, error) {
	q, err := odptFilter(in.Args)
	if err != nil {
		return Output{}, err
	}
	q.Set("acl:consumerKey", o.key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.base+odptPath+"?"+q.Encode(), nil)
	if err != nil {
		return Output{}, errors.New("odpt: building the request failed")
	}
	req.Header.Set("Accept", "application/json")
	var infos []odptTrainInformation
	if err := sendJSON(ctx, o.hc, req, &infos); err != nil {
		return Output{}, err
	}

	type ranked struct {
		op, line int
		row      Row
		valid    time.Time // zero: no dct:valid
	}
	now := o.now()
	var rs []ranked
	for _, it := range infos {
		oi := odptOperatorIndex(it.Operator)
		if oi < 0 {
			continue // not on the allow-list (e.g. a contest-only operator)
		}
		date, err := time.Parse(time.RFC3339, it.Date)
		if err != nil {
			continue // the time must be shown with the row (ガイドライン 2.1.1)
		}
		valid, verr := time.Parse(time.RFC3339, it.Valid)
		if verr == nil && valid.Before(now) {
			continue // past its valid time (ガイドライン 2.1.2)
		}
		title, li := odptOperators[oi].name, -1
		if it.Railway != "" {
			ri, j := odptLineIndex(it.Railway)
			if ri != oi {
				continue
			}
			title, li = odptOperators[oi].lines[j].name, j
		}
		text := odptClip(strings.TrimSpace(string(it.Text)))
		if s := odptClip(strings.TrimSpace(string(it.Status))); s != "" && text != "" {
			text = s + ": " + text
		} else if s != "" {
			text = s
		}
		rs = append(rs, ranked{oi, li, Row{Title: title, Subtitle: text, Value: date.In(odptJST).Format("1/2 15:04") + " 更新"}, valid})
	}
	slices.SortStableFunc(rs, func(a, b ranked) int {
		if a.op != b.op {
			return a.op - b.op
		}
		return a.line - b.line
	})
	// the cache keeps the answer no longer than its first row is valid
	// (Output.ValidUntil), so a cached row is never shown past dct:valid
	var out Output
	out.Rows = make([]Row, 0, min(len(rs), odptMaxRows))
	for _, r := range rs {
		if len(out.Rows) == odptMaxRows {
			break
		}
		out.Rows = append(out.Rows, r.row)
		if !r.valid.IsZero() && (out.ValidUntil.IsZero() || r.valid.Before(out.ValidUntil)) {
			out.ValidUntil = r.valid
		}
	}
	// TODO(#1643): Output has no place for a notice yet, so ODPTAttribution
	// (and ODPTToeiCredit for Toei rows) is not sent with the rows; the
	// part that shows them adds it when the DSL's external-API parts land.
	return out, nil
}

func odptClip(s string) string {
	if r := []rune(s); len(r) > odptMaxRunes {
		return string(r[:odptMaxRunes]) + "…"
	}
	return s
}
