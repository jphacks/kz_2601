package extapi

import (
	"cmp"
	"context"
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

// Withings (#1669): the person's body scale and blood pressure monitor,
// read only. Free (one `runs` a call), per-user OAuth 2.0.
//
//	weight(days?, limit?)          Rows: weighings, newest first     user.metrics
//	bloodPressure(days?, limit?)   Rows: BP readings, newest first   user.metrics
//
// days 1-90 (default 30) is how far back; limit 1-50 (default 20). Only real
// measures (category 1) are read; a measure the scale could not tell apart
// from another family member's (attrib 1) is left out. Health data: never
// cached (an OAuth provider never is) and never logged.
//
// Withings answers HTTP 200 with its own status in the body (0 = ok). It
// does not take PKCE (the client secret stays on the runner). Revoking needs
// the Withings user id and a signed nonce; Token has no place for the user id,
// so Revoke is ErrNoRevoke (the tokens are deleted; the person removes the
// app in Withings' settings).
//
// Env: EXT_WITHINGS_ENABLED=1, EXT_WITHINGS_CLIENT_ID, EXT_WITHINGS_CLIENT_SECRET
// (Secret Manager). Off by default; not in the review build (NeedsOAuth).
const Withings = "withings"

const withingsScopeMetrics = "user.metrics"

// Withings measure types (getmeas meastype).
const (
	withingsTypeWeight    = 1
	withingsTypeFatRatio  = 6
	withingsTypeDiastolic = 9
	withingsTypeSystolic  = 10
	withingsTypePulse     = 11
)

// withingsWire is where the provider talks to; tests point it at httptest.
type withingsWire struct {
	AuthURL  string // opened in the browser, not reached by the runner
	TokenURL string
	APIBase  string
	// Client is nil in production: NewHTTPClient on the provider's Hosts.
	Client *http.Client
	// Now is nil in production (time.Now).
	Now func() time.Time
}

var withingsWireDefault = withingsWire{
	AuthURL:  "https://account.withings.com/oauth2_user/authorize2",
	TokenURL: "https://wbsapi.withings.net/v2/oauth2",
	APIBase:  "https://wbsapi.withings.net",
}

var withingsHosts = []string{"https://wbsapi.withings.net"}

func init() {
	register(Provider{ID: Withings, Billing: BillingFree, NeedsOAuth: true, NameJa: "Withings", NameEn: "Withings",
		Ops: []OpSpec{
			{Name: "weight", Scopes: []string{withingsScopeMetrics}, Args: []string{"days", "limit"}, Check: checkWithingsRange},
			{Name: "bloodPressure", Scopes: []string{withingsScopeMetrics}, Args: []string{"days", "limit"}, Check: checkWithingsRange},
		},
		Hosts: withingsHosts,
		New: func(sec Secrets) (map[string]Operation, error) {
			c, err := newWithings(sec, withingsWireDefault)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"weight": OperationFunc(c.weight), "bloodPressure": OperationFunc(c.bloodPressure)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) { return newWithings(sec, withingsWireDefault) },
	})
}

type withings struct {
	sec  Secrets
	wire withingsWire
	hc   *http.Client
	now  func() time.Time
}

func newWithings(sec Secrets, w withingsWire) (*withings, error) {
	if sec.ClientID == "" || sec.ClientSecret == "" {
		return nil, errors.New("EXT_WITHINGS_CLIENT_ID / _CLIENT_SECRET not set")
	}
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(withingsHosts, DefaultTimeout)
	}
	now := w.Now
	if now == nil {
		now = time.Now
	}
	return &withings{sec: sec, wire: w, hc: hc, now: now}, nil
}

// --- OAuth ---

// AuthURL: scopes are comma separated. The PKCE challenge is not sent
// (Withings does not take it).
func (c *withings) AuthURL(state, _, redirectURI string, scopes []string) string {
	v := url.Values{
		"client_id":     {c.sec.ClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {strings.Join(scopes, ",")},
		"state":         {state},
	}
	return c.wire.AuthURL + "?" + v.Encode()
}

func (c *withings) Exchange(ctx context.Context, code, _, redirectURI string) (Token, error) {
	return c.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirectURI}}, Token{})
}

// Refresh exchanges the refresh token (Withings rotates it).
func (c *withings) Refresh(ctx context.Context, t Token) (Token, error) {
	if t.RefreshToken == "" {
		return Token{}, ErrInvalidGrant
	}
	return c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.RefreshToken}}, t)
}

// Revoke: see the file comment.
func (c *withings) Revoke(context.Context, Token) error { return ErrNoRevoke }

// withingsAnswer is every Withings answer: status 0 is ok.
type withingsAnswer struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

// withingsAuthFailed are the statuses of a token Withings no longer takes.
var withingsAuthFailed = []int{100, 101, 102, 200, 401}

// withingsStatusErr turns a non-zero status into an error. On the token
// endpoint, 503 (invalid params) is a code or refresh token Withings does not
// know: the other params are fixed.
func withingsStatusErr(status int, tokenEndpoint bool) error {
	switch {
	case slices.Contains(withingsAuthFailed, status), tokenEndpoint && status == 503:
		return ErrInvalidGrant
	case status == 601: // too many requests
		return &ProviderError{Status: http.StatusTooManyRequests, Retryable: true, Err: errors.New("withings: too many requests")}
	case status == 2555: // unknown error on Withings' side
		return &ProviderError{Status: http.StatusBadGateway, Retryable: true, Err: errors.New("withings: unknown error")}
	}
	return &ProviderError{Status: http.StatusBadGateway, Err: fmt.Errorf("withings status %d", status)}
}

// post sends a form and reads the answer's body into out.
func (c *withings) post(ctx context.Context, rawURL string, form url.Values, bearer string, tokenEndpoint bool, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return errors.New("withings: building the request failed")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
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
	switch {
	case resp.StatusCode == http.StatusUnauthorized && !tokenEndpoint:
		return ErrInvalidGrant
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return &ProviderError{Status: resp.StatusCode, Retryable: true, Err: errors.New("withings unavailable")}
	case resp.StatusCode != http.StatusOK:
		return &ProviderError{Status: resp.StatusCode, Err: errors.New("withings refused")}
	}
	var a withingsAnswer
	if err := json.Unmarshal(b, &a); err != nil {
		return &ProviderError{Status: resp.StatusCode, Err: errors.New("withings: bad answer")}
	}
	if a.Status != 0 {
		return withingsStatusErr(a.Status, tokenEndpoint)
	}
	if err := json.Unmarshal(a.Body, out); err != nil {
		return &ProviderError{Status: resp.StatusCode, Err: errors.New("withings: bad answer body")}
	}
	return nil
}

func (c *withings) token(ctx context.Context, form url.Values, prev Token) (Token, error) {
	form.Set("action", "requesttoken")
	form.Set("client_id", c.sec.ClientID)
	form.Set("client_secret", c.sec.ClientSecret)
	var r struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := c.post(ctx, c.wire.TokenURL, form, "", true, &r); err != nil {
		return Token{}, err
	}
	if r.AccessToken == "" {
		return Token{}, errors.New("withings: bad token answer")
	}
	t := Token{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken,
		Scopes: strings.FieldsFunc(r.Scope, func(r rune) bool { return r == ',' || r == ' ' })}
	if r.ExpiresIn > 0 {
		t.Expiry = c.now().Add(time.Duration(r.ExpiresIn) * time.Second).UTC()
	}
	if t.RefreshToken == "" {
		t.RefreshToken = prev.RefreshToken
	}
	if len(t.Scopes) == 0 {
		t.Scopes = prev.Scopes
	}
	return t, nil
}

// --- ops ---

// withingsMaxPages bounds the pages read when Withings answers more = 1.
const withingsMaxPages = 3

var withingsJST = time.FixedZone("JST", 9*60*60)

func withingsRange(a map[string]string) (days, limit int, err error) {
	if days, err = argInt(a, "days", 30, 1, 90); err != nil {
		return 0, 0, err
	}
	if limit, err = argInt(a, "limit", 20, 1, 50); err != nil {
		return 0, 0, err
	}
	return days, limit, nil
}

func checkWithingsRange(a map[string]string) error {
	_, _, err := withingsRange(a)
	return err
}

type withingsMeasure struct {
	Value int64 `json:"value"`
	Type  int   `json:"type"`
	Unit  int   `json:"unit"`
}

type withingsGroup struct {
	Date     int64             `json:"date"`
	Attrib   int               `json:"attrib"`
	Category int               `json:"category"`
	Measures []withingsMeasure `json:"measures"`
}

// value is the group's measure of type typ as an exact decimal.
func (g withingsGroup) value(typ int) (string, bool) {
	for _, m := range g.Measures {
		if m.Type == typ {
			return withingsDecimal(m.Value, m.Unit)
		}
	}
	return "", false
}

// withingsDecimal writes value × 10^unit exactly (65750, -3 → "65.75"),
// trailing zeros dropped: the number as the device gave it.
func withingsDecimal(value int64, unit int) (string, bool) {
	if unit < -9 || unit > 9 || value < 0 {
		return "", false
	}
	if unit >= 0 {
		return strconv.FormatInt(value, 10) + strings.Repeat("0", unit), true
	}
	s := strconv.FormatInt(value, 10)
	n := -unit
	if len(s) <= n {
		s = strings.Repeat("0", n-len(s)+1) + s
	}
	i, f := s[:len(s)-n], strings.TrimRight(s[len(s)-n:], "0")
	if f == "" {
		return i, true
	}
	return i + "." + f, true
}

// measures reads the person's real measures of types from the last days,
// newest first, at most limit groups that have need.
func (c *withings) measures(ctx context.Context, in Input, types []int, need int) ([]withingsGroup, int, error) {
	if in.Token == nil || in.Token.AccessToken == "" {
		return nil, 0, ErrNotConnected
	}
	days, limit, err := withingsRange(in.Args)
	if err != nil {
		return nil, 0, err
	}
	end := c.now()
	ts := make([]string, len(types))
	for i, t := range types {
		ts[i] = strconv.Itoa(t)
	}
	form := url.Values{
		"action":    {"getmeas"},
		"meastypes": {strings.Join(ts, ",")},
		"category":  {"1"},
		"startdate": {strconv.FormatInt(end.AddDate(0, 0, -days).Unix(), 10)},
		"enddate":   {strconv.FormatInt(end.Unix(), 10)},
	}
	var groups []withingsGroup
	for page := 0; page < withingsMaxPages; page++ {
		var r struct {
			MeasureGrps []withingsGroup `json:"measuregrps"`
			More        json.RawMessage `json:"more"`
			Offset      int             `json:"offset"`
		}
		if err := c.post(ctx, c.wire.APIBase+"/measure", form, in.Token.AccessToken, false, &r); err != nil {
			return nil, 0, err
		}
		for _, g := range r.MeasureGrps {
			// attrib 1: the device could not tell whose measure it is
			if _, ok := g.value(need); ok && g.Category == 1 && g.Attrib != 1 {
				groups = append(groups, g)
			}
		}
		more := strings.TrimSpace(string(r.More))
		if more == "" || more == "0" || more == "false" || r.Offset <= 0 {
			break
		}
		form.Set("offset", strconv.Itoa(r.Offset))
	}
	slices.SortStableFunc(groups, func(a, b withingsGroup) int { return cmp.Compare(b.Date, a.Date) })
	return groups, limit, nil
}

func withingsWhen(unix int64) (title, cell string) {
	t := time.Unix(unix, 0).In(withingsJST)
	return t.Format("2006/1/2 15:04"), t.Format(time.RFC3339)
}

// weight: title = when (JST), value = "65.75 kg", subtitle = body fat when
// the scale measured it. Cells: when (RFC 3339), kg, fat %.
func (c *withings) weight(ctx context.Context, in Input) (Output, error) {
	groups, limit, err := c.measures(ctx, in, []int{withingsTypeWeight, withingsTypeFatRatio}, withingsTypeWeight)
	if err != nil {
		return Output{}, err
	}
	out := Output{Rows: []Row{}, Cells: [][]string{}}
	for _, g := range groups {
		if len(out.Rows) == limit {
			break
		}
		kg, _ := g.value(withingsTypeWeight)
		title, when := withingsWhen(g.Date)
		r := Row{Title: title, Value: kg + " kg"}
		fat, ok := g.value(withingsTypeFatRatio)
		if ok {
			r.Subtitle = "体脂肪率 " + fat + "%"
		}
		out.Rows = append(out.Rows, r)
		out.Cells = append(out.Cells, []string{when, kg, fat})
	}
	return out, nil
}

// bloodPressure: title = when (JST), value = "128/82 mmHg", subtitle =
// pulse when measured. A reading without both numbers is left out. Cells:
// when (RFC 3339), systolic, diastolic, pulse.
func (c *withings) bloodPressure(ctx context.Context, in Input) (Output, error) {
	groups, limit, err := c.measures(ctx, in, []int{withingsTypeDiastolic, withingsTypeSystolic, withingsTypePulse}, withingsTypeSystolic)
	if err != nil {
		return Output{}, err
	}
	out := Output{Rows: []Row{}, Cells: [][]string{}}
	for _, g := range groups {
		if len(out.Rows) == limit {
			break
		}
		sys, _ := g.value(withingsTypeSystolic)
		dia, ok := g.value(withingsTypeDiastolic)
		if !ok {
			continue
		}
		title, when := withingsWhen(g.Date)
		r := Row{Title: title, Value: sys + "/" + dia + " mmHg"}
		pulse, ok := g.value(withingsTypePulse)
		if ok {
			r.Subtitle = "脈拍 " + pulse
		}
		out.Rows = append(out.Rows, r)
		out.Cells = append(out.Cells, []string{when, sys, dia, pulse})
	}
	return out, nil
}
