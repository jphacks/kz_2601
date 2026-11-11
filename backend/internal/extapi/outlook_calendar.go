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
	"slices"
	"strconv"
	"strings"
	"time"
)

// Outlook calendar (#1495, Microsoft Graph v1.0): free (one `runs` a call),
// OAuth with PKCE on the common tenant (work / school and personal
// accounts), the person's default calendar only.
//
//	listEvents(timeMin?, timeMax?, maxResults?, q?)          Calendars.Read
//	createEvent(title, start, end?, description?, location?)  Calendars.ReadWrite
//
// The arguments and the rows are the same as googleCalendar's. offline_access
// is always asked for (the refresh token). Microsoft has no revoke endpoint:
// disconnecting deletes the stored token (ErrNoRevoke) and the person can
// remove the app in their Microsoft account's settings.
//
// Env: EXT_OUTLOOK_CALENDAR_ENABLED=1, EXT_OUTLOOK_CALENDAR_CLIENT_ID,
// EXT_OUTLOOK_CALENDAR_CLIENT_SECRET (Secret Manager). Off by default; not in
// the review build (NeedsOAuth).
const OutlookCalendar = "outlookCalendar"

const (
	outlookScopeRead    = "https://graph.microsoft.com/Calendars.Read"
	outlookScopeWrite   = "https://graph.microsoft.com/Calendars.ReadWrite"
	outlookScopeOffline = "offline_access"
	outlookGraphPrefix  = "https://graph.microsoft.com/"
	// outlookTZ is the Windows name of JST (Graph's dateTimeTimeZone).
	outlookTZ = "Tokyo Standard Time"
)

// outlookWire is where the provider talks to; tests point it at httptest.
type outlookWire struct {
	AuthURL  string // opened in the browser, not reached by the runner
	TokenURL string
	APIBase  string
	// Client is nil in production: NewHTTPClient on the provider's Hosts.
	Client *http.Client
	// Now is nil in production (time.Now).
	Now func() time.Time
}

var outlookCalendarWire = outlookWire{
	AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
	TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
	APIBase:  "https://graph.microsoft.com/v1.0",
}

var outlookCalendarHosts = []string{"https://login.microsoftonline.com", "https://graph.microsoft.com"}

func init() {
	register(Provider{ID: OutlookCalendar, Billing: BillingFree, NeedsOAuth: true, NameJa: "Outlook カレンダー", NameEn: "Outlook Calendar",
		Ops: []OpSpec{
			{Name: "listEvents", Scopes: []string{outlookScopeRead}, Args: []string{"timeMin", "timeMax", "maxResults", "q"}},
			{Name: "createEvent", Write: true, Scopes: []string{outlookScopeWrite}, Args: []string{"title", "start", "end", "description", "location"}},
		},
		Hosts: outlookCalendarHosts,
		New: func(sec Secrets) (map[string]Operation, error) {
			o, err := newOutlookCalendar(sec, outlookCalendarWire)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"listEvents": OperationFunc(o.listEvents), "createEvent": OperationFunc(o.createEvent)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) { return newOutlookCalendar(sec, outlookCalendarWire) },
	})
}

// outlookCalendar is the provider's ops and OAuth client.
type outlookCalendar struct {
	sec  Secrets
	wire outlookWire
	hc   *http.Client
	now  func() time.Time
}

func newOutlookCalendar(sec Secrets, w outlookWire) (*outlookCalendar, error) {
	if sec.ClientID == "" || sec.ClientSecret == "" {
		return nil, errors.New("EXT_OUTLOOK_CALENDAR_CLIENT_ID / _CLIENT_SECRET not set")
	}
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(outlookCalendarHosts, DefaultTimeout)
	}
	now := w.Now
	if now == nil {
		now = time.Now
	}
	return &outlookCalendar{sec: sec, wire: w, hc: hc, now: now}, nil
}

// --- OAuth ---

// outlookScopeParam is the scope parameter: the asked scopes plus
// offline_access (Microsoft sends a refresh token only with it).
func outlookScopeParam(scopes []string) string {
	out := slices.Clone(scopes)
	if !slices.Contains(out, outlookScopeOffline) {
		out = append(out, outlookScopeOffline)
	}
	return strings.Join(out, " ")
}

// AuthURL: Microsoft remembers what was consented to, so asking for
// Calendars.ReadWrite later keeps the reads (ReadWrite covers them).
func (o *outlookCalendar) AuthURL(state, codeChallenge, redirectURI string, scopes []string) string {
	v := url.Values{
		"client_id":             {o.sec.ClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"response_mode":         {"query"},
		"scope":                 {outlookScopeParam(scopes)},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}
	return o.wire.AuthURL + "?" + v.Encode()
}

func (o *outlookCalendar) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (Token, error) {
	return o.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {codeVerifier}, "redirect_uri": {redirectURI}}, Token{})
}

// Refresh exchanges the refresh token (Microsoft rotates it). The scope
// must be the granted ones or fewer, so the stored scopes are sent again.
func (o *outlookCalendar) Refresh(ctx context.Context, t Token) (Token, error) {
	if t.RefreshToken == "" {
		return Token{}, ErrInvalidGrant
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.RefreshToken}}
	if graph := slices.DeleteFunc(slices.Clone(t.Scopes), func(s string) bool { return !strings.HasPrefix(s, outlookGraphPrefix) }); len(graph) > 0 {
		form.Set("scope", outlookScopeParam(graph))
	}
	return o.token(ctx, form, t)
}

// Revoke: Microsoft identity has no endpoint that revokes one app's grant
// for a person, so only the stored token is deleted.
func (o *outlookCalendar) Revoke(context.Context, Token) error { return ErrNoRevoke }

func (o *outlookCalendar) token(ctx context.Context, form url.Values, prev Token) (Token, error) {
	form.Set("client_id", o.sec.ClientID)
	form.Set("client_secret", o.sec.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.wire.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := o.hc.Do(req)
	if err != nil {
		return Token{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := ReadBody(resp.Body)
	if err != nil {
		return Token{}, err
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		// interaction_required: conditional access or new consent; the
		// person has to sign in again either way
		if e.Error == "invalid_grant" || e.Error == "interaction_required" {
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
		return Token{}, errors.New("outlookCalendar: bad token answer")
	}
	t := Token{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken, Scopes: outlookScopes(r.Scope)}
	if r.ExpiresIn > 0 {
		t.Expiry = o.now().Add(time.Duration(r.ExpiresIn) * time.Second).UTC()
	}
	if len(t.Scopes) == 0 {
		t.Scopes = prev.Scopes
	}
	return t, nil
}

// outlookScopes keeps the granted Graph scopes in the op's form
// (https://graph.microsoft.com/Calendars.Read); Microsoft may answer the
// short name. openid / profile / offline_access are dropped.
func outlookScopes(s string) []string {
	var out []string
	for _, f := range strings.Fields(s) {
		switch {
		case strings.HasPrefix(f, outlookGraphPrefix):
		case strings.Contains(f, "://") || slices.Contains([]string{"openid", "profile", "email", outlookScopeOffline}, f):
			continue
		default:
			f = outlookGraphPrefix + f
		}
		if !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

// outlookHasScope: the token's granted scopes cover want (unknown scopes
// pass; Graph then answers 403 and the person connects again).
func outlookHasScope(t *Token, want string) bool {
	if t == nil || len(t.Scopes) == 0 {
		return true
	}
	ok := []string{want, outlookScopeWrite}
	return slices.ContainsFunc(t.Scopes, func(s string) bool {
		return slices.ContainsFunc(ok, func(o string) bool { return strings.EqualFold(s, o) })
	})
}

// --- ops ---

type outlookWhen struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

type outlookEvent struct {
	ID       string `json:"id"`
	Subject  string `json:"subject"`
	IsAllDay bool   `json:"isAllDay"`
	WebLink  string `json:"webLink"`
	Location struct {
		DisplayName string `json:"displayName"`
	} `json:"location"`
	Start outlookWhen `json:"start"`
}

// outlookLinkHosts are the Outlook on the web hosts a row may link to.
var outlookLinkHosts = []string{"outlook.live.com", "outlook.office365.com", "outlook.office.com"}

// start is the event's start as RFC 3339 in JST (all day: YYYY-MM-DD).
// Graph answers in outlookTZ (Prefer header) without an offset.
func (e outlookEvent) start() string {
	v := e.Start.DateTime
	if i := strings.IndexByte(v, '.'); i > 0 {
		v = v[:i] // 7 fraction digits
	}
	loc := gcalJST
	if strings.EqualFold(e.Start.TimeZone, "UTC") {
		loc = time.UTC
	}
	t, err := time.ParseInLocation("2006-01-02T15:04:05", v, loc)
	if err != nil {
		return ""
	}
	if e.IsAllDay {
		return t.Format(time.DateOnly)
	}
	return t.In(gcalJST).Format(time.RFC3339)
}

func (e outlookEvent) row() Row {
	r := Row{Title: e.Subject, Subtitle: e.Location.DisplayName, Value: e.start()}
	if r.Title == "" {
		r.Title = gcalNoTitle
	}
	if u, err := url.Parse(e.WebLink); err == nil && u.Scheme == "https" && slices.Contains(outlookLinkHosts, u.Host) {
		r.URL = e.WebLink
	}
	return r
}

// listEvents lists the default calendar's events in [timeMin, timeMax)
// (default: now → 7 days), one page, start time order, repeats expanded
// (calendarView). q keeps the events whose subject or location has it
// (calendarView has no search; up to 50 events are read and filtered).
func (o *outlookCalendar) listEvents(ctx context.Context, in Input) (Output, error) {
	if !outlookHasScope(in.Token, outlookScopeRead) {
		return Output{}, ErrNotConnected
	}
	from := gcalTime{t: o.now()}
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
	top := n
	if q != "" {
		top = gcalMaxResults
	}
	v := url.Values{
		"startDateTime": {from.t.Format(time.RFC3339)},
		"endDateTime":   {to.t.Format(time.RFC3339)},
		"$orderby":      {"start/dateTime"},
		"$top":          {strconv.Itoa(top)},
		"$select":       {"id,subject,location,start,isAllDay,webLink"},
	}
	var r struct {
		Value []outlookEvent `json:"value"`
	}
	if err := o.do(ctx, in.Token, http.MethodGet, "/me/calendarView?"+v.Encode(), nil, &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, min(len(r.Value), n))}
	lq := strings.ToLower(q)
	for _, e := range r.Value {
		if len(out.Rows) == n {
			break
		}
		if q != "" && !strings.Contains(strings.ToLower(e.Subject), lq) && !strings.Contains(strings.ToLower(e.Location.DisplayName), lq) {
			continue
		}
		out.Rows = append(out.Rows, e.row())
	}
	return out, nil
}

// createEvent adds an event to the default calendar. start YYYY-MM-DD is an
// all-day event (end: the last day, default the same day); a time without
// end lasts an hour. transactionId comes from the request id, so a retried
// request makes no second event (Graph keeps the first).
func (o *outlookCalendar) createEvent(ctx context.Context, in Input) (Output, error) {
	if !outlookHasScope(in.Token, outlookScopeWrite) {
		return Output{}, ErrNotConnected
	}
	title, err := gcalText("title", in.Args["title"], 255, true)
	if err != nil {
		return Output{}, err
	}
	desc, err := gcalText("description", in.Args["description"], 8000, false)
	if err != nil {
		return Output{}, err
	}
	loc, err := gcalText("location", in.Args["location"], 255, false)
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
	const layout = "2006-01-02T15:04:05"
	var s, e outlookWhen
	if start.allDay {
		if end.t.Before(start.t) || end.t.Sub(start.t) > gcalMaxRange {
			return Output{}, fmt.Errorf("%w: end must be on or after start and within 366 days", ErrUnknown)
		}
		// all day: midnights in one time zone, the end exclusive
		s = outlookWhen{DateTime: start.t.Format(layout), TimeZone: outlookTZ}
		e = outlookWhen{DateTime: end.t.AddDate(0, 0, 1).Format(layout), TimeZone: outlookTZ}
	} else {
		if !end.t.After(start.t) || end.t.Sub(start.t) > gcalMaxRange {
			return Output{}, fmt.Errorf("%w: end must be after start and within 366 days", ErrUnknown)
		}
		s = outlookWhen{DateTime: start.t.In(gcalJST).Format(layout), TimeZone: outlookTZ}
		e = outlookWhen{DateTime: end.t.In(gcalJST).Format(layout), TimeZone: outlookTZ}
	}
	type text struct {
		ContentType string `json:"contentType"`
		Content     string `json:"content"`
	}
	type place struct {
		DisplayName string `json:"displayName"`
	}
	body := struct {
		Subject       string      `json:"subject"`
		Body          *text       `json:"body,omitempty"`
		Location      *place      `json:"location,omitempty"`
		Start         outlookWhen `json:"start"`
		End           outlookWhen `json:"end"`
		IsAllDay      bool        `json:"isAllDay"`
		TransactionID string      `json:"transactionId,omitempty"`
	}{Subject: title, Start: s, End: e, IsAllDay: start.allDay, TransactionID: outlookTransactionID(in.RequestID)}
	if desc != "" {
		body.Body = &text{ContentType: "text", Content: desc}
	}
	if loc != "" {
		body.Location = &place{DisplayName: loc}
	}
	b, err := json.Marshal(body)
	if err != nil {
		return Output{}, err
	}
	var ev outlookEvent
	err = o.do(ctx, in.Token, http.MethodPost, "/me/events", b, &ev)
	var pe *ProviderError
	if errors.As(err, &pe) && pe.Status == http.StatusConflict && body.TransactionID != "" {
		// the same request made it already
		return Output{Item: &Row{Title: title, Value: in.Args["start"]}}, nil
	}
	if err != nil {
		return Output{}, err
	}
	row := ev.row()
	return Output{ResultID: ev.ID, Item: &row}, nil
}

// outlookTransactionID turns a request id into an event's transactionId.
func outlookTransactionID(requestID string) string {
	if requestID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("tailorit:" + requestID))
	return "tailorit-" + hex.EncodeToString(sum[:16])
}

// do calls Graph with the person's token and decodes the answer. Times come
// back in JST (Prefer: outlook.timezone). 401 is a token Microsoft no longer
// takes (ErrInvalidGrant: the connection turns to error); 403 access denied
// is a missing scope (ErrNotConnected: connect again with it); 429 / 5xx
// are retryable provider errors.
func (o *outlookCalendar) do(ctx context.Context, t *Token, method, path string, body []byte, out any) error {
	if t == nil || t.AccessToken == "" {
		return ErrNotConnected
	}
	req, err := http.NewRequestWithContext(ctx, method, o.wire.APIBase+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+t.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Prefer", `outlook.timezone="`+outlookTZ+`"`)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := o.hc.Do(req)
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
			return fmt.Errorf("outlookCalendar: bad answer: %w", err)
		}
		return nil
	}
	var e struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(b, &e)
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrInvalidGrant
	case resp.StatusCode == http.StatusForbidden && slices.Contains([]string{"ErrorAccessDenied", "AccessDenied", "Authorization_RequestDenied"}, e.Error.Code):
		return ErrNotConnected
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode == 429 || resp.StatusCode >= 500,
		Err: fmt.Errorf("outlookCalendar %s", e.Error.Code)}
}
