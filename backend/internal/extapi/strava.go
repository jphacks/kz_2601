package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Strava (API v3): the person's own activities and recent totals. Free
// (one `runs` a call), read only, per-user OAuth 2.0 (no PKCE: the client
// secret stays on the runner).
//
//	activities(limit?, after?)   Rows: activities, newest first   activity:read
//	stats()                      Rows: the last 4 weeks' ride / run / swim totals   activity:read
//
// limit is 1-50 (default 20); after is YYYY-MM-DD (JST): activities that
// started on or after it. activity:read reads activities the person set to
// Everyone or Followers (activity:read_all is not asked for). The consent
// page is asked for `read` too (Strava's base scope).
//
// Strava's quota is per app, not per person (100 requests a 15 minutes and
// 1,000 a day on the owner's registration): set EXT_GLOBAL_DAILY_STRAVA
// (an OAuth provider has no environment-wide day by default) and keep the
// per-user limits small. A 429 pauses only the person it came from.
//
// Terms (docs/backend/features/external-api-terms.md): "Powered by Strava"
// (StravaAttribution) is shown with the data; the data is the person's own
// and is not fed to the AI. Whether to ship it is the owner's decision
// (default off; decisions/external-apis.md says not to add it, #1670).
//
// Env: EXT_STRAVA_ENABLED=1, EXT_STRAVA_CLIENT_ID, EXT_STRAVA_CLIENT_SECRET
// (Secret Manager). Off by default; not in the review build (NeedsOAuth).
const (
	Strava = "strava"
	// StravaAttribution is the credit shown under the rows (Strava's API
	// agreement asks for it).
	StravaAttribution = "Powered by Strava"
)

const (
	stravaScopeRead     = "read"
	stravaScopeActivity = "activity:read"
)

// stravaWire is where the provider talks to; tests point it at httptest.
type stravaWire struct {
	AuthURL       string // opened in the browser, not reached by the runner
	TokenURL      string
	DeauthURL     string
	APIBase       string
	ActivityLinks string // the activity page's prefix
	// Client is nil in production: NewHTTPClient on the provider's Hosts.
	Client *http.Client
	// Now is nil in production (time.Now).
	Now func() time.Time
}

var stravaWireDefault = stravaWire{
	AuthURL:       "https://www.strava.com/oauth/authorize",
	TokenURL:      "https://www.strava.com/oauth/token",
	DeauthURL:     "https://www.strava.com/oauth/deauthorize",
	APIBase:       "https://www.strava.com/api/v3",
	ActivityLinks: "https://www.strava.com/activities/",
}

var stravaHosts = []string{"https://www.strava.com"}

const (
	stravaDefaultLimit = 20
	stravaMaxLimit     = 50
)

func init() {
	register(Provider{ID: Strava, Billing: BillingFree, NeedsOAuth: true, NameJa: "Strava", NameEn: "Strava",
		Ops: []OpSpec{
			{Name: "activities", Scopes: []string{stravaScopeActivity}, Args: []string{"limit", "after"}, Check: checkStravaActivities},
			{Name: "stats", Scopes: []string{stravaScopeActivity}},
		},
		Hosts: stravaHosts,
		New: func(sec Secrets) (map[string]Operation, error) {
			c, err := newStrava(sec, stravaWireDefault)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"activities": OperationFunc(c.activities), "stats": OperationFunc(c.stats)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) { return newStrava(sec, stravaWireDefault) },
	})
}

type strava struct {
	sec  Secrets
	wire stravaWire
	hc   *http.Client
	now  func() time.Time
}

func newStrava(sec Secrets, w stravaWire) (*strava, error) {
	if sec.ClientID == "" || sec.ClientSecret == "" {
		return nil, errors.New("EXT_STRAVA_CLIENT_ID / _CLIENT_SECRET not set")
	}
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(stravaHosts, DefaultTimeout)
	}
	now := w.Now
	if now == nil {
		now = time.Now
	}
	return &strava{sec: sec, wire: w, hc: hc, now: now}, nil
}

// --- OAuth ---

// AuthURL: scopes are comma separated and `read` goes first. The PKCE
// challenge is not sent (Strava does not take it).
func (c *strava) AuthURL(state, _, redirectURI string, scopes []string) string {
	if !slices.Contains(scopes, stravaScopeRead) {
		scopes = append([]string{stravaScopeRead}, scopes...)
	}
	v := url.Values{
		"client_id":       {c.sec.ClientID},
		"redirect_uri":    {redirectURI},
		"response_type":   {"code"},
		"approval_prompt": {"auto"},
		"scope":           {strings.Join(scopes, ",")},
		"state":           {state},
	}
	return c.wire.AuthURL + "?" + v.Encode()
}

func (c *strava) Exchange(ctx context.Context, code, _, _ string) (Token, error) {
	return c.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}}, Token{})
}

// Refresh exchanges the refresh token (Strava rotates it).
func (c *strava) Refresh(ctx context.Context, t Token) (Token, error) {
	if t.RefreshToken == "" {
		return Token{}, ErrInvalidGrant
	}
	return c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.RefreshToken}}, t)
}

// Revoke deauthorizes the app at Strava (it takes a live access token, so an
// expired one is renewed first).
func (c *strava) Revoke(ctx context.Context, t Token) error {
	if !t.Expiry.IsZero() && t.expired(c.now()) && t.RefreshToken != "" {
		nt, err := c.Refresh(ctx, t)
		if err != nil {
			return err // ErrInvalidGrant: gone at Strava, or renewed elsewhere
		}
		t = nt
	}
	if t.AccessToken == "" {
		return nil
	}
	form := url.Values{"access_token": {t.AccessToken}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.wire.DeauthURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = ReadBody(resp.Body)
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrInvalidGrant
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500 || resp.StatusCode == 429, Err: errors.New("deauthorize refused")}
}

func (c *strava) token(ctx context.Context, form url.Values, prev Token) (Token, error) {
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
		// Strava answers 400 {"message":"Bad Request","errors":[{"code":"invalid",...}]} to a code or refresh token it does not know
		if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized {
			return Token{}, ErrInvalidGrant
		}
		return Token{}, &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500 || resp.StatusCode == 429, Err: errors.New("token endpoint refused")}
	}
	var r struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresAt    int64  `json:"expires_at"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &r); err != nil || r.AccessToken == "" {
		return Token{}, errors.New("strava: bad token answer")
	}
	// Strava's token answer has no scope: the asked scopes are kept (Callback / prev)
	t := Token{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken, Scopes: prev.Scopes}
	switch {
	case r.ExpiresAt > 0:
		t.Expiry = time.Unix(r.ExpiresAt, 0).UTC()
	case r.ExpiresIn > 0:
		t.Expiry = c.now().Add(time.Duration(r.ExpiresIn) * time.Second).UTC()
	}
	if t.RefreshToken == "" {
		t.RefreshToken = prev.RefreshToken
	}
	return t, nil
}

// --- ops ---

// stravaDate reads after (YYYY-MM-DD, JST): the start of that day, not after
// today.
func stravaDate(args map[string]string, now time.Time) (time.Time, bool, error) {
	s := strings.TrimSpace(args["after"])
	if s == "" {
		return time.Time{}, false, nil
	}
	d, err := time.ParseInLocation(time.DateOnly, s, jst)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("%w: after must be YYYY-MM-DD", ErrBadArgs)
	}
	if d.After(now.In(jst)) {
		return time.Time{}, false, fmt.Errorf("%w: after is after today", ErrBadArgs)
	}
	return d, true, nil
}

func checkStravaActivities(a map[string]string) error {
	if _, err := argInt(a, "limit", stravaDefaultLimit, 1, stravaMaxLimit); err != nil {
		return err
	}
	_, _, err := stravaDate(a, time.Now())
	return err
}

// stravaSportJa names the sport types people use most; others keep
// Strava's name.
var stravaSportJa = map[string]string{
	"AlpineSki":        "スキー",
	"EBikeRide":        "E バイク",
	"GravelRide":       "グラベルライド",
	"Hike":             "ハイク",
	"MountainBikeRide": "MTB",
	"Ride":             "ライド",
	"Run":              "ラン",
	"Snowboard":        "スノーボード",
	"Swim":             "スイム",
	"TrailRun":         "トレイルラン",
	"VirtualRide":      "バーチャルライド",
	"VirtualRun":       "バーチャルラン",
	"Walk":             "ウォーク",
	"WeightTraining":   "筋トレ",
	"Workout":          "ワークアウト",
	"Yoga":             "ヨガ",
}

func stravaSport(sportType, typ string) string {
	name := sportType
	if name == "" {
		name = typ
	}
	if ja, ok := stravaSportJa[name]; ok {
		return ja
	}
	if r := []rune(name); len(r) > 30 {
		name = string(r[:30])
	}
	return name
}

// stravaKm writes metres as "10.52 km".
func stravaKm(m float64) string {
	if !finite(m) || m < 0 {
		return ""
	}
	return strconv.FormatFloat(math.Round(m/10)/100+0, 'f', -1, 64) + " km"
}

// stravaDuration writes seconds as "52分" / "1時間05分".
func stravaDuration(sec int64) string {
	if sec < 0 {
		return ""
	}
	h, m := sec/3600, (sec%3600)/60
	if h > 0 {
		return fmt.Sprintf("%d時間%02d分", h, m)
	}
	return strconv.FormatInt(m, 10) + "分"
}

type stravaActivity struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Distance   float64 `json:"distance"`
	MovingTime int64   `json:"moving_time"`
	Type       string  `json:"type"`
	SportType  string  `json:"sport_type"`
	StartDate  string  `json:"start_date"`
}

// row: title = name, subtitle = sport・start (JST), value = distance / moving
// time, link = the activity's page.
func (a stravaActivity) row(prefix string) Row {
	r := Row{Title: a.Name, Value: joinSlash(stravaKm(a.Distance), stravaDuration(a.MovingTime))}
	when := ""
	if t, err := time.Parse(time.RFC3339, a.StartDate); err == nil {
		when = t.In(jst).Format("1/2 15:04")
	}
	r.Subtitle = joinDots(stravaSport(a.SportType, a.Type), when)
	if a.ID > 0 {
		r.URL = prefix + strconv.FormatInt(a.ID, 10)
	}
	return r
}

// joinSlash joins the parts that are not empty with " / ".
func joinSlash(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " / ")
}

// activities: GET /athlete/activities, newest first.
func (c *strava) activities(ctx context.Context, in Input) (Output, error) {
	limit, err := argInt(in.Args, "limit", stravaDefaultLimit, 1, stravaMaxLimit)
	if err != nil {
		return Output{}, err
	}
	after, hasAfter, err := stravaDate(in.Args, c.now())
	if err != nil {
		return Output{}, err
	}
	// Strava answers oldest first when `after` is sent, so it is not sent:
	// the newest `limit` are read and the ones before after are dropped
	// (the newest of those on or after it, newest first).
	q := url.Values{"per_page": {strconv.Itoa(limit)}, "page": {"1"}}
	var list []stravaActivity
	if err := c.get(ctx, in, "/athlete/activities", q, &list); err != nil {
		return Output{}, err
	}
	out := Output{Rows: []Row{}}
	for _, a := range list {
		if a.Name == "" || len(out.Rows) == limit {
			continue
		}
		if hasAfter {
			if st, err := time.Parse(time.RFC3339, a.StartDate); err != nil || st.Before(after) {
				continue
			}
		}
		out.Rows = append(out.Rows, a.row(c.wire.ActivityLinks))
	}
	return out, nil
}

type stravaTotals struct {
	Count         int     `json:"count"`
	Distance      float64 `json:"distance"`
	MovingTime    int64   `json:"moving_time"`
	ElevationGain float64 `json:"elevation_gain"`
}

func (t stravaTotals) row(title string) Row {
	r := Row{Title: title, Subtitle: strconv.Itoa(t.Count) + " 回", Value: joinSlash(stravaKm(t.Distance), stravaDuration(t.MovingTime))}
	if finite(t.ElevationGain) && t.ElevationGain > 0 {
		r.Subtitle += "・獲得標高 " + strconv.FormatFloat(math.Round(t.ElevationGain), 'f', -1, 64) + " m"
	}
	return r
}

// stats: GET /athlete (the person's id), then GET /athletes/{id}/stats.
// Rows: the last 4 weeks' ride, run and swim totals.
func (c *strava) stats(ctx context.Context, in Input) (Output, error) {
	var me struct {
		ID int64 `json:"id"`
	}
	if err := c.get(ctx, in, "/athlete", nil, &me); err != nil {
		return Output{}, err
	}
	if me.ID <= 0 {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("strava: no athlete id")}
	}
	var s struct {
		Ride stravaTotals `json:"recent_ride_totals"`
		Run  stravaTotals `json:"recent_run_totals"`
		Swim stravaTotals `json:"recent_swim_totals"`
	}
	if err := c.get(ctx, in, "/athletes/"+strconv.FormatInt(me.ID, 10)+"/stats", nil, &s); err != nil {
		return Output{}, err
	}
	return Output{Rows: []Row{s.Ride.row("最近 4 週間のライド"), s.Run.row("最近 4 週間のラン"), s.Swim.row("最近 4 週間のスイム")}}, nil
}

// get calls the API with the person's token. 401 is a token Strava no
// longer takes (ErrInvalidGrant: the connection turns to error) unless its
// errors name a missing scope (…_permission: ErrNotConnected, connect
// again); 403 is ErrNotConnected too; 404 is bad_request; 429 / 5xx are
// retryable provider errors.
func (c *strava) get(ctx context.Context, in Input, path string, q url.Values, out any) error {
	if in.Token == nil || in.Token.AccessToken == "" {
		return ErrNotConnected
	}
	u := c.wire.APIBase + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
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
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if err := json.Unmarshal(b, out); err != nil {
			return &ProviderError{Status: resp.StatusCode, Err: errors.New("strava: bad answer")}
		}
		return nil
	case resp.StatusCode == http.StatusUnauthorized:
		if stravaMissingScope(b) {
			return ErrNotConnected
		}
		return ErrInvalidGrant
	case resp.StatusCode == http.StatusForbidden:
		return ErrNotConnected
	case resp.StatusCode == http.StatusNotFound:
		return &ProviderError{Status: resp.StatusCode, Err: fmt.Errorf("%w: strava has no such item", ErrUnknown)}
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode == 429 || resp.StatusCode >= 500,
		Err: fmt.Errorf("strava status %d", resp.StatusCode)}
}

// stravaMissingScope: a 401 whose errors name a permission the grant lacks
// ({"field":"activity:read_permission","code":"missing"}).
func stravaMissingScope(body []byte) bool {
	var e struct {
		Errors []struct {
			Field string `json:"field"`
			Code  string `json:"code"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &e) != nil {
		return false
	}
	for _, x := range e.Errors {
		if x.Code == "missing" && strings.HasSuffix(x.Field, "_permission") {
			return true
		}
	}
	return false
}
