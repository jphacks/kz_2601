package extapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Philips Hue remote API (#2142): the person's lights through Hue's cloud.
//
//	lights()   Rows: name / on・brightness 0-100 (or not reachable) / light id;
//	           Cells: id, name, on, brightness, reachable
//
// Off by default and registered only with EXT_HUE_ENABLED=1. Answers are
// never cached (an OAuth provider never is).
//
// The remote CLIP v2 API needs, besides the bearer token, an application key
// of the person's bridge (hue-application-key). It is made by the remote
// link (PUT /route/api/0/config linkbutton, then POST /route/api) and kept
// in memory per access token only: Token has no place for it. Storing it
// sealed with the connection is a follow-up.
//
// Per-user OAuth 2.0 (Hue's v2 endpoints). The client id and secret go to
// the token endpoint as HTTP Basic, never in a URL. Hue has no scopes, does
// not take PKCE, and has no revoke endpoint (Revoke is ErrNoRevoke: the
// tokens are deleted; the person removes the app in the Hue account).
//
// Env: EXT_HUE_ENABLED=1, EXT_HUE_CLIENT_ID, EXT_HUE_CLIENT_SECRET (Secret
// Manager).
const Hue = "hue"

func init() {
	if flag(os.Getenv("EXT_HUE_ENABLED")) {
		register(hueProvider())
	}
}

// hueProvider is Hue's definition (registered only when enabled).
func hueProvider() Provider {
	return Provider{ID: Hue, Billing: BillingFree, NeedsOAuth: true, NameJa: "Philips Hue", NameEn: "Philips Hue",
		Ops: []OpSpec{
			{Name: "lights"},
		},
		Hosts: hueHosts,
		New: func(sec Secrets) (map[string]Operation, error) {
			c, err := newHue(sec, hueWireDefault)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"lights": OperationFunc(c.lights)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) { return newHue(sec, hueWireDefault) },
	}
}

// hueWire is where the provider talks to; tests point it at httptest.
type hueWire struct {
	AuthURL  string // opened in the browser, not reached by the runner
	TokenURL string
	APIBase  string
	// Client is nil in production: NewHTTPClient on hueHosts.
	Client *http.Client
	// Now is nil in production (time.Now).
	Now func() time.Time
}

var hueWireDefault = hueWire{
	AuthURL:  "https://api.meethue.com/v2/oauth2/authorize",
	TokenURL: "https://api.meethue.com/v2/oauth2/token",
	APIBase:  "https://api.meethue.com",
}

// hueHosts: the only host the runner reaches (token and remote API).
var hueHosts = []string{"https://api.meethue.com"}

type hue struct {
	sec  Secrets
	wire hueWire
	hc   *http.Client
	now  func() time.Time

	// keys are the bridges' application keys by sha256(access token).
	mu   sync.Mutex
	keys map[string]string
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
	return &hue{sec: sec, wire: w, hc: hc, now: now, keys: map[string]string{}}, nil
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

// --- remote API ---

// hueMaxKeys bounds the in-memory application keys (dropped all at once past it).
const hueMaxKeys = 1000

// hueDeviceType is the name the bridge lists the runner's key under.
const hueDeviceType = "tailorit#runner"

func hueTokenKey(t *Token) string {
	sum := sha256.Sum256([]byte(t.AccessToken))
	return hex.EncodeToString(sum[:])
}

// send makes one call to the remote API and reads a 2xx answer into out.
// 401 is a token Hue no longer takes; 404 a light the bridge does not have;
// 429 / 5xx are retryable.
func (c *hue) send(ctx context.Context, tok *Token, appKey, method, path string, body, out any) error {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = b
	}
	req, err := http.NewRequestWithContext(ctx, method, c.wire.APIBase+path, bytes.NewReader(payload))
	if err != nil {
		return errors.New("hue: building the request failed")
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if appKey != "" {
		req.Header.Set("hue-application-key", appKey)
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
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrInvalidGrant
	case resp.StatusCode == http.StatusNotFound:
		return &ProviderError{Status: resp.StatusCode, Err: fmt.Errorf("%w: hue has no such light", ErrUnknown)}
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return &ProviderError{Status: resp.StatusCode, Retryable: true, Err: errors.New("hue unavailable")}
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return &ProviderError{Status: resp.StatusCode, Err: errors.New("hue refused")}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return &ProviderError{Status: resp.StatusCode, Err: errors.New("hue: bad answer")}
	}
	return nil
}

// appKey is the bridge's application key for tok: from memory, or made by
// the remote link.
func (c *hue) appKey(ctx context.Context, tok *Token) (string, error) {
	k := hueTokenKey(tok)
	c.mu.Lock()
	key, ok := c.keys[k]
	c.mu.Unlock()
	if ok {
		return key, nil
	}
	if err := c.send(ctx, tok, "", http.MethodPut, "/route/api/0/config", map[string]bool{"linkbutton": true}, nil); err != nil {
		return "", err
	}
	var r []struct {
		Success *struct {
			Username string `json:"username"`
		} `json:"success"`
	}
	if err := c.send(ctx, tok, "", http.MethodPost, "/route/api", map[string]string{"devicetype": hueDeviceType}, &r); err != nil {
		return "", err
	}
	if len(r) == 0 || r[0].Success == nil || r[0].Success.Username == "" {
		return "", &ProviderError{Status: http.StatusBadGateway, Err: errors.New("hue: the bridge gave no application key")}
	}
	key = r[0].Success.Username
	c.mu.Lock()
	if len(c.keys) >= hueMaxKeys {
		c.keys = map[string]string{}
	}
	c.keys[k] = key
	c.mu.Unlock()
	return key, nil
}

// get reads a CLIP v2 resource list (data[]) into out.
func (c *hue) get(ctx context.Context, tok *Token, resource string, out any) error {
	key, err := c.appKey(ctx, tok)
	if err != nil {
		return err
	}
	var r struct {
		Data json.RawMessage `json:"data"`
	}
	if err := c.send(ctx, tok, key, http.MethodGet, "/route/clip/v2/resource/"+resource, nil, &r); err != nil {
		return err
	}
	if len(r.Data) == 0 || string(r.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(r.Data, out); err != nil {
		return &ProviderError{Status: http.StatusBadGateway, Err: errors.New("hue: bad answer data")}
	}
	return nil
}

type hueLight struct {
	ID       string `json:"id"`
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Owner struct {
		RID string `json:"rid"`
	} `json:"owner"`
	On struct {
		On bool `json:"on"`
	} `json:"on"`
	Dimming *struct {
		Brightness float64 `json:"brightness"`
	} `json:"dimming"`
}

type hueConnectivity struct {
	Owner struct {
		RID string `json:"rid"`
	} `json:"owner"`
	Status string `json:"status"`
}

// brightness is 0-100 as an integer ("" for a light that cannot dim).
func (l hueLight) brightness() string {
	if l.Dimming == nil {
		return ""
	}
	return strconv.Itoa(int(math.Max(0, math.Min(100, math.Round(l.Dimming.Brightness)))))
}

// lights: title = name, subtitle = on / off and brightness (or not
// reachable), value = the light id. Kept in Hue's order.
func (c *hue) lights(ctx context.Context, in Input) (Output, error) {
	if in.Token == nil || in.Token.AccessToken == "" {
		return Output{}, ErrNotConnected
	}
	var ls []hueLight
	if err := c.get(ctx, in.Token, "light", &ls); err != nil {
		return Output{}, err
	}
	out := Output{Rows: []Row{}, Cells: [][]string{}}
	if len(ls) == 0 {
		return out, nil
	}
	var conns []hueConnectivity
	if err := c.get(ctx, in.Token, "zigbee_connectivity", &conns); err != nil {
		return Output{}, err
	}
	unreachable := map[string]bool{}
	for _, z := range conns {
		if z.Status != "connected" {
			unreachable[z.Owner.RID] = true
		}
	}
	for _, l := range ls {
		name := clip(strings.TrimSpace(l.Metadata.Name), 100)
		bri := l.brightness()
		reach := !unreachable[l.Owner.RID]
		var sub string
		switch {
		case !reach:
			sub = "つながっていません"
		case !l.On.On:
			sub = "オフ"
		case bri != "":
			sub = "オン・明るさ " + bri + "%"
		default:
			sub = "オン"
		}
		out.Rows = append(out.Rows, Row{Title: name, Subtitle: sub, Value: l.ID})
		out.Cells = append(out.Cells, []string{l.ID, name, strconv.FormatBool(l.On.On), bri, strconv.FormatBool(reach)})
	}
	return out, nil
}
