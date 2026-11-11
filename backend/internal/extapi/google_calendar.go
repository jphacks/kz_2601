package extapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Google Calendar (#1494): free (one `runs` a call), OAuth with PKCE, the
// person's primary calendar only.
//
//	listEvents(timeMin?, timeMax?, maxResults?, q?)        calendar.events.owned.readonly
//	createEvent(title, start, end?, description?, location?)  calendar.events.owned
//
// The scopes are the narrowest that read / write events of calendars the
// person owns (not calendar.events, which reaches every calendar they can
// see). Both are sensitive: Google's verification is a human step.
//
// Env: EXT_GOOGLE_CALENDAR_ENABLED=1, EXT_GOOGLE_CALENDAR_CLIENT_ID,
// EXT_GOOGLE_CALENDAR_CLIENT_SECRET (Secret Manager). Off by default; not in
// the review build (NeedsOAuth).
const GoogleCalendar = "googleCalendar"

const (
	gcalScopeRead  = "https://www.googleapis.com/auth/calendar.events.owned.readonly"
	gcalScopeWrite = "https://www.googleapis.com/auth/calendar.events.owned"
)

// gcalWire is where the provider talks to; tests point it at httptest.
type gcalWire struct {
	AuthURL   string // opened in the browser, not reached by the runner
	TokenURL  string
	RevokeURL string
	APIBase   string
	// Client is nil in production: NewHTTPClient on the provider's Hosts.
	Client *http.Client
	// Now is nil in production (time.Now).
	Now func() time.Time
}

var googleCalendarWire = gcalWire{
	AuthURL:   "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL:  "https://oauth2.googleapis.com/token",
	RevokeURL: "https://oauth2.googleapis.com/revoke",
	APIBase:   "https://www.googleapis.com/calendar/v3",
}

var googleCalendarHosts = []string{"https://oauth2.googleapis.com", "https://www.googleapis.com"}

func init() {
	register(Provider{ID: GoogleCalendar, Billing: BillingFree, NeedsOAuth: true, NameJa: "Google カレンダー", NameEn: "Google Calendar",
		Ops: []OpSpec{
			{Name: "listEvents", Scopes: []string{gcalScopeRead}, Args: []string{"timeMin", "timeMax", "maxResults", "q"}},
			{Name: "createEvent", Write: true, Scopes: []string{gcalScopeWrite}, Args: []string{"title", "start", "end", "description", "location"}},
		},
		Hosts: googleCalendarHosts,
		New: func(sec Secrets) (map[string]Operation, error) {
			g, err := newGoogleCalendar(sec, googleCalendarWire)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"listEvents": OperationFunc(g.listEvents), "createEvent": OperationFunc(g.createEvent)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) { return newGoogleCalendar(sec, googleCalendarWire) },
	})
}

// googleCalendar is the provider's ops and OAuth client. googleTasks uses
// the same OAuth client and do with its own wire.
type googleCalendar struct {
	id   string // the provider, in errors
	sec  Secrets
	wire gcalWire
	hc   *http.Client
	now  func() time.Time
}

func newGoogleCalendar(sec Secrets, w gcalWire) (*googleCalendar, error) {
	return newGoogleClient(GoogleCalendar, sec, w, googleCalendarHosts)
}

// newGoogleClient is the OAuth client and API caller of one Google provider
// (Calendar, Tasks): the same token endpoints, its own API and hosts.
func newGoogleClient(id string, sec Secrets, w gcalWire, hosts []string) (*googleCalendar, error) {
	if sec.ClientID == "" || sec.ClientSecret == "" {
		return nil, fmt.Errorf("EXT_%s_CLIENT_ID / _CLIENT_SECRET not set", envName(id))
	}
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(hosts, DefaultTimeout)
	}
	now := w.Now
	if now == nil {
		now = time.Now
	}
	return &googleCalendar{id: id, sec: sec, wire: w, hc: hc, now: now}, nil
}

// --- OAuth ---

// AuthURL asks for offline access (a refresh token) and adds to the scopes
// already granted (include_granted_scopes: the write scope comes later).
// prompt=consent makes Google send a refresh token on every connect.
func (g *googleCalendar) AuthURL(state, codeChallenge, redirectURI string, scopes []string) string {
	v := url.Values{
		"client_id":              {g.sec.ClientID},
		"redirect_uri":           {redirectURI},
		"response_type":          {"code"},
		"scope":                  {strings.Join(scopes, " ")},
		"state":                  {state},
		"code_challenge":         {codeChallenge},
		"code_challenge_method":  {"S256"},
		"access_type":            {"offline"},
		"include_granted_scopes": {"true"},
		"prompt":                 {"consent"},
	}
	return g.wire.AuthURL + "?" + v.Encode()
}

func (g *googleCalendar) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (Token, error) {
	return g.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {codeVerifier}, "redirect_uri": {redirectURI}}, Token{})
}

func (g *googleCalendar) Refresh(ctx context.Context, t Token) (Token, error) {
	if t.RefreshToken == "" {
		return Token{}, ErrInvalidGrant
	}
	return g.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.RefreshToken}}, t)
}

// Revoke revokes the refresh token (that ends every access token of the
// grant), or the access token when there is none. Already invalid is done.
func (g *googleCalendar) Revoke(ctx context.Context, t Token) error {
	tok := t.RefreshToken
	if tok == "" {
		tok = t.AccessToken
	}
	if tok == "" {
		return nil
	}
	resp, err := g.postForm(ctx, g.wire.RevokeURL, url.Values{"token": {tok}})
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := ReadBody(resp.Body)
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode == http.StatusBadRequest && gcalOAuthErrorCode(body) == "invalid_token" {
		return nil
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500, Err: errors.New("revoke refused")}
}

// token posts to the token endpoint; prev keeps the scopes when the answer
// has none.
func (g *googleCalendar) token(ctx context.Context, form url.Values, prev Token) (Token, error) {
	form.Set("client_id", g.sec.ClientID)
	form.Set("client_secret", g.sec.ClientSecret)
	resp, err := g.postForm(ctx, g.wire.TokenURL, form)
	if err != nil {
		return Token{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := ReadBody(resp.Body)
	if err != nil {
		return Token{}, err
	}
	if resp.StatusCode != http.StatusOK {
		if code := gcalOAuthErrorCode(body); code == "invalid_grant" {
			return Token{}, ErrInvalidGrant
		}
		return Token{}, &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500 || resp.StatusCode == 429, Err: errors.New("token endpoint refused")}
	}
	var r struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &r); err != nil || r.AccessToken == "" {
		return Token{}, fmt.Errorf("%s: bad token answer", g.id)
	}
	t := Token{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken, Scopes: strings.Fields(r.Scope)}
	if r.ExpiresIn > 0 {
		t.Expiry = g.now().Add(time.Duration(r.ExpiresIn) * time.Second).UTC()
	}
	if len(t.Scopes) == 0 {
		t.Scopes = prev.Scopes
	}
	return t, nil
}

func (g *googleCalendar) postForm(ctx context.Context, u string, form url.Values) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	return g.hc.Do(req)
}

func gcalOAuthErrorCode(body []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &e)
	return e.Error
}

// --- ops ---

// gcalJST reads a date without an offset (the app is Japanese).
var gcalJST = time.FixedZone("JST", 9*60*60)

const (
	gcalMaxRange       = 366 * 24 * time.Hour
	gcalDefaultRange   = 7 * 24 * time.Hour
	gcalDefaultResults = 20
	gcalMaxResults     = 50
	gcalNoTitle        = "(タイトルなし)"
)

// gcalTime is an argument time: RFC 3339, or YYYY-MM-DD (all day, JST).
type gcalTime struct {
	t      time.Time
	allDay bool
}

func gcalParseTime(name, v string) (gcalTime, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return gcalTime{t: t}, nil
	}
	if t, err := time.ParseInLocation(time.DateOnly, v, gcalJST); err == nil {
		return gcalTime{t: t, allDay: true}, nil
	}
	return gcalTime{}, fmt.Errorf("%w: %s must be RFC 3339 or YYYY-MM-DD", ErrUnknown, name)
}

// gcalText checks a free-text argument's length (runes) and control chars.
func gcalText(name, v string, max int, required bool) (string, error) {
	v = strings.TrimSpace(v)
	switch {
	case v == "" && required:
		return "", fmt.Errorf("%w: %s is required", ErrUnknown, name)
	case !utf8.ValidString(v) || utf8.RuneCountInString(v) > max:
		return "", fmt.Errorf("%w: %s is over %d characters", ErrUnknown, name, max)
	case strings.ContainsFunc(v, func(r rune) bool { return r < 0x20 && r != '\n' && r != '\t' }):
		return "", fmt.Errorf("%w: %s has control characters", ErrUnknown, name)
	}
	return v, nil
}

// gcalHasScope: the token's granted scopes cover want (unknown scopes pass;
// Google then answers 403 and the person connects again).
func gcalHasScope(t *Token, want string) bool {
	if t == nil || len(t.Scopes) == 0 {
		return true
	}
	ok := []string{want, "https://www.googleapis.com/auth/calendar.events", "https://www.googleapis.com/auth/calendar"}
	if want == gcalScopeRead {
		ok = append(ok, gcalScopeWrite, "https://www.googleapis.com/auth/calendar.events.readonly", "https://www.googleapis.com/auth/calendar.readonly")
	}
	return slices.ContainsFunc(t.Scopes, func(s string) bool { return slices.Contains(ok, s) })
}

type gcalEvent struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	Location string `json:"location"`
	HTMLLink string `json:"htmlLink"`
	Start    struct {
		DateTime string `json:"dateTime"`
		Date     string `json:"date"`
	} `json:"start"`
}

func (e gcalEvent) row() Row {
	r := Row{Title: e.Summary, Subtitle: e.Location, Value: e.Start.DateTime}
	if r.Title == "" {
		r.Title = gcalNoTitle
	}
	if r.Value == "" {
		r.Value = e.Start.Date
	}
	if u, err := url.Parse(e.HTMLLink); err == nil && u.Scheme == "https" && (u.Host == "www.google.com" || u.Host == "calendar.google.com") {
		r.URL = e.HTMLLink
	}
	return r
}

// listEvents lists the primary calendar's events in [timeMin, timeMax)
// (default: now → 7 days), one page, start time order, repeats expanded.
func (g *googleCalendar) listEvents(ctx context.Context, in Input) (Output, error) {
	if !gcalHasScope(in.Token, gcalScopeRead) {
		return Output{}, ErrNotConnected
	}
	from := gcalTime{t: g.now()}
	if v := in.Args["timeMin"]; v != "" {
		t, err := gcalParseTime("timeMin", v)
		if err != nil {
			return Output{}, err
		}
		from = t
	}
	to := gcalTime{t: from.t.Add(gcalDefaultRange)}
	if v := in.Args["timeMax"]; v != "" {
		t, err := gcalParseTime("timeMax", v)
		if err != nil {
			return Output{}, err
		}
		to = t
	}
	if !to.t.After(from.t) || to.t.Sub(from.t) > gcalMaxRange {
		return Output{}, fmt.Errorf("%w: timeMax must be after timeMin and within 366 days", ErrUnknown)
	}
	n := gcalDefaultResults
	if v := in.Args["maxResults"]; v != "" {
		x, err := strconv.Atoi(v)
		if err != nil || x < 1 || x > gcalMaxResults {
			return Output{}, fmt.Errorf("%w: maxResults must be 1-%d", ErrUnknown, gcalMaxResults)
		}
		n = x
	}
	q, err := gcalText("q", in.Args["q"], 256, false)
	if err != nil {
		return Output{}, err
	}
	v := url.Values{
		"timeMin":      {from.t.Format(time.RFC3339)},
		"timeMax":      {to.t.Format(time.RFC3339)},
		"singleEvents": {"true"},
		"orderBy":      {"startTime"},
		"maxResults":   {strconv.Itoa(n)},
		"fields":       {"items(id,summary,location,htmlLink,start)"},
	}
	if q != "" {
		v.Set("q", q)
	}
	var r struct {
		Items []gcalEvent `json:"items"`
	}
	if err := g.do(ctx, in.Token, http.MethodGet, "/calendars/primary/events?"+v.Encode(), nil, "", &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, len(r.Items))}
	for _, e := range r.Items {
		out.Rows = append(out.Rows, e.row())
	}
	return out, nil
}

// createEvent adds an event to the primary calendar. start YYYY-MM-DD is an
// all-day event (end: the last day, default the same day); a time without
// end lasts an hour. The event id comes from the request id, so a retried
// request makes no second event (Google answers 409 for the same id).
func (g *googleCalendar) createEvent(ctx context.Context, in Input) (Output, error) {
	if !gcalHasScope(in.Token, gcalScopeWrite) {
		return Output{}, ErrNotConnected
	}
	title, err := gcalText("title", in.Args["title"], 500, true)
	if err != nil {
		return Output{}, err
	}
	desc, err := gcalText("description", in.Args["description"], 8000, false)
	if err != nil {
		return Output{}, err
	}
	loc, err := gcalText("location", in.Args["location"], 500, false)
	if err != nil {
		return Output{}, err
	}
	if in.Args["start"] == "" {
		return Output{}, fmt.Errorf("%w: start is required", ErrUnknown)
	}
	start, err := gcalParseTime("start", in.Args["start"])
	if err != nil {
		return Output{}, err
	}
	end := start
	if v := in.Args["end"]; v != "" {
		if end, err = gcalParseTime("end", v); err != nil {
			return Output{}, err
		}
		if end.allDay != start.allDay {
			return Output{}, fmt.Errorf("%w: start and end must both be dates or both be times", ErrUnknown)
		}
	} else if !start.allDay {
		end.t = start.t.Add(time.Hour)
	}
	type when struct {
		Date     string `json:"date,omitempty"`
		DateTime string `json:"dateTime,omitempty"`
	}
	var s, e when
	if start.allDay {
		if end.t.Before(start.t) || end.t.Sub(start.t) > gcalMaxRange {
			return Output{}, fmt.Errorf("%w: end must be on or after start and within 366 days", ErrUnknown)
		}
		// Google's all-day end is exclusive
		s, e = when{Date: start.t.Format(time.DateOnly)}, when{Date: end.t.AddDate(0, 0, 1).Format(time.DateOnly)}
	} else {
		if !end.t.After(start.t) || end.t.Sub(start.t) > gcalMaxRange {
			return Output{}, fmt.Errorf("%w: end must be after start and within 366 days", ErrUnknown)
		}
		s, e = when{DateTime: start.t.Format(time.RFC3339)}, when{DateTime: end.t.Format(time.RFC3339)}
	}
	id := gcalEventID(in.RequestID)
	body := struct {
		ID          string `json:"id,omitempty"`
		Summary     string `json:"summary"`
		Description string `json:"description,omitempty"`
		Location    string `json:"location,omitempty"`
		Start       when   `json:"start"`
		End         when   `json:"end"`
	}{ID: id, Summary: title, Description: desc, Location: loc, Start: s, End: e}
	b, err := json.Marshal(body)
	if err != nil {
		return Output{}, err
	}
	var ev gcalEvent
	err = g.do(ctx, in.Token, http.MethodPost, "/calendars/primary/events?sendUpdates=none", b, "application/json", &ev)
	var pe *ProviderError
	if errors.As(err, &pe) && pe.Status == http.StatusConflict && id != "" {
		// the same request made it already
		return Output{ResultID: id, Item: &Row{Title: title, Value: s.DateTime + s.Date}}, nil
	}
	if err != nil {
		return Output{}, err
	}
	row := ev.row()
	return Output{ResultID: ev.ID, Item: &row}, nil
}

// gcalEventID turns a request id into a valid event id (base32hex, 5-1024).
func gcalEventID(requestID string) string {
	if requestID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("tailorit:" + requestID))
	return strings.ToLower(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:]))
}

// do calls the provider's API (wire.APIBase) with the person's token and
// decodes the answer.
// 401 is a token Google no longer takes (ErrInvalidGrant: the connection
// turns to error); a missing scope is ErrNotConnected (connect again with
// it); 429 / 5xx / rate limits are retryable provider errors.
func (g *googleCalendar) do(ctx context.Context, t *Token, method, path string, body []byte, ctype string, out any) error {
	if t == nil || t.AccessToken == "" {
		return ErrNotConnected
	}
	req, err := http.NewRequestWithContext(ctx, method, g.wire.APIBase+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+t.AccessToken)
	req.Header.Set("Accept", "application/json")
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	resp, err := g.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := ReadBody(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("%s: bad answer: %w", g.id, err)
		}
		return nil
	}
	var e struct {
		Error struct {
			Status  string `json:"status"`
			Details []struct {
				Reason string `json:"reason"`
			} `json:"details"`
			Errors []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	_ = json.Unmarshal(b, &e)
	reasons := []string{e.Error.Status}
	for _, d := range e.Error.Details {
		reasons = append(reasons, d.Reason)
	}
	for _, d := range e.Error.Errors {
		reasons = append(reasons, d.Reason)
	}
	has := func(rs ...string) bool {
		return slices.ContainsFunc(reasons, func(r string) bool { return slices.Contains(rs, r) })
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrInvalidGrant
	case resp.StatusCode == http.StatusForbidden && has("insufficientPermissions", "ACCESS_TOKEN_SCOPE_INSUFFICIENT"):
		return ErrNotConnected
	case resp.StatusCode == http.StatusForbidden && has("rateLimitExceeded", "userRateLimitExceeded", "quotaExceeded", "RATE_LIMIT_EXCEEDED"):
		return &ProviderError{Status: resp.StatusCode, Retryable: true, Err: errors.New("rate limited")}
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode == 429 || resp.StatusCode >= 500,
		Err: fmt.Errorf("%s %s", g.id, strings.Join(slices.DeleteFunc(reasons, func(r string) bool { return r == "" }), ","))}
}
