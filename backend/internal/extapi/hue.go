package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Philips Hue remote API (#2142): the person's lights through Hue's cloud.
// Per-user OAuth 2.0 (Hue's v2 endpoints). The client id and secret go to
// the token endpoint as HTTP Basic, never in a URL. Hue has no scopes, does
// not take PKCE, and has no revoke endpoint (Revoke is ErrNoRevoke: the
// tokens are deleted; the person removes the app in the Hue account).
//
// Env: EXT_HUE_CLIENT_ID, EXT_HUE_CLIENT_SECRET (Secret Manager).
const Hue = "hue"

// hueWire is where the provider talks to; tests point it at httptest.
type hueWire struct {
	AuthURL  string // opened in the browser, not reached by the runner
	TokenURL string
	// Client is nil in production: NewHTTPClient on hueHosts.
	Client *http.Client
	// Now is nil in production (time.Now).
	Now func() time.Time
}

var hueWireDefault = hueWire{
	AuthURL:  "https://api.meethue.com/v2/oauth2/authorize",
	TokenURL: "https://api.meethue.com/v2/oauth2/token",
}

// hueHosts: the only host the runner reaches (token and remote API).
var hueHosts = []string{"https://api.meethue.com"}

type hue struct {
	sec  Secrets
	wire hueWire
	hc   *http.Client
	now  func() time.Time
}

func newHue(sec Secrets, w hueWire) (*hue, error) {
	if sec.ClientID == "" || sec.ClientSecret == "" {
		return nil, errors.New("EXT_HUE_CLIENT_ID / _CLIENT_SECRET not set")
	}
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(hueHosts, DefaultTimeout)
	}
	now := w.Now
	if now == nil {
		now = time.Now
	}
	return &hue{sec: sec, wire: w, hc: hc, now: now}, nil
}

// --- OAuth ---

// AuthURL: Hue sends the person back to the callback registered with the
// app, so redirect_uri, scopes and the PKCE challenge are not sent.
func (c *hue) AuthURL(state, _, _ string, _ []string) string {
	v := url.Values{
		"client_id":     {c.sec.ClientID},
		"response_type": {"code"},
		"state":         {state},
	}
	return c.wire.AuthURL + "?" + v.Encode()
}

func (c *hue) Exchange(ctx context.Context, code, _, _ string) (Token, error) {
	return c.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}}, Token{})
}

// Refresh exchanges the refresh token (Hue rotates it).
func (c *hue) Refresh(ctx context.Context, t Token) (Token, error) {
	if t.RefreshToken == "" {
		return Token{}, ErrInvalidGrant
	}
	return c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.RefreshToken}}, t)
}

// Revoke: see the file comment.
func (c *hue) Revoke(context.Context, Token) error { return ErrNoRevoke }

// token posts to the token endpoint. 400 / 401 are a code or refresh token
// Hue does not take (the client is fixed); 429 / 5xx are retryable.
func (c *hue) token(ctx context.Context, form url.Values, prev Token) (Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.wire.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, errors.New("hue: building the request failed")
	}
	req.SetBasicAuth(c.sec.ClientID, c.sec.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return Token{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := ReadBody(resp.Body)
	if err != nil {
		return Token{}, err
	}
	switch {
	case resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized:
		return Token{}, ErrInvalidGrant
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return Token{}, &ProviderError{Status: resp.StatusCode, Retryable: true, Err: errors.New("hue token endpoint unavailable")}
	case resp.StatusCode != http.StatusOK:
		return Token{}, &ProviderError{Status: resp.StatusCode, Err: errors.New("hue token endpoint refused")}
	}
	var r struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(b, &r); err != nil || r.AccessToken == "" {
		return Token{}, errors.New("hue: bad token answer")
	}
	t := Token{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken, Scopes: prev.Scopes}
	if r.ExpiresIn > 0 {
		t.Expiry = c.now().Add(time.Duration(r.ExpiresIn) * time.Second).UTC()
	}
	if t.RefreshToken == "" {
		t.RefreshToken = prev.RefreshToken
	}
	return t, nil
}
