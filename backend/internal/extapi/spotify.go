package extapi

import (
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
	"unicode/utf8"
)

// Spotify (Web API): what the person played, what is playing now, and the
// catalog search. Free (one `runs` a call), read only, per-user OAuth 2.0
// with PKCE.
//
//	recentlyPlayed(limit?)      Rows: tracks, newest first   user-read-recently-played
//	nowPlaying()                Rows: the playing track or none   user-read-currently-playing
//	search(q, type?, limit?)    Rows: tracks / artists / albums / playlists   (no scope; the person's token)
//
// limit is 1-50 (recentlyPlayed default 20, search default 10). type is
// track (default), artist, album or playlist. The search needs a token too
// (Spotify has no keyless search), so it uses the person's.
//
// Spotify's token endpoint takes PKCE; a client secret, when set, goes in
// Basic auth as well (EXT_SPOTIFY_CLIENT_SECRET is optional). The refresh
// token may rotate. Spotify has no revoke endpoint (ErrNoRevoke): the
// stored token is deleted and the person removes the app in Spotify's
// account settings.
//
// Terms (docs/backend/features/external-api-terms.md): a developer-mode app
// serves at most 5 registered people and the extended quota is for
// companies only; the content is shown with its link to open.spotify.com.
// Whether to ship it is the owner's decision (default off).
//
// Env: EXT_SPOTIFY_ENABLED=1, EXT_SPOTIFY_CLIENT_ID, EXT_SPOTIFY_CLIENT_SECRET
// (optional; Secret Manager). Off by default; not in the review build (NeedsOAuth).
const Spotify = "spotify"

const (
	spotifyScopeRecent = "user-read-recently-played"
	spotifyScopeNow    = "user-read-currently-playing"
)

// spotifyWire is where the provider talks to; tests point it at httptest.
type spotifyWire struct {
	AuthURL  string // opened in the browser, not reached by the runner
	TokenURL string
	APIBase  string
	// Client is nil in production: NewHTTPClient on the provider's Hosts.
	Client *http.Client
	// Now is nil in production (time.Now).
	Now func() time.Time
}

var spotifyWireDefault = spotifyWire{
	AuthURL:  "https://accounts.spotify.com/authorize",
	TokenURL: "https://accounts.spotify.com/api/token",
	APIBase:  "https://api.spotify.com/v1",
}

var spotifyHosts = []string{"https://accounts.spotify.com", "https://api.spotify.com"}

const (
	spotifyMaxLimit           = 50
	spotifyRecentDefaultLimit = 20
	spotifySearchDefaultLimit = 10
	spotifyMaxQuery           = 200
	// spotifyLinkPrefix is the only link a row carries.
	spotifyLinkPrefix = "https://open.spotify.com/"
)

var spotifySearchTypes = []string{"track", "artist", "album", "playlist"}

func init() {
	register(Provider{ID: Spotify, Billing: BillingFree, NeedsOAuth: true, NameJa: "Spotify", NameEn: "Spotify",
		Ops: []OpSpec{
			{Name: "recentlyPlayed", Scopes: []string{spotifyScopeRecent}, Args: []string{"limit"}, Check: checkSpotifyRecent},
			{Name: "nowPlaying", Scopes: []string{spotifyScopeNow}},
			{Name: "search", Args: []string{"q", "type", "limit"}, Check: checkSpotifySearch},
		},
		Hosts: spotifyHosts,
		New: func(sec Secrets) (map[string]Operation, error) {
			c, err := newSpotify(sec, spotifyWireDefault)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"recentlyPlayed": OperationFunc(c.recentlyPlayed), "nowPlaying": OperationFunc(c.nowPlaying),
				"search": OperationFunc(c.search)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) { return newSpotify(sec, spotifyWireDefault) },
	})
}

type spotify struct {
	sec  Secrets
	wire spotifyWire
	hc   *http.Client
	now  func() time.Time
}

func newSpotify(sec Secrets, w spotifyWire) (*spotify, error) {
	if sec.ClientID == "" {
		return nil, errors.New("EXT_SPOTIFY_CLIENT_ID not set")
	}
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(spotifyHosts, DefaultTimeout)
	}
	now := w.Now
	if now == nil {
		now = time.Now
	}
	return &spotify{sec: sec, wire: w, hc: hc, now: now}, nil
}

// --- OAuth ---

// AuthURL: scopes are space separated; PKCE S256.
func (c *spotify) AuthURL(state, codeChallenge, redirectURI string, scopes []string) string {
	v := url.Values{
		"client_id":             {c.sec.ClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {strings.Join(scopes, " ")},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}
	return c.wire.AuthURL + "?" + v.Encode()
}

func (c *spotify) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (Token, error) {
	return c.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {codeVerifier}, "redirect_uri": {redirectURI}}, Token{})
}

// Refresh renews the access token (the refresh token may rotate; when the
// answer has none the old one stays).
func (c *spotify) Refresh(ctx context.Context, t Token) (Token, error) {
	if t.RefreshToken == "" {
		return Token{}, ErrInvalidGrant
	}
	return c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.RefreshToken}}, t)
}

// Revoke: Spotify has no revoke endpoint (see the file comment).
func (c *spotify) Revoke(context.Context, Token) error { return ErrNoRevoke }

func (c *spotify) token(ctx context.Context, form url.Values, prev Token) (Token, error) {
	form.Set("client_id", c.sec.ClientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.wire.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if c.sec.ClientSecret != "" {
		req.SetBasicAuth(c.sec.ClientID, c.sec.ClientSecret)
	}
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
		return Token{}, errors.New("spotify: bad token answer")
	}
	t := Token{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken, Scopes: strings.Fields(r.Scope)}
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

// spotifyHas: the granted scopes cover scope (unknown grants pass; Spotify
// then answers 403 and the person connects again).
func spotifyHas(t *Token, scope string) bool {
	return t == nil || len(t.Scopes) == 0 || slices.Contains(t.Scopes, scope)
}

// --- ops ---

func checkSpotifyRecent(a map[string]string) error {
	_, err := argInt(a, "limit", spotifyRecentDefaultLimit, 1, spotifyMaxLimit)
	return err
}

// spotifyQuery checks search's q: 1-200 characters, one line.
func spotifyQuery(v string) (string, error) {
	v = strings.TrimSpace(v)
	switch {
	case v == "":
		return "", fmt.Errorf("%w: q is required", ErrBadArgs)
	case !utf8.ValidString(v) || utf8.RuneCountInString(v) > spotifyMaxQuery:
		return "", fmt.Errorf("%w: q is over %d characters", ErrBadArgs, spotifyMaxQuery)
	case strings.ContainsFunc(v, func(r rune) bool { return r < 0x20 }):
		return "", fmt.Errorf("%w: q has control characters", ErrBadArgs)
	}
	return v, nil
}

func spotifyType(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "track", nil
	}
	if !slices.Contains(spotifySearchTypes, v) {
		return "", fmt.Errorf("%w: type must be one of %s", ErrBadArgs, strings.Join(spotifySearchTypes, ", "))
	}
	return v, nil
}

func checkSpotifySearch(a map[string]string) error {
	if _, err := spotifyQuery(a["q"]); err != nil {
		return err
	}
	if _, err := spotifyType(a["type"]); err != nil {
		return err
	}
	_, err := argInt(a, "limit", spotifySearchDefaultLimit, 1, spotifyMaxLimit)
	return err
}

type spotifyNamed struct {
	Name string `json:"name"`
}

// spotifyItem is a track, artist, album or playlist (the fields differ).
type spotifyItem struct {
	Name        string         `json:"name"`
	Artists     []spotifyNamed `json:"artists"`
	Album       *spotifyNamed  `json:"album"`
	DurationMs  int64          `json:"duration_ms"`
	ReleaseDate string         `json:"release_date"`
	Owner       *struct {
		DisplayName string `json:"display_name"`
	} `json:"owner"`
	Followers *struct {
		Total int64 `json:"total"`
	} `json:"followers"`
	Tracks *struct {
		Total int64 `json:"total"`
	} `json:"tracks"`
	ExternalURLs struct {
		Spotify string `json:"spotify"`
	} `json:"external_urls"`
}

// link is the item's open.spotify.com page, or "".
func (it spotifyItem) link() string {
	if strings.HasPrefix(it.ExternalURLs.Spotify, spotifyLinkPrefix) {
		return it.ExternalURLs.Spotify
	}
	return ""
}

func (it spotifyItem) artistNames() string {
	names := make([]string, 0, len(it.Artists))
	for _, a := range it.Artists {
		if a.Name != "" {
			names = append(names, a.Name)
		}
	}
	return strings.Join(names, ", ")
}

// trackRow: title = track, subtitle = artists・album, link = the track's page.
func (it spotifyItem) trackRow() Row {
	album := ""
	if it.Album != nil {
		album = it.Album.Name
	}
	return Row{Title: it.Name, Subtitle: joinDots(it.artistNames(), album), URL: it.link()}
}

// spotifyDuration writes "3:45".
func spotifyDuration(ms int64) string {
	if ms <= 0 {
		return ""
	}
	s := ms / 1000
	return strconv.FormatInt(s/60, 10) + ":" + fmt.Sprintf("%02d", s%60)
}

// recentlyPlayed: GET /me/player/recently-played. Rows: newest first,
// value = when (JST).
func (c *spotify) recentlyPlayed(ctx context.Context, in Input) (Output, error) {
	if !spotifyHas(in.Token, spotifyScopeRecent) {
		return Output{}, ErrNotConnected
	}
	limit, err := argInt(in.Args, "limit", spotifyRecentDefaultLimit, 1, spotifyMaxLimit)
	if err != nil {
		return Output{}, err
	}
	var r struct {
		Items []struct {
			Track    *spotifyItem `json:"track"`
			PlayedAt string       `json:"played_at"`
		} `json:"items"`
	}
	if _, err := c.get(ctx, in, "/me/player/recently-played", url.Values{"limit": {strconv.Itoa(limit)}}, &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: []Row{}}
	for _, it := range r.Items {
		if it.Track == nil || it.Track.Name == "" || len(out.Rows) == limit {
			continue
		}
		row := it.Track.trackRow()
		if at, err := time.Parse(time.RFC3339Nano, it.PlayedAt); err == nil {
			row.Value = at.In(jst).Format("2006/1/2 15:04")
		}
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

// nowPlaying: GET /me/player/currently-playing. Nothing playing (204, or
// no track item) is no rows; otherwise one row, value = 再生中 / 一時停止.
func (c *spotify) nowPlaying(ctx context.Context, in Input) (Output, error) {
	if !spotifyHas(in.Token, spotifyScopeNow) {
		return Output{}, ErrNotConnected
	}
	var r struct {
		IsPlaying bool         `json:"is_playing"`
		Item      *spotifyItem `json:"item"`
	}
	status, err := c.get(ctx, in, "/me/player/currently-playing", nil, &r)
	if err != nil {
		return Output{}, err
	}
	out := Output{Rows: []Row{}}
	if status == http.StatusNoContent || r.Item == nil || r.Item.Name == "" {
		return out, nil
	}
	row := r.Item.trackRow()
	row.Value = "一時停止"
	if r.IsPlaying {
		row.Value = "再生中"
	}
	out.Rows = append(out.Rows, row)
	return out, nil
}

// search: GET /search with the person's market. One row per hit of the
// asked type.
func (c *spotify) search(ctx context.Context, in Input) (Output, error) {
	q, err := spotifyQuery(in.Args["q"])
	if err != nil {
		return Output{}, err
	}
	typ, err := spotifyType(in.Args["type"])
	if err != nil {
		return Output{}, err
	}
	limit, err := argInt(in.Args, "limit", spotifySearchDefaultLimit, 1, spotifyMaxLimit)
	if err != nil {
		return Output{}, err
	}
	var r map[string]struct {
		Items []*spotifyItem `json:"items"`
	}
	v := url.Values{"q": {q}, "type": {typ}, "limit": {strconv.Itoa(limit)}, "market": {"from_token"}}
	if _, err := c.get(ctx, in, "/search", v, &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: []Row{}}
	for _, it := range r[typ+"s"].Items {
		if it == nil || it.Name == "" || len(out.Rows) == limit {
			continue // Spotify pads some lists with null
		}
		out.Rows = append(out.Rows, spotifySearchRow(typ, *it))
	}
	return out, nil
}

func spotifySearchRow(typ string, it spotifyItem) Row {
	switch typ {
	case "artist":
		row := Row{Title: it.Name, Subtitle: "アーティスト", URL: it.link()}
		if it.Followers != nil && it.Followers.Total > 0 {
			row.Value = strconv.FormatInt(it.Followers.Total, 10) + " フォロワー"
		}
		return row
	case "album":
		return Row{Title: it.Name, Subtitle: it.artistNames(), Value: it.ReleaseDate, URL: it.link()}
	case "playlist":
		row := Row{Title: it.Name, URL: it.link()}
		if it.Owner != nil {
			row.Subtitle = it.Owner.DisplayName
		}
		if it.Tracks != nil && it.Tracks.Total > 0 {
			row.Value = strconv.FormatInt(it.Tracks.Total, 10) + " 曲"
		}
		return row
	}
	row := it.trackRow()
	row.Value = spotifyDuration(it.DurationMs)
	return row
}

// get calls the API with the person's token and returns the status. 401 is
// a token Spotify no longer takes (ErrInvalidGrant: the connection turns to
// error); 403 is a missing scope or a person the developer-mode app does
// not list (ErrNotConnected: connect again); 429 / 5xx are retryable
// provider errors. 204 reads no body.
func (c *spotify) get(ctx context.Context, in Input, path string, q url.Values, out any) (int, error) {
	if in.Token == nil || in.Token.AccessToken == "" {
		return 0, ErrNotConnected
	}
	u := c.wire.APIBase + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+in.Token.AccessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := ReadBody(resp.Body)
	if err != nil {
		return 0, err
	}
	switch {
	case resp.StatusCode == http.StatusNoContent:
		return resp.StatusCode, nil
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if err := json.Unmarshal(b, out); err != nil {
			return resp.StatusCode, &ProviderError{Status: resp.StatusCode, Err: errors.New("spotify: bad answer")}
		}
		return resp.StatusCode, nil
	case resp.StatusCode == http.StatusUnauthorized:
		return resp.StatusCode, ErrInvalidGrant
	case resp.StatusCode == http.StatusForbidden:
		return resp.StatusCode, ErrNotConnected
	}
	return resp.StatusCode, &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode == 429 || resp.StatusCode >= 500,
		Err: fmt.Errorf("spotify status %d", resp.StatusCode)}
}
