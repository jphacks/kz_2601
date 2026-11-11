package extapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Notion (#1497, API version notionVersion): free (one `runs` a call),
// OAuth of a public integration. The person picks the pages and databases
// the integration may use on Notion's consent page; only those are reached.
//
//	queryDatabase(databaseId, limit?, subtitleProperty?, valueProperty?)
//	addRow(databaseId, title, properties?)
//	updateProperties(pageId, properties)
//	pageText(pageId)
//
// Notion has no OAuth scopes (the integration's capabilities are set when
// it is registered: read content, insert content) and documents no PKCE,
// so the consent URL has none; the one-use state still binds the callback.
// Its access tokens do not expire: a token without an expiry is used until
// Notion refuses it (401 → connect again). When Notion does send a refresh
// token and an expiry, they are used.
//
// Env: EXT_NOTION_ENABLED=1, EXT_NOTION_CLIENT_ID, EXT_NOTION_CLIENT_SECRET
// (Secret Manager). Off by default; not in the review build (NeedsOAuth).
const Notion = "notion"

// notionVersion is the Notion-Version header of every request (databases
// hold data sources since 2025-09-03).
const notionVersion = "2026-03-11"

// notionWire is where the provider talks to; tests point it at httptest.
type notionWire struct {
	AuthURL   string // opened in the browser, not reached by the runner
	TokenURL  string
	RevokeURL string
	APIBase   string
	// Client is nil in production: NewHTTPClient on the provider's Hosts.
	Client *http.Client
	// Now is nil in production (time.Now).
	Now func() time.Time
}

var notionWireDefault = notionWire{
	AuthURL:   "https://api.notion.com/v1/oauth/authorize",
	TokenURL:  "https://api.notion.com/v1/oauth/token",
	RevokeURL: "https://api.notion.com/v1/oauth/revoke",
	APIBase:   "https://api.notion.com/v1",
}

var notionHosts = []string{"https://api.notion.com"}

func init() {
	register(Provider{ID: Notion, Billing: BillingFree, NeedsOAuth: true, NameJa: "Notion", NameEn: "Notion",
		Ops: []OpSpec{
			{Name: "queryDatabase", Args: []string{"databaseId", "limit", "subtitleProperty", "valueProperty"}},
			{Name: "addRow", Write: true, Args: []string{"databaseId", "title", "properties"}},
			{Name: "updateProperties", Write: true, Args: []string{"pageId", "properties"}, Check: checkNotionUpdate},
			{Name: "pageText", Args: []string{"pageId"}, Check: checkNotionPageText},
		},
		Hosts: notionHosts,
		New: func(sec Secrets) (map[string]Operation, error) {
			c, err := newNotion(sec, notionWireDefault)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"queryDatabase": OperationFunc(c.queryDatabase), "addRow": OperationFunc(c.addRow),
				"updateProperties": OperationFunc(c.updateProperties), "pageText": OperationFunc(c.pageText)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) { return newNotion(sec, notionWireDefault) },
	})
}

// notionMaxConcurrent keeps one instance under Notion's rate limit (about
// three requests a second per integration).
const notionMaxConcurrent = 3

// notion is the provider's ops and OAuth client.
type notion struct {
	sec  Secrets
	wire notionWire
	hc   *http.Client
	now  func() time.Time
	sem  chan struct{}
	memo *notionMemo
}

func newNotion(sec Secrets, w notionWire) (*notion, error) {
	if sec.ClientID == "" || sec.ClientSecret == "" {
		return nil, errors.New("EXT_NOTION_CLIENT_ID / _CLIENT_SECRET not set")
	}
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(notionHosts, DefaultTimeout)
	}
	now := w.Now
	if now == nil {
		now = time.Now
	}
	return &notion{sec: sec, wire: w, hc: hc, now: now, sem: make(chan struct{}, notionMaxConcurrent), memo: newNotionMemo(now)}, nil
}

// --- OAuth ---

// AuthURL is the public integration's consent (owner=user). Scopes and the
// code challenge are not Notion's; the person picks pages there instead.
func (c *notion) AuthURL(state, _, redirectURI string, _ []string) string {
	v := url.Values{
		"client_id":     {c.sec.ClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"owner":         {"user"},
		"state":         {state},
	}
	return c.wire.AuthURL + "?" + v.Encode()
}

func (c *notion) Exchange(ctx context.Context, code, _, redirectURI string) (Token, error) {
	return c.token(ctx, map[string]string{"grant_type": "authorization_code", "code": code, "redirect_uri": redirectURI})
}

// Refresh: a token without a refresh token (Notion's do not expire) cannot
// be renewed; the service only asks when an expiry was given.
func (c *notion) Refresh(ctx context.Context, t Token) (Token, error) {
	if t.RefreshToken == "" {
		return Token{}, ErrInvalidGrant
	}
	return c.token(ctx, map[string]string{"grant_type": "refresh_token", "refresh_token": t.RefreshToken})
}

// Revoke revokes the access token at Notion (the integration leaves the
// person's workspace). Already invalid is done.
func (c *notion) Revoke(ctx context.Context, t Token) error {
	if t.AccessToken == "" {
		return nil
	}
	resp, err := c.basic(ctx, c.wire.RevokeURL, map[string]string{"token": t.AccessToken})
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := ReadBody(resp.Body)
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode == http.StatusBadRequest && notionErrorCode(body) == "invalid_grant" {
		return nil
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500, Err: errors.New("revoke refused")}
}

func (c *notion) token(ctx context.Context, form map[string]string) (Token, error) {
	resp, err := c.basic(ctx, c.wire.TokenURL, form)
	if err != nil {
		return Token{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := ReadBody(resp.Body)
	if err != nil {
		return Token{}, err
	}
	if resp.StatusCode != http.StatusOK {
		if notionErrorCode(body) == "invalid_grant" {
			return Token{}, ErrInvalidGrant
		}
		return Token{}, &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500 || resp.StatusCode == 429, Err: errors.New("token endpoint refused")}
	}
	var r struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &r); err != nil || r.AccessToken == "" {
		return Token{}, errors.New("notion: bad token answer")
	}
	t := Token{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken}
	// no expires_in: the token does not expire (never refreshed)
	if r.ExpiresIn > 0 && r.RefreshToken != "" {
		t.Expiry = c.now().Add(time.Duration(r.ExpiresIn) * time.Second).UTC()
	}
	return t, nil
}

// basic posts JSON with the client's Basic auth (token / revoke).
func (c *notion) basic(ctx context.Context, u string, body map[string]string) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.sec.ClientID, c.sec.ClientSecret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Notion-Version", notionVersion)
	return c.hc.Do(req)
}

// notionErrorCode reads OAuth's {"error"} or Notion's {"code"}.
func notionErrorCode(body []byte) string {
	var e struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	_ = json.Unmarshal(body, &e)
	if e.Error != "" {
		return e.Error
	}
	return e.Code
}

// --- ops ---

const (
	notionDefaultLimit = 20
	notionMaxLimit     = 50
	notionMaxProps     = 20
	notionMaxPropsLen  = 8 << 10
	notionMaxText      = 2000 // Notion's rich text content
	notionMaxName      = 100  // select / status option names
)

var notionHexRe = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)

// notionID checks a database id: 32 hex, with or without dashes (the
// part of the database's link), and returns the dashed form.
func notionID(name, v string) (string, error) {
	h := strings.ReplaceAll(strings.TrimSpace(v), "-", "")
	if !notionHexRe.MatchString(h) {
		return "", fmt.Errorf("%w: %s must be a Notion id (32 hex)", ErrUnknown, name)
	}
	h = strings.ToLower(h)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}

// notionPropName checks a property name argument.
func notionPropName(name, v string) (string, error) {
	return gcalText(name, v, 200, false)
}

type notionRichText struct {
	PlainText string `json:"plain_text"`
}

func notionPlain(rt []notionRichText) string {
	var b strings.Builder
	for _, t := range rt {
		b.WriteString(t.PlainText)
	}
	return b.String()
}

// notionValue is one property's value in a page (the types a row shows).
type notionValue struct {
	Type        string                  `json:"type"`
	Title       []notionRichText        `json:"title"`
	RichText    []notionRichText        `json:"rich_text"`
	Number      *float64                `json:"number"`
	Checkbox    *bool                   `json:"checkbox"`
	Select      *struct{ Name string }  `json:"select"`
	Status      *struct{ Name string }  `json:"status"`
	MultiSelect []struct{ Name string } `json:"multi_select"`
	Date        *struct {
		Start string `json:"start"`
		End   string `json:"end"`
	} `json:"date"`
	URL            *string `json:"url"`
	Email          *string `json:"email"`
	PhoneNumber    *string `json:"phone_number"`
	CreatedTime    string  `json:"created_time"`
	LastEditedTime string  `json:"last_edited_time"`
	UniqueID       *struct {
		Prefix *string `json:"prefix"`
		Number *int64  `json:"number"`
	} `json:"unique_id"`
	Formula *struct {
		Type    string   `json:"type"`
		String  *string  `json:"string"`
		Number  *float64 `json:"number"`
		Boolean *bool    `json:"boolean"`
		Date    *struct {
			Start string `json:"start"`
		} `json:"date"`
	} `json:"formula"`
}

func notionNumber(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func notionBool(b bool) string {
	if b {
		return "✓"
	}
	return "−"
}

// text is the value as one line (types a row cannot show are "").
func (v notionValue) text() string {
	switch v.Type {
	case "title":
		return notionPlain(v.Title)
	case "rich_text":
		return notionPlain(v.RichText)
	case "number":
		if v.Number != nil {
			return notionNumber(*v.Number)
		}
	case "checkbox":
		if v.Checkbox != nil {
			return notionBool(*v.Checkbox)
		}
	case "select":
		if v.Select != nil {
			return v.Select.Name
		}
	case "status":
		if v.Status != nil {
			return v.Status.Name
		}
	case "multi_select":
		names := make([]string, 0, len(v.MultiSelect))
		for _, o := range v.MultiSelect {
			names = append(names, o.Name)
		}
		return strings.Join(names, ", ")
	case "date":
		if v.Date != nil {
			if v.Date.End != "" {
				return v.Date.Start + " → " + v.Date.End
			}
			return v.Date.Start
		}
	case "url":
		if v.URL != nil {
			return *v.URL
		}
	case "email":
		if v.Email != nil {
			return *v.Email
		}
	case "phone_number":
		if v.PhoneNumber != nil {
			return *v.PhoneNumber
		}
	case "created_time":
		return v.CreatedTime
	case "last_edited_time":
		return v.LastEditedTime
	case "unique_id":
		if v.UniqueID != nil && v.UniqueID.Number != nil {
			n := strconv.FormatInt(*v.UniqueID.Number, 10)
			if v.UniqueID.Prefix != nil && *v.UniqueID.Prefix != "" {
				return *v.UniqueID.Prefix + "-" + n
			}
			return n
		}
	case "formula":
		if f := v.Formula; f != nil {
			switch {
			case f.String != nil:
				return *f.String
			case f.Number != nil:
				return notionNumber(*f.Number)
			case f.Boolean != nil:
				return notionBool(*f.Boolean)
			case f.Date != nil:
				return f.Date.Start
			}
		}
	}
	return ""
}

type notionPage struct {
	ID         string                 `json:"id"`
	URL        string                 `json:"url"`
	Properties map[string]notionValue `json:"properties"`
}

// row: the title property is the title; subtitle / value are the named
// properties; the link only to notion.so.
func (p notionPage) row(subtitleProp, valueProp string) Row {
	var r Row
	for _, v := range p.Properties {
		if v.Type == "title" {
			r.Title = notionPlain(v.Title)
			break
		}
	}
	if r.Title == "" {
		r.Title = gcalNoTitle
	}
	if subtitleProp != "" {
		r.Subtitle = p.Properties[subtitleProp].text()
	}
	if valueProp != "" {
		r.Value = p.Properties[valueProp].text()
	}
	if u, err := url.Parse(p.URL); err == nil && u.Scheme == "https" && (u.Host == "www.notion.so" || u.Host == "notion.so") {
		r.URL = p.URL
	}
	return r
}

// dataSource returns the database's (first) data source id. A database the
// person did not share with the integration is 404 (connect again and pick it).
func (c *notion) dataSource(ctx context.Context, t *Token, databaseID string) (string, error) {
	var db struct {
		DataSources []struct {
			ID string `json:"id"`
		} `json:"data_sources"`
	}
	if err := c.do(ctx, t, http.MethodGet, "/databases/"+databaseID, nil, &db); err != nil {
		return "", err
	}
	if len(db.DataSources) == 0 {
		return "", &ProviderError{Status: http.StatusNotFound, Err: errors.New("notion: the database has no data source")}
	}
	id, err := notionID("data source", db.DataSources[0].ID)
	if err != nil {
		return "", errors.New("notion: bad data source id")
	}
	return id, nil
}

// queryDatabase lists a database's rows (its first data source), the most
// recently edited first, one page of limit (default 20). subtitleProperty /
// valueProperty name the properties shown under the title / on the right.
func (c *notion) queryDatabase(ctx context.Context, in Input) (Output, error) {
	dbID, err := notionID("databaseId", in.Args["databaseId"])
	if err != nil {
		return Output{}, err
	}
	n := notionDefaultLimit
	if v := in.Args["limit"]; v != "" {
		x, err := strconv.Atoi(v)
		if err != nil || x < 1 || x > notionMaxLimit {
			return Output{}, fmt.Errorf("%w: limit must be 1-%d", ErrUnknown, notionMaxLimit)
		}
		n = x
	}
	sub, err := notionPropName("subtitleProperty", in.Args["subtitleProperty"])
	if err != nil {
		return Output{}, err
	}
	val, err := notionPropName("valueProperty", in.Args["valueProperty"])
	if err != nil {
		return Output{}, err
	}
	ds, err := c.dataSource(ctx, in.Token, dbID)
	if err != nil {
		return Output{}, err
	}
	q := map[string]any{
		"page_size":   n,
		"result_type": "page",
		"sorts":       []map[string]string{{"timestamp": "last_edited_time", "direction": "descending"}},
	}
	b, err := json.Marshal(q)
	if err != nil {
		return Output{}, err
	}
	var r struct {
		Results []notionPage `json:"results"`
	}
	if err := c.do(ctx, in.Token, http.MethodPost, "/data_sources/"+ds+"/query", b, &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, len(r.Results))}
	for _, p := range r.Results {
		out.Rows = append(out.Rows, p.row(sub, val))
	}
	return out, nil
}

// notionSchema is a data source's property types by name.
type notionSchema map[string]struct {
	Type string `json:"type"`
}

// addRow adds a page to a database (its first data source): title goes to
// the title property; properties is a JSON object of property name → value
// for the types listed in notionPropValue. Notion has no idempotency key:
// the request id is remembered on this instance for 10 minutes, so a
// retried request makes no second row.
func (c *notion) addRow(ctx context.Context, in Input) (Output, error) {
	dbID, err := notionID("databaseId", in.Args["databaseId"])
	if err != nil {
		return Output{}, err
	}
	title, err := gcalText("title", in.Args["title"], notionMaxText, true)
	if err != nil {
		return Output{}, err
	}
	props, err := notionProps(in.Args["properties"], false)
	if err != nil {
		return Output{}, err
	}
	if in.Token == nil || in.Token.AccessToken == "" {
		return Output{}, ErrNotConnected
	}
	return c.memo.do(notionMemoKey(in, dbID), func() (Output, error) {
		ds, err := c.dataSource(ctx, in.Token, dbID)
		if err != nil {
			return Output{}, err
		}
		var schema struct {
			Properties notionSchema `json:"properties"`
		}
		if err := c.do(ctx, in.Token, http.MethodGet, "/data_sources/"+ds, nil, &schema); err != nil {
			return Output{}, err
		}
		values := map[string]any{}
		for name, p := range schema.Properties {
			if p.Type == "title" {
				values[name] = map[string]any{"title": notionText(title)}
			}
		}
		if len(values) != 1 {
			return Output{}, errors.New("notion: the data source has no title property")
		}
		names := make([]string, 0, len(props))
		for name := range props {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			p, ok := schema.Properties[name]
			if !ok {
				return Output{}, fmt.Errorf("%w: the database has no property %q", ErrUnknown, name)
			}
			v, err := notionPropValue(name, p.Type, props[name])
			if err != nil {
				return Output{}, err
			}
			values[name] = v
		}
		body, err := json.Marshal(map[string]any{
			"parent":     map[string]string{"type": "data_source_id", "data_source_id": ds},
			"properties": values,
		})
		if err != nil {
			return Output{}, err
		}
		var page notionPage
		if err := c.do(ctx, in.Token, http.MethodPost, "/pages", body, &page); err != nil {
			return Output{}, err
		}
		row := page.row("", "")
		if row.Title == gcalNoTitle {
			row.Title = title
		}
		return Output{ResultID: page.ID, Item: &row}, nil
	})
}

// notionProps reads a properties argument: a JSON object of property name
// → value, at most notionMaxProps entries.
func notionProps(v string, required bool) (map[string]any, error) {
	if v == "" {
		if required {
			return nil, fmt.Errorf("%w: properties is required", ErrUnknown)
		}
		return nil, nil
	}
	if len(v) > notionMaxPropsLen {
		return nil, fmt.Errorf("%w: properties is over %d bytes", ErrUnknown, notionMaxPropsLen)
	}
	var props map[string]any
	d := json.NewDecoder(strings.NewReader(v))
	d.UseNumber()
	if err := d.Decode(&props); err != nil || d.More() {
		return nil, fmt.Errorf("%w: properties must be a JSON object", ErrUnknown)
	}
	if len(props) > notionMaxProps {
		return nil, fmt.Errorf("%w: properties has over %d entries", ErrUnknown, notionMaxProps)
	}
	if required && len(props) == 0 {
		return nil, fmt.Errorf("%w: properties is empty", ErrUnknown)
	}
	return props, nil
}

func notionText(s string) []map[string]any {
	return []map[string]any{{"type": "text", "text": map[string]string{"content": s}}}
}

// notionPropValue turns one properties entry into Notion's property value.
// Settable: rich_text, number, checkbox, select, status, multi_select, date,
// url (https), email, phone_number. Others (relation, people, files,
// formula ...) are refused.
func notionPropValue(name, typ string, v any) (any, error) {
	bad := func(want string) error {
		return fmt.Errorf("%w: property %q must be %s", ErrUnknown, name, want)
	}
	str := func(max int) (string, error) {
		s, ok := v.(string)
		if !ok {
			return "", bad("a string")
		}
		return gcalText(name, s, max, false)
	}
	switch typ {
	case "rich_text":
		s, err := str(notionMaxText)
		if err != nil {
			return nil, err
		}
		return map[string]any{"rich_text": notionText(s)}, nil
	case "number":
		var f float64
		var err error
		switch x := v.(type) {
		case json.Number:
			f, err = x.Float64()
		case string:
			f, err = strconv.ParseFloat(strings.TrimSpace(x), 64)
		default:
			return nil, bad("a number")
		}
		if err != nil {
			return nil, bad("a number")
		}
		return map[string]any{"number": f}, nil
	case "checkbox":
		switch x := v.(type) {
		case bool:
			return map[string]any{"checkbox": x}, nil
		case string:
			if b, err := strconv.ParseBool(x); err == nil {
				return map[string]any{"checkbox": b}, nil
			}
		}
		return nil, bad("true or false")
	case "select", "status":
		s, err := str(notionMaxName)
		if err != nil {
			return nil, err
		}
		if s == "" {
			return map[string]any{typ: nil}, nil
		}
		return map[string]any{typ: map[string]string{"name": s}}, nil
	case "multi_select":
		var list []string
		switch x := v.(type) {
		case string:
			for _, s := range strings.Split(x, ",") {
				if s = strings.TrimSpace(s); s != "" {
					list = append(list, s)
				}
			}
		case []any:
			for _, e := range x {
				s, ok := e.(string)
				if !ok {
					return nil, bad("a list of names")
				}
				list = append(list, s)
			}
		default:
			return nil, bad("a list of names")
		}
		if len(list) > 20 {
			return nil, bad("at most 20 names")
		}
		opts := make([]map[string]string, 0, len(list))
		for _, s := range list {
			s, err := gcalText(name, s, notionMaxName, true)
			if err != nil {
				return nil, err
			}
			if strings.Contains(s, ",") {
				return nil, bad("names without commas")
			}
			opts = append(opts, map[string]string{"name": s})
		}
		return map[string]any{"multi_select": opts}, nil
	case "date":
		s, ok := v.(string)
		if !ok {
			return nil, bad("YYYY-MM-DD or RFC 3339")
		}
		t, err := gcalParseTime(name, s)
		if err != nil {
			return nil, err
		}
		start := t.t.Format(time.RFC3339)
		if t.allDay {
			start = t.t.Format(time.DateOnly)
		}
		return map[string]any{"date": map[string]string{"start": start}}, nil
	case "url":
		s, err := str(2000)
		if err != nil {
			return nil, err
		}
		if u, err := url.Parse(s); s != "" && (err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil) {
			return nil, bad("an https link")
		}
		return map[string]any{"url": notionNullable(s)}, nil
	case "email":
		s, err := str(200)
		if err != nil {
			return nil, err
		}
		if s != "" && (!strings.Contains(s, "@") || strings.ContainsAny(s, " \t\n")) {
			return nil, bad("an email address")
		}
		return map[string]any{"email": notionNullable(s)}, nil
	case "phone_number":
		s, err := str(50)
		if err != nil {
			return nil, err
		}
		if strings.ContainsFunc(s, func(r rune) bool { return !strings.ContainsRune("0123456789+-() ", r) }) {
			return nil, bad("a phone number")
		}
		return map[string]any{"phone_number": notionNullable(s)}, nil
	}
	return nil, fmt.Errorf("%w: property %q (%s) cannot be set", ErrUnknown, name, typ)
}

func notionNullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// notionUpdatable are the property types updateProperties sets; others
// (status, relation, people, files, formula ...) are refused.
var notionUpdatable = []string{"title", "rich_text", "number", "select", "checkbox", "date"}

// notionUpdateArgs checks updateProperties' pageId and properties (1-20
// named entries; the values are checked against the page's types later).
func notionUpdateArgs(a map[string]string) (string, map[string]any, error) {
	id, err := notionID("pageId", a["pageId"])
	if err != nil {
		return "", nil, err
	}
	props, err := notionProps(a["properties"], true)
	if err != nil {
		return "", nil, err
	}
	for name := range props {
		if _, err := gcalText("property name", name, 200, true); err != nil {
			return "", nil, err
		}
	}
	return id, props, nil
}

func checkNotionUpdate(a map[string]string) error {
	_, _, err := notionUpdateArgs(a)
	return err
}

// updateProperties sets properties of one page (a database row): a JSON
// object of property name → value, of the types in notionUpdatable (title
// is a string; the rest as notionPropValue). The page's own types decide
// how a value is read. Setting the same values again changes nothing, so a
// retried request is harmless.
func (c *notion) updateProperties(ctx context.Context, in Input) (Output, error) {
	id, props, err := notionUpdateArgs(in.Args)
	if err != nil {
		return Output{}, err
	}
	if in.Token == nil || in.Token.AccessToken == "" {
		return Output{}, ErrNotConnected
	}
	var page notionPage
	if err := c.do(ctx, in.Token, http.MethodGet, "/pages/"+id, nil, &page); err != nil {
		return Output{}, err
	}
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	slices.Sort(names)
	values := make(map[string]any, len(props))
	for _, name := range names {
		p, ok := page.Properties[name]
		if !ok {
			return Output{}, fmt.Errorf("%w: the page has no property %q", ErrUnknown, name)
		}
		if !slices.Contains(notionUpdatable, p.Type) {
			return Output{}, fmt.Errorf("%w: property %q (%s) cannot be set; only %s", ErrUnknown, name, p.Type, strings.Join(notionUpdatable, ", "))
		}
		if p.Type == "title" {
			s, ok := props[name].(string)
			if !ok {
				return Output{}, fmt.Errorf("%w: property %q must be a string", ErrUnknown, name)
			}
			s, err := gcalText(name, s, notionMaxText, true)
			if err != nil {
				return Output{}, err
			}
			values[name] = map[string]any{"title": notionText(s)}
			continue
		}
		v, err := notionPropValue(name, p.Type, props[name])
		if err != nil {
			return Output{}, err
		}
		values[name] = v
	}
	body, err := json.Marshal(map[string]any{"properties": values})
	if err != nil {
		return Output{}, err
	}
	var updated notionPage
	if err := c.do(ctx, in.Token, http.MethodPatch, "/pages/"+id, body, &updated); err != nil {
		return Output{}, err
	}
	row := updated.row("", "")
	return Output{ResultID: updated.ID, Item: &row}, nil
}

const (
	// notionMaxPageText caps pageText's text (bytes; MaxBody/16 keeps the
	// answer far under the runner's answer cap even when JSON escapes it).
	notionMaxPageText = MaxBody / 16
	// notionMaxBlockReads caps the block pages read (100 blocks each).
	notionMaxBlockReads = 5
)

func checkNotionPageText(a map[string]string) error {
	_, err := notionID("pageId", a["pageId"])
	return err
}

// notionBlockText is the text of one block's content (types with text).
type notionBlockText struct {
	RichText []notionRichText `json:"rich_text"`
	Checked  bool             `json:"checked"`
	Title    string           `json:"title"`
}

// line is one block as a line of plain text; ok false for blocks without
// text (images, tables ...). n numbers a run of numbered list items.
func notionBlockLine(typ string, b notionBlockText, n int) (string, bool) {
	t := notionPlain(b.RichText)
	switch typ {
	case "paragraph", "heading_1", "heading_2", "heading_3", "quote", "callout", "toggle", "code":
		return t, true
	case "bulleted_list_item":
		return "• " + t, true
	case "numbered_list_item":
		return strconv.Itoa(n) + ". " + t, true
	case "to_do":
		if b.Checked {
			return "☑ " + t, true
		}
		return "☐ " + t, true
	case "child_page":
		return b.Title, true
	case "divider":
		return "---", true
	}
	return "", false
}

// pageText reads one page's top-level blocks as plain text (nested blocks
// are not read): title = the page's title, value = the text, cut at
// notionMaxPageText (ending in "…") after at most notionMaxBlockReads reads.
func (c *notion) pageText(ctx context.Context, in Input) (Output, error) {
	id, err := notionID("pageId", in.Args["pageId"])
	if err != nil {
		return Output{}, err
	}
	var page notionPage
	if err := c.do(ctx, in.Token, http.MethodGet, "/pages/"+id, nil, &page); err != nil {
		return Output{}, err
	}
	var b strings.Builder
	cut, number, cursor := false, 0, ""
	for read := 0; ; read++ {
		if read == notionMaxBlockReads {
			cut = true
			break
		}
		v := url.Values{"page_size": {"100"}}
		if cursor != "" {
			v.Set("start_cursor", cursor)
		}
		var r struct {
			Results    []map[string]json.RawMessage `json:"results"`
			HasMore    bool                         `json:"has_more"`
			NextCursor string                       `json:"next_cursor"`
		}
		if err := c.do(ctx, in.Token, http.MethodGet, "/blocks/"+id+"/children?"+v.Encode(), nil, &r); err != nil {
			return Output{}, err
		}
		for _, blk := range r.Results {
			var typ string
			if json.Unmarshal(blk["type"], &typ) != nil {
				continue
			}
			var bt notionBlockText
			if raw, ok := blk[typ]; ok {
				_ = json.Unmarshal(raw, &bt)
			}
			if typ == "numbered_list_item" {
				number++
			} else {
				number = 0
			}
			line, ok := notionBlockLine(typ, bt, number)
			if !ok {
				continue
			}
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(line)
			if b.Len() > notionMaxPageText {
				cut = true
				break
			}
		}
		if cut || !r.HasMore {
			break
		}
		if cursor, err = notionID("next_cursor", r.NextCursor); err != nil {
			cut = true
			break
		}
	}
	text := strings.TrimRight(b.String(), "\n ")
	if cut {
		text = notionCut(text, notionMaxPageText-len("…")) + "…"
	}
	row := page.row("", "")
	row.Value = text
	return Output{Item: &row}, nil
}

// notionCut cuts s to at most max bytes on a rune boundary.
func notionCut(s string, max int) string {
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
}

// do calls the API with the person's token and the Notion-Version. 401 is
// a token Notion no longer takes (ErrInvalidGrant: the connection turns to
// error); 403 / 404 are pages the person did not share with the integration
// (ErrNotConnected: connect again and pick them); 429 / 409 / 5xx are
// retryable provider errors. At most notionMaxConcurrent requests at once.
func (c *notion) do(ctx context.Context, t *Token, method, path string, body []byte, out any) error {
	if t == nil || t.AccessToken == "" {
		return ErrNotConnected
	}
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return ctx.Err()
	}
	req, err := http.NewRequestWithContext(ctx, method, c.wire.APIBase+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+t.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Notion-Version", notionVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := ReadBody(resp.Body)
	if err != nil {
		return err
	}
	code := notionErrorCode(b)
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("notion: bad answer: %w", err)
		}
		return nil
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrInvalidGrant
	case resp.StatusCode == http.StatusForbidden || (resp.StatusCode == http.StatusNotFound && code == "object_not_found"):
		return ErrNotConnected
	case resp.StatusCode == http.StatusBadRequest && code == "validation_error":
		return &ProviderError{Status: resp.StatusCode, Err: fmt.Errorf("%w: notion refused a value", ErrUnknown)}
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode == 429 || resp.StatusCode == 409 || resp.StatusCode >= 500,
		Err: fmt.Errorf("notion %s", code)}
}

// notionMemo remembers a write's answer by request id for a while, and
// runs one write per request id at a time (a retry waits for the first).
type notionMemo struct {
	mu   sync.Mutex
	now  func() time.Time
	done map[string]notionMemoEntry
	busy map[string]chan struct{}
}

type notionMemoEntry struct {
	out Output
	at  time.Time
}

const (
	notionMemoTTL = 10 * time.Minute
	notionMemoMax = 1000
)

// notionMemoKey binds the request id to the person's token and the
// database ("" without a request id: not remembered).
func notionMemoKey(in Input, dbID string) string {
	if in.RequestID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(in.Token.AccessToken + "\x00" + dbID + "\x00" + in.RequestID))
	return hex.EncodeToString(sum[:])
}

func newNotionMemo(now func() time.Time) *notionMemo {
	return &notionMemo{now: now, done: map[string]notionMemoEntry{}, busy: map[string]chan struct{}{}}
}

func (m *notionMemo) do(key string, f func() (Output, error)) (Output, error) {
	if key == "" {
		return f()
	}
	for {
		m.mu.Lock()
		if e, ok := m.done[key]; ok && m.now().Sub(e.at) < notionMemoTTL {
			m.mu.Unlock()
			return e.out, nil
		}
		ch, running := m.busy[key]
		if !running {
			ch = make(chan struct{})
			m.busy[key] = ch
			m.mu.Unlock()
			break
		}
		m.mu.Unlock()
		<-ch
	}
	out, err := f()
	m.mu.Lock()
	defer m.mu.Unlock()
	close(m.busy[key])
	delete(m.busy, key)
	if err == nil {
		now := m.now()
		if len(m.done) >= notionMemoMax {
			for k, e := range m.done {
				if now.Sub(e.at) >= notionMemoTTL {
					delete(m.done, k)
				}
			}
			for k := range m.done {
				if len(m.done) < notionMemoMax {
					break
				}
				delete(m.done, k)
			}
		}
		m.done[key] = notionMemoEntry{out: out, at: now}
	}
	return out, err
}
