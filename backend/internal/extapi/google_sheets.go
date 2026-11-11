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
	"strconv"
	"strings"
	"unicode/utf8"
)

// Google Sheets (#1685): free (one `runs` a call), OAuth with PKCE.
//
//	readRange(spreadsheetId, range, limit?)    drive.file
//	appendRow(spreadsheetId, range, values)    drive.file
//
// drive.file reaches only files the person picked in the Google Picker (or
// the app made): not sensitive, unlike spreadsheets. A sheet not picked is
// 404 from Google and 400 bad_request here. The OAuth client is Google
// Calendar's unless EXT_GOOGLE_SHEETS_CLIENT_ID / _CLIENT_SECRET are set
// (ClientFrom: one consent screen). Sheet contents are never cached.
//
// Env: EXT_GOOGLE_SHEETS_ENABLED=1. Off by default; not in the review build
// (NeedsOAuth).
const GoogleSheets = "googleSheets"

const gsheetsScope = "https://www.googleapis.com/auth/drive.file"

var googleSheetsWire = gcalWire{
	AuthURL:   "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL:  "https://oauth2.googleapis.com/token",
	RevokeURL: "https://oauth2.googleapis.com/revoke",
	APIBase:   "https://sheets.googleapis.com/v4",
}

var googleSheetsHosts = []string{"https://oauth2.googleapis.com", "https://sheets.googleapis.com"}

func init() {
	register(Provider{ID: GoogleSheets, Billing: BillingFree, NeedsOAuth: true, NameJa: "Google スプレッドシート", NameEn: "Google Sheets",
		Ops: []OpSpec{
			{Name: "readRange", Scopes: []string{gsheetsScope}, Args: []string{"spreadsheetId", "range", "limit"}, Check: checkGsheetsRead},
			{Name: "appendRow", Write: true, Scopes: []string{gsheetsScope}, Args: []string{"spreadsheetId", "range", "values"}, Check: checkGsheetsAppend},
		},
		Hosts:      googleSheetsHosts,
		ClientFrom: GoogleCalendar,
		New: func(sec Secrets) (map[string]Operation, error) {
			s, err := newGoogleSheets(sec, googleSheetsWire)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"readRange": OperationFunc(s.readRange), "appendRow": OperationFunc(s.appendRow)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) {
			s, err := newGoogleSheets(sec, googleSheetsWire)
			if err != nil {
				return nil, err
			}
			return s.g, nil
		},
	})
}

// googleSheets is the provider's ops; the OAuth client and do are Google
// Calendar's code on the Sheets wire.
type googleSheets struct {
	g *googleCalendar
}

func newGoogleSheets(sec Secrets, w gcalWire) (*googleSheets, error) {
	g, err := newGoogleClient(GoogleSheets, sec, w, googleSheetsHosts)
	if err != nil {
		return nil, err
	}
	return &googleSheets{g: g}, nil
}

// --- arguments ---

const (
	gsheetsDefaultLimit = 50
	gsheetsMaxLimit     = 100
	gsheetsMaxCols      = 10 // readRange: cells after the 10th are dropped
	gsheetsMaxValues    = 26 // appendRow: A-Z
	gsheetsCellMax      = 1000
	gsheetsShownMax     = 200 // runes of one cell in a row
	gsheetsRangeMax     = 200
)

var (
	gsheetsIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{20,128}$`)
	// gsheetsCellsRe: the cell part of an A1 range (A1, A1:D50, A:D, A).
	gsheetsCellsRe = regexp.MustCompile(`^\$?([A-Z]{1,3})\$?([0-9]{0,7})(?::\$?([A-Z]{1,3})\$?([0-9]{0,7}))?$`)
)

func gsheetsID(v string) (string, error) {
	if !gsheetsIDRe.MatchString(v) {
		return "", fmt.Errorf("%w: spreadsheetId must be a Google Sheets file id (pick the file first)", ErrUnknown)
	}
	return v, nil
}

func gsheetsLimit(v string) (int, error) {
	if v == "" {
		return gsheetsDefaultLimit, nil
	}
	x, err := strconv.Atoi(v)
	if err != nil || x < 1 || x > gsheetsMaxLimit {
		return 0, fmt.Errorf("%w: limit must be 1-%d", ErrUnknown, gsheetsMaxLimit)
	}
	return x, nil
}

// gsheetsRange reads a sheet name or an A1 range (Sheet1!A1:D50, 'My
// sheet'!A:D, A1:D50 on the first sheet). It returns the range to send:
// one without row bounds (a sheet, A:D) gets rows 1..limit, so Google never
// sends more than the op shows.
func gsheetsRange(v string, limit int) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" || !utf8.ValidString(v) || utf8.RuneCountInString(v) > gsheetsRangeMax ||
		strings.ContainsFunc(v, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "", fmt.Errorf("%w: range must be a sheet name or an A1 range like Sheet1!A1:D50", ErrUnknown)
	}
	sheet, cells, hasCells := "", v, true
	if i := strings.LastIndex(v, "!"); i >= 0 {
		sheet, cells = v[:i], v[i+1:]
		if sheet == "" {
			return "", fmt.Errorf("%w: range has no sheet name before the \"!\" mark", ErrUnknown)
		}
	} else if !gsheetsCellsRe.MatchString(v) {
		// a sheet name alone; quoted so a name like "売上 2026" is one sheet
		sheet, cells, hasCells = "'"+strings.ReplaceAll(v, "'", "''")+"'", "", false
	}
	if hasCells && !gsheetsCellsRe.MatchString(cells) {
		return "", fmt.Errorf("%w: range must be a sheet name or an A1 range like Sheet1!A1:D50", ErrUnknown)
	}
	if !hasCells {
		if limit == 0 {
			return sheet, nil
		}
		return sheet + "!1:" + strconv.Itoa(limit), nil
	}
	prefix := ""
	if sheet != "" {
		prefix = sheet + "!"
	}
	if limit == 0 {
		return prefix + cells, nil
	}
	m := gsheetsCellsRe.FindStringSubmatch(cells)
	if m[3] == "" { // one cell or one column
		if m[2] != "" {
			return prefix + cells, nil
		}
		m[3] = m[1]
	}
	// rows start..start+limit-1 at most (A:D → A1:D50, A2:D → A2:D51)
	start := 1
	if m[2] != "" {
		start, _ = strconv.Atoi(m[2])
		if start < 1 {
			return "", fmt.Errorf("%w: range rows start at 1", ErrUnknown)
		}
	}
	end := start + limit - 1
	if m[4] != "" {
		if e, _ := strconv.Atoi(m[4]); e >= start && e < end {
			end = e
		}
	}
	return prefix + m[1] + strconv.Itoa(start) + ":" + m[3] + strconv.Itoa(end), nil
}

// gsheetsValues reads appendRow's values: a JSON array of 1-26 strings.
func gsheetsValues(v string) ([]string, error) {
	var vals []string
	if err := json.Unmarshal([]byte(v), &vals); err != nil || len(vals) == 0 || len(vals) > gsheetsMaxValues {
		return nil, fmt.Errorf("%w: values must be a JSON array of 1-%d strings", ErrUnknown, gsheetsMaxValues)
	}
	for i, c := range vals {
		if !utf8.ValidString(c) || utf8.RuneCountInString(c) > gsheetsCellMax {
			return nil, fmt.Errorf("%w: values[%d] is over %d characters", ErrUnknown, i, gsheetsCellMax)
		}
		if strings.ContainsFunc(c, func(r rune) bool { return r < 0x20 && r != '\n' && r != '\t' }) {
			return nil, fmt.Errorf("%w: values[%d] has control characters", ErrUnknown, i)
		}
	}
	return vals, nil
}

func checkGsheetsRead(a map[string]string) error {
	_, _, _, err := gsheetsReadQuery(a)
	return err
}

func checkGsheetsAppend(a map[string]string) error {
	_, _, err := gsheetsAppendBody(a)
	return err
}

// gsheetsCan: the granted scopes cover the ops (unknown scopes pass; Google
// then answers 403 and the person connects again).
func gsheetsCan(t *Token) bool {
	if t == nil || len(t.Scopes) == 0 {
		return true
	}
	ok := []string{gsheetsScope, "https://www.googleapis.com/auth/drive", "https://www.googleapis.com/auth/spreadsheets"}
	return slices.ContainsFunc(t.Scopes, func(s string) bool { return slices.Contains(ok, s) })
}

// --- answers ---

// gsheetsRawCell is a cell as Output.Cells has it (the import): trimmed, at
// most gsheetsCellMax runes.
func gsheetsRawCell(v any) string {
	var s string
	switch x := v.(type) {
	case nil:
	case string:
		s = x
	default:
		b, _ := json.Marshal(x)
		s = string(b)
	}
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > gsheetsCellMax {
		s = string([]rune(s)[:gsheetsCellMax])
	}
	return s
}

// gsheetsRawCells are the first gsheetsMaxCols cells of a row.
func gsheetsRawCells(cells []any) []string {
	out := make([]string, min(len(cells), gsheetsMaxCols))
	for i := range out {
		out[i] = gsheetsRawCell(cells[i])
	}
	return out
}

func gsheetsCell(v any) string {
	s := strings.TrimSpace(strings.ReplaceAll(gsheetsRawCell(v), "\n", " "))
	if utf8.RuneCountInString(s) > gsheetsShownMax {
		s = string([]rune(s)[:gsheetsShownMax]) + "…"
	}
	return s
}

// gsheetsRow: title = 1st column, value = 2nd (a number for a chart),
// subtitle = 3rd to 10th joined with ・.
func gsheetsRow(cells []any) Row {
	if len(cells) > gsheetsMaxCols {
		cells = cells[:gsheetsMaxCols]
	}
	var r Row
	var rest []string
	for i, c := range cells {
		s := gsheetsCell(c)
		switch i {
		case 0:
			r.Title = s
		case 1:
			r.Value = s
		default:
			if s != "" {
				rest = append(rest, s)
			}
		}
	}
	r.Subtitle = strings.Join(rest, "・")
	return r
}

func gsheetsLink(id string) string {
	return "https://docs.google.com/spreadsheets/d/" + id + "/edit"
}

// --- ops ---

func gsheetsReadQuery(a map[string]string) (string, url.Values, int, error) {
	id, err := gsheetsID(a["spreadsheetId"])
	if err != nil {
		return "", nil, 0, err
	}
	n, err := gsheetsLimit(a["limit"])
	if err != nil {
		return "", nil, 0, err
	}
	rng, err := gsheetsRange(a["range"], n)
	if err != nil {
		return "", nil, 0, err
	}
	v := url.Values{"majorDimension": {"ROWS"}, "valueRenderOption": {"FORMATTED_VALUE"}}
	return "/spreadsheets/" + url.PathEscape(id) + "/values/" + url.PathEscape(rng), v, n, nil
}

// readRange reads a range of a sheet the person picked: one row a sheet
// row (empty rows left out), limit rows (default 50, at most 100), the
// first 10 columns. Cells keeps the columns for the import. Nothing is
// cached.
func (s *googleSheets) readRange(ctx context.Context, in Input) (Output, error) {
	if !gsheetsCan(in.Token) {
		return Output{}, ErrNotConnected
	}
	path, v, n, err := gsheetsReadQuery(in.Args)
	if err != nil {
		return Output{}, err
	}
	var r struct {
		Values [][]any `json:"values"`
	}
	if err := s.do(ctx, in.Token, http.MethodGet, path+"?"+v.Encode(), nil, &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, min(len(r.Values), n))}
	for _, cells := range r.Values {
		if len(out.Rows) == n {
			break
		}
		row := gsheetsRow(cells)
		if row.Title == "" && row.Value == "" && row.Subtitle == "" {
			continue
		}
		out.Rows = append(out.Rows, row)
		out.Cells = append(out.Cells, gsheetsRawCells(cells))
	}
	return out, nil
}

// gsheetsAppendBody checks appendRow's arguments and builds the path and
// body. RAW: a value starting with = stays text, never a formula.
func gsheetsAppendBody(a map[string]string) (string, []byte, error) {
	id, err := gsheetsID(a["spreadsheetId"])
	if err != nil {
		return "", nil, err
	}
	rng, err := gsheetsRange(a["range"], 0)
	if err != nil {
		return "", nil, err
	}
	vals, err := gsheetsValues(a["values"])
	if err != nil {
		return "", nil, err
	}
	b, err := json.Marshal(struct {
		MajorDimension string     `json:"majorDimension"`
		Values         [][]string `json:"values"`
	}{"ROWS", [][]string{vals}})
	if err != nil {
		return "", nil, err
	}
	v := url.Values{"valueInputOption": {"RAW"}, "insertDataOption": {"INSERT_ROWS"}}
	return "/spreadsheets/" + url.PathEscape(id) + "/values/" + url.PathEscape(rng) + ":append?" + v.Encode(), b, nil
}

// appendRow adds one row after the table in range. Google takes no request
// id, so the service's never-retried write is the only guard against a
// second row. resultId is the range written (Sheet1!A5:C5).
func (s *googleSheets) appendRow(ctx context.Context, in Input) (Output, error) {
	if !gsheetsCan(in.Token) {
		return Output{}, ErrNotConnected
	}
	path, body, err := gsheetsAppendBody(in.Args)
	if err != nil {
		return Output{}, err
	}
	var r struct {
		Updates struct {
			UpdatedRange string `json:"updatedRange"`
		} `json:"updates"`
	}
	if err := s.do(ctx, in.Token, http.MethodPost, path, body, &r); err != nil {
		return Output{}, err
	}
	vals, _ := gsheetsValues(in.Args["values"])
	cells := make([]any, len(vals))
	for i, c := range vals {
		cells[i] = c
	}
	row := gsheetsRow(cells)
	row.URL = gsheetsLink(in.Args["spreadsheetId"])
	return Output{ResultID: r.Updates.UpdatedRange, Item: &row}, nil
}

// do is Google Calendar's do (401 → ErrInvalidGrant, a missing scope →
// ErrNotConnected, 429 / 5xx / rate limits → retryable) plus: any other 403
// is ErrNotConnected (connect again), 404 is a sheet the person did not
// pick (bad_request), 400 a range the sheet does not have (bad_request).
func (s *googleSheets) do(ctx context.Context, tok *Token, method, path string, body []byte, out any) error {
	ctype := ""
	if body != nil {
		ctype = "application/json"
	}
	err := s.g.do(ctx, tok, method, path, body, ctype, out)
	var pe *ProviderError
	if errors.As(err, &pe) && !pe.Retryable {
		switch pe.Status {
		case http.StatusForbidden:
			return ErrNotConnected
		case http.StatusNotFound:
			return &ProviderError{Status: pe.Status, Err: fmt.Errorf("%w: googleSheets cannot see this spreadsheet; pick it in the Google Picker first", ErrUnknown)}
		case http.StatusBadRequest:
			return &ProviderError{Status: pe.Status, Err: fmt.Errorf("%w: googleSheets has no such sheet or range", ErrUnknown)}
		}
	}
	return err
}
