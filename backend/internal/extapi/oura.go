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
	"sort"
	"strconv"
	"strings"
	"time"
)

// Oura (#1668, API v2): free (one `runs` a call), OAuth with PKCE. Health
// data, so read only and kept as small as it can be.
//
//	dailySleep(from?, to?)      daily
//	dailyReadiness(from?, to?)  daily
//	dailyActivity(from?, to?)   daily
//
// daily is Oura's narrowest scope for the daily summaries (no email,
// personal, heartrate ...). AuthURL always sends it: Oura asks for every
// scope when none is given.
//
// One row a day (title = the day, value = the score, subtitle = a few key
// numbers). Only the fields shown are asked for (Oura's fields=).
//
// No cache (CacheTTL 0, as every OAuth op) and no Cells (#1712's scheduled
// import would store the values): the answer only passes through the runner
// to the device. Never log a value; errors carry the status only.
//
// Oura's API Agreement (2026-06-08) limits storing, caching, AI use and
// sharing of User Data; a person must read it before this is turned on.
//
// Env: EXT_OURA_ENABLED=1, EXT_OURA_CLIENT_ID, EXT_OURA_CLIENT_SECRET
// (Secret Manager). Off by default; not in the review build (NeedsOAuth).
const Oura = "oura"

const ouraScopeDaily = "daily"

// ouraWire is where the provider talks to; tests point it at httptest.
type ouraWire struct {
	AuthURL   string // opened in the browser, not reached by the runner
	TokenURL  string
	RevokeURL string
	APIBase   string
	// Client is nil in production: NewHTTPClient on the provider's Hosts.
	Client *http.Client
	// Now is nil in production (time.Now).
	Now func() time.Time
}

var ouraWireDefault = ouraWire{
	AuthURL:   "https://cloud.ouraring.com/oauth/authorize",
	TokenURL:  "https://api.ouraring.com/oauth/token",
	RevokeURL: "https://api.ouraring.com/oauth/revoke",
	APIBase:   "https://api.ouraring.com/v2/usercollection",
}

// ouraHosts: the token, revoke and API are on one host (the consent page
// is opened by the device, not the runner).
var ouraHosts = []string{"https://api.ouraring.com"}

func init() {
	args := []string{"from", "to"}
	register(Provider{ID: Oura, Billing: BillingFree, NeedsOAuth: true, NameJa: "Oura", NameEn: "Oura",
		Ops: []OpSpec{
			{Name: "dailySleep", Scopes: []string{ouraScopeDaily}, Args: args, Check: checkOuraDates},
			{Name: "dailyReadiness", Scopes: []string{ouraScopeDaily}, Args: args, Check: checkOuraDates},
			{Name: "dailyActivity", Scopes: []string{ouraScopeDaily}, Args: args, Check: checkOuraDates},
		},
		Hosts: ouraHosts,
		New: func(sec Secrets) (map[string]Operation, error) {
			c, err := newOura(sec, ouraWireDefault)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"dailySleep": OperationFunc(c.dailySleep), "dailyReadiness": OperationFunc(c.dailyReadiness),
				"dailyActivity": OperationFunc(c.dailyActivity)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) { return newOura(sec, ouraWireDefault) },
	})
}

// oura is the provider's ops and OAuth client.
type oura struct {
	sec  Secrets
	wire ouraWire
	hc   *http.Client
	now  func() time.Time
}

func newOura(sec Secrets, w ouraWire) (*oura, error) {
	if sec.ClientID == "" || sec.ClientSecret == "" {
		return nil, errors.New("EXT_OURA_CLIENT_ID / _CLIENT_SECRET not set")
	}
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(ouraHosts, DefaultTimeout)
	}
	now := w.Now
	if now == nil {
		now = time.Now
	}
	return &oura{sec: sec, wire: w, hc: hc, now: now}, nil
}

// --- OAuth ---

// AuthURL: scopes are space separated. Only daily is ever sent, and always
// (an empty scope is every scope at Oura).
func (c *oura) AuthURL(state, codeChallenge, redirectURI string, _ []string) string {
	v := url.Values{
		"client_id":             {c.sec.ClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {ouraScopeDaily},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}
	return c.wire.AuthURL + "?" + v.Encode()
}

func (c *oura) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (Token, error) {
	return c.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {codeVerifier}, "redirect_uri": {redirectURI}}, Token{})
}

// Refresh exchanges the refresh token. Oura's refresh token is single use:
// the new one replaces it.
func (c *oura) Refresh(ctx context.Context, t Token) (Token, error) {
	if t.RefreshToken == "" {
		return Token{}, ErrInvalidGrant
	}
	return c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.RefreshToken}}, t)
}

// Revoke revokes the access token at Oura (/oauth/revoke?access_token=).
// An expired one is renewed first; a refused refresh means it is gone.
func (c *oura) Revoke(ctx context.Context, t Token) error {
	if t.expired(c.now()) && t.RefreshToken != "" {
		nt, err := c.Refresh(ctx, t)
		if errors.Is(err, ErrInvalidGrant) {
			return nil
		}
		if err != nil {
			return err
		}
		t = nt
	}
	if t.AccessToken == "" {
		return nil
	}
	u := c.wire.RevokeURL + "?" + url.Values{"access_token": {t.AccessToken}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return errors.New("oura: revoke failed") // the URL holds the token
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = ReadBody(resp.Body)
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusUnauthorized:
		return nil // already gone at Oura
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500, Err: errors.New("revoke refused")}
}

func (c *oura) token(ctx context.Context, form url.Values, prev Token) (Token, error) {
	form.Set("client_id", c.sec.ClientID)
	form.Set("client_secret", c.sec.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.wire.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
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
		if e.Error == "invalid_grant" {
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
		return Token{}, errors.New("oura: bad token answer")
	}
	t := Token{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken, Scopes: strings.Fields(r.Scope)}
	if r.ExpiresIn > 0 {
		t.Expiry = c.now().Add(time.Duration(r.ExpiresIn) * time.Second).UTC()
	}
	if t.RefreshToken == "" {
		t.RefreshToken = prev.RefreshToken
	}
	if len(t.Scopes) == 0 {
		t.Scopes = prev.Scopes // Oura's token answer has no scope (oauth.go fills the asked ones)
	}
	return t, nil
}

// --- ops ---

const (
	ouraDefaultDays = 7
	ouraMaxDays     = 31
	ouraMaxPages    = 3
)

// ouraZone is the day the dates are read in.
var ouraZone = time.FixedZone("JST", 9*60*60)

var ouraNextRe = regexp.MustCompile(`^[A-Za-z0-9_=+/.-]{1,512}$`)

// ouraDates reads from / to (YYYY-MM-DD, JST): default the last 7 days to
// today; to is not after today; at most 31 days.
func ouraDates(a map[string]string, now time.Time) (time.Time, time.Time, error) {
	today := now.In(ouraZone)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, ouraZone)
	parse := func(name, v string) (time.Time, error) {
		d, err := time.ParseInLocation(time.DateOnly, v, ouraZone)
		if err != nil {
			return time.Time{}, fmt.Errorf("%w: %s must be YYYY-MM-DD", ErrUnknown, name)
		}
		return d, nil
	}
	to := today
	if v := a["to"]; v != "" {
		d, err := parse("to", v)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		to = d
	}
	from := to.AddDate(0, 0, -(ouraDefaultDays - 1))
	if v := a["from"]; v != "" {
		d, err := parse("from", v)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		from = d
	}
	switch {
	case to.After(today):
		return time.Time{}, time.Time{}, fmt.Errorf("%w: to is after today", ErrUnknown)
	case from.After(to):
		return time.Time{}, time.Time{}, fmt.Errorf("%w: from is after to", ErrUnknown)
	case from.Year() < 2015:
		return time.Time{}, time.Time{}, fmt.Errorf("%w: from is before 2015", ErrUnknown)
	case from.AddDate(0, 0, ouraMaxDays-1).Before(to):
		return time.Time{}, time.Time{}, fmt.Errorf("%w: from - to is over %d days", ErrUnknown, ouraMaxDays)
	}
	return from, to, nil
}

func checkOuraDates(a map[string]string) error {
	_, _, err := ouraDates(a, time.Now())
	return err
}

// ouraDoc is one daily document; only the fields an op asks for are set.
type ouraDoc struct {
	Day                  string          `json:"day"`
	Score                *int            `json:"score"`
	Contributors         map[string]*int `json:"contributors"`
	TemperatureDeviation *float64        `json:"temperature_deviation"`
	Steps                *int            `json:"steps"`
	ActiveCalories       *int            `json:"active_calories"`
}

// ouraPart is one number of the subtitle.
type ouraPart struct {
	label string
	value func(d ouraDoc) string
}

func ouraContributor(key string) func(d ouraDoc) string {
	return func(d ouraDoc) string { return ouraInt(d.Contributors[key]) }
}

func ouraInt(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}

// ouraKind is one daily collection and how a day of it is shown.
type ouraKind struct {
	path   string
	fields string
	parts  []ouraPart
}

// The contributors are Oura's 1-100 scores of each part, not minutes.
var (
	ouraSleep = ouraKind{path: "/daily_sleep", fields: "day,score,contributors", parts: []ouraPart{
		{"合計睡眠", ouraContributor("total_sleep")},
		{"効率", ouraContributor("efficiency")},
		{"深い睡眠", ouraContributor("deep_sleep")},
		{"レム睡眠", ouraContributor("rem_sleep")},
	}}
	ouraReadiness = ouraKind{path: "/daily_readiness", fields: "day,score,contributors,temperature_deviation", parts: []ouraPart{
		{"体温", func(d ouraDoc) string {
			if d.TemperatureDeviation == nil {
				return ""
			}
			return fmt.Sprintf("%+.1f℃", *d.TemperatureDeviation)
		}},
		{"HRV バランス", ouraContributor("hrv_balance")},
		{"安静時心拍", ouraContributor("resting_heart_rate")},
		{"回復", ouraContributor("recovery_index")},
	}}
	ouraActivity = ouraKind{path: "/daily_activity", fields: "day,score,steps,active_calories", parts: []ouraPart{
		{"歩数", func(d ouraDoc) string { return ouraInt(d.Steps) }},
		{"活動", func(d ouraDoc) string {
			if d.ActiveCalories == nil {
				return ""
			}
			return strconv.Itoa(*d.ActiveCalories) + " kcal"
		}},
	}}
)

func (k ouraKind) row(d ouraDoc) Row {
	var sub []string
	for _, p := range k.parts {
		if v := p.value(d); v != "" {
			sub = append(sub, p.label+" "+v)
		}
	}
	return Row{Title: d.Day, Value: ouraInt(d.Score), Subtitle: strings.Join(sub, " · ")}
}

// dailySleep is the sleep score of each day (subtitle: the parts' scores).
func (c *oura) dailySleep(ctx context.Context, in Input) (Output, error) {
	return c.daily(ctx, in, ouraSleep)
}

// dailyReadiness is the readiness score of each day (subtitle: the body
// temperature's deviation and the parts' scores).
func (c *oura) dailyReadiness(ctx context.Context, in Input) (Output, error) {
	return c.daily(ctx, in, ouraReadiness)
}

// dailyActivity is the activity score of each day (subtitle: steps and
// active calories).
func (c *oura) dailyActivity(ctx context.Context, in Input) (Output, error) {
	return c.daily(ctx, in, ouraActivity)
}

// daily reads one collection from from to to: one row a day, oldest first.
// end_date is sent a day later and the days outside are cut, whichever way
// Oura counts it.
func (c *oura) daily(ctx context.Context, in Input, k ouraKind) (Output, error) {
	from, to, err := ouraDates(in.Args, c.now())
	if err != nil {
		return Output{}, err
	}
	lo, hi := from.Format(time.DateOnly), to.Format(time.DateOnly)
	q := url.Values{"start_date": {lo}, "end_date": {to.AddDate(0, 0, 1).Format(time.DateOnly)}, "fields": {k.fields}}
	byDay := map[string]ouraDoc{}
	for range ouraMaxPages {
		var r struct {
			Data      []ouraDoc `json:"data"`
			NextToken *string   `json:"next_token"`
		}
		if err := c.get(ctx, in, k.path+"?"+q.Encode(), &r); err != nil {
			return Output{}, err
		}
		for _, d := range r.Data {
			if d.Day >= lo && d.Day <= hi && len(d.Day) == len(lo) {
				byDay[d.Day] = d
			}
		}
		if r.NextToken == nil || *r.NextToken == "" || !ouraNextRe.MatchString(*r.NextToken) {
			break
		}
		q.Set("next_token", *r.NextToken)
	}
	days := make([]string, 0, len(byDay))
	for d := range byDay {
		days = append(days, d)
	}
	sort.Strings(days)
	out := Output{Rows: make([]Row, 0, len(days))}
	for _, d := range days {
		out.Rows = append(out.Rows, k.row(byDay[d]))
	}
	return out, nil
}

// get calls the API with the person's token. 401 is a token Oura no longer
// takes (ErrInvalidGrant: the connection turns to error); 403 is Oura's "the
// membership has ended" (connecting again does not help); 429 / 5xx are
// retryable. No error carries the answer's body.
func (c *oura) get(ctx context.Context, in Input, path string, out any) error {
	if in.Token == nil || in.Token.AccessToken == "" {
		return ErrNotConnected
	}
	if len(in.Token.Scopes) > 0 && !slices.Contains(in.Token.Scopes, ouraScopeDaily) {
		return ErrNotConnected
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.wire.APIBase+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+in.Token.AccessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := ReadBody(resp.Body)
	if err != nil {
		return err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		if err := json.Unmarshal(b, out); err != nil {
			return errors.New("oura: bad answer")
		}
		return nil
	case http.StatusUnauthorized:
		return ErrInvalidGrant
	case http.StatusForbidden:
		return &ProviderError{Status: resp.StatusCode, Err: errors.New("oura: no access (membership ended?)")}
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode == 429 || resp.StatusCode >= 500,
		Err: errors.New("oura refused")}
}
