package extapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	spotifyTestID     = "sp-client"
	spotifyTestSecret = "sp-secret"
)

// fakeSpotify is Spotify's OAuth and Web API on httptest (no live calls).
type fakeSpotify struct {
	url string

	mu         sync.Mutex
	challenge  string
	refreshes  int
	rotateNext int
	playing    string // "" = 204, else the body of currently-playing
	lastPath   string
	lastQuery  url.Values
}

func newFakeSpotify(t *testing.T) (*fakeSpotify, *httptest.Server) {
	f := &fakeSpotify{}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("POST /api/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		id, secret, ok := r.BasicAuth()
		if !ok || id != spotifyTestID || secret != spotifyTestSecret || r.Form.Get("client_id") != spotifyTestID {
			writeJSON(w, 400, map[string]string{"error": "invalid_client"})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "sp-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			writeJSON(w, 200, map[string]any{"access_token": "sp-access-0", "refresh_token": "sp-refresh-0", "expires_in": 3600,
				"scope": "user-read-recently-played user-read-currently-playing", "token_type": "Bearer"})
		case "refresh_token":
			if r.Form.Get("refresh_token") != "sp-refresh-"+strconv.Itoa(f.rotateNext) {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			f.rotateNext++
			f.refreshes++
			n := strconv.Itoa(f.rotateNext)
			// Spotify may leave the refresh token out: the old one stays
			writeJSON(w, 200, map[string]any{"access_token": "sp-access-" + n, "expires_in": 3600, "token_type": "Bearer"})
		default:
			writeJSON(w, 400, map[string]string{"error": "unsupported_grant_type"})
		}
	})
	authed := func(w http.ResponseWriter, r *http.Request) bool {
		switch tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); {
		case tok == "noscope":
			writeJSON(w, 403, map[string]any{"error": map[string]any{"status": 403, "message": "Insufficient client scope"}})
			return false
		case tok == "busy":
			w.Header().Set("Retry-After", "5")
			writeJSON(w, 429, map[string]any{"error": map[string]any{"status": 429, "message": "Too many requests"}})
			return false
		case tok == "down":
			writeJSON(w, 503, map[string]any{"error": map[string]any{"status": 503}})
			return false
		case !strings.HasPrefix(tok, "sp-access-"):
			writeJSON(w, 401, map[string]any{"error": map[string]any{"status": 401, "message": "The access token expired"}})
			return false
		}
		f.mu.Lock()
		f.lastPath, f.lastQuery = r.URL.Path, r.URL.Query()
		f.mu.Unlock()
		return true
	}
	track := func(name, artist, album, id string, ms int) map[string]any {
		return map[string]any{"name": name, "duration_ms": ms, "artists": []map[string]any{{"name": artist}}, "album": map[string]any{"name": album},
			"external_urls": map[string]any{"spotify": "https://open.spotify.com/track/" + id}}
	}
	mux.HandleFunc("GET /v1/me/player/recently-played", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		writeJSON(w, 200, map[string]any{"items": []map[string]any{
			{"track": track("夜に駆ける", "YOASOBI", "THE BOOK", "3dPB0AfneStP7FgCfnvFBI", 261000), "played_at": "2026-11-11T02:34:56.789Z"},
			{"track": map[string]any{"name": "外のリンク", "external_urls": map[string]any{"spotify": "https://example.com/x"}}, "played_at": "bad"},
			{"track": nil, "played_at": "2026-11-11T01:00:00Z"},
		}})
	})
	mux.HandleFunc("GET /v1/me/player/currently-playing", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		body := f.playing
		f.mu.Unlock()
		if body == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("GET /v1/search", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		writeJSON(w, 200, map[string]any{
			"tracks":  map[string]any{"items": []any{track("アイドル", "YOASOBI", "THE BOOK 3", "6D4ao1bBPbbHdGIp5ySHVt", 213000), nil}},
			"artists": map[string]any{"items": []any{map[string]any{"name": "YOASOBI", "followers": map[string]any{"total": 1234567}, "external_urls": map[string]any{"spotify": "https://open.spotify.com/artist/64tJ2EAv1R6UaZqc4iOCyj"}}}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, srv
}

func (f *fakeSpotify) wire(srv *httptest.Server) spotifyWire {
	return spotifyWire{AuthURL: srv.URL + "/authorize", TokenURL: srv.URL + "/api/token", APIBase: srv.URL + "/v1", Client: srv.Client(),
		Now: func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC) }}
}

func useSpotify(t *testing.T) (*env, *fakeSpotify) {
	f, srv := newFakeSpotify(t)
	old := spotifyWireDefault
	spotifyWireDefault = f.wire(srv)
	t.Cleanup(func() { spotifyWireDefault = old })
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[Spotify] = true
		c.Secrets = map[string]Secrets{Spotify: {ClientID: spotifyTestID, ClientSecret: spotifyTestSecret}}
	}})
	return e, f
}

// connectSpotify connects user; want is the scope param (space separated).
func (e *env) connectSpotify(f *fakeSpotify, scopes []string, want string) {
	e.t.Helper()
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, user, Spotify, scopes)
	if err != nil {
		e.t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/authorize?") || q.Get("scope") != want || q.Get("code_challenge_method") != "S256" || q.Get("response_type") != "code" ||
		q.Get("client_id") != spotifyTestID || q.Get("redirect_uri") != "https://runner.example/oauth/callback/spotify" || q.Get("client_secret") != "" {
		e.t.Fatalf("consent url %s", auth)
	}
	f.mu.Lock()
	f.challenge = q.Get("code_challenge")
	f.rotateNext = 0
	f.mu.Unlock()
	if back := e.svc.Callback(ctx, Spotify, q.Get("state"), "sp-code"); back != "tailorit://connected?provider=spotify&status=ok" {
		e.t.Fatalf("callback → %s", back)
	}
}

func TestSpotifyRegistered(t *testing.T) {
	p, ok := Lookup(Spotify)
	if !ok || p.Billing != BillingFree || !p.NeedsOAuth || p.Fake || p.NameJa == "" ||
		!slices.Equal(p.Hosts, []string{"https://accounts.spotify.com", "https://api.spotify.com"}) {
		t.Fatalf("spotify %+v", p)
	}
	if slices.Contains(ReviewBuildFeatures(), "externalCall.spotify") {
		t.Fatal("an OAuth provider is not in the review build")
	}
	for _, op := range p.Ops {
		if op.Write || op.CacheTTL != 0 {
			t.Fatalf("%s: read ops, never cached", op.Name)
		}
	}
	cfg, err := ConfigFromEnv(func(k string) string {
		return map[string]string{"EXT_SPOTIFY_ENABLED": "1", "EXT_SPOTIFY_CLIENT_ID": "id"}[k]
	})
	if err != nil || !cfg.Enabled[Spotify] || cfg.Secrets[Spotify].ClientID != "id" {
		t.Fatalf("env: %v %v", cfg.Enabled, err)
	}
	// the client id is required; the secret is not (PKCE)
	if _, err := newSpotify(Secrets{}, spotifyWireDefault); err == nil {
		t.Fatal("no client id")
	}
	if _, err := newSpotify(Secrets{ClientID: "id"}, spotifyWireDefault); err != nil {
		t.Fatal(err)
	}
}

// connect (PKCE, both read scopes) → recentlyPlayed → nowPlaying (204 /
// playing) → search → refresh (refresh token kept) → refused refresh → revoke
// (no endpoint: the stored token is deleted).
func TestSpotifyFlow(t *testing.T) {
	e, f := useSpotify(t)
	ctx := context.Background()
	e.connectSpotify(f, nil, "user-read-recently-played user-read-currently-playing")

	res, err := e.call(e.req(Spotify, "recentlyPlayed", map[string]string{"limit": "10"}))
	if err != nil {
		t.Fatal(err)
	}
	want := []Row{
		{Title: "夜に駆ける", Subtitle: "YOASOBI・THE BOOK", Value: "2026/11/11 11:34", URL: "https://open.spotify.com/track/3dPB0AfneStP7FgCfnvFBI"},
		{Title: "外のリンク"},
	}
	if !slices.Equal(res.Output.Rows, want) || f.lastPath != "/v1/me/player/recently-played" || f.lastQuery.Get("limit") != "10" {
		t.Fatalf("rows %+v (%s %v)", res.Output.Rows, f.lastPath, f.lastQuery)
	}
	if e.runs() != 1 || res.Charge.Coins != 0 {
		t.Fatalf("runs = %d (free: one `runs` a call)", e.runs())
	}

	// nothing playing: 204 → no rows, still one `runs`
	res, err = e.call(e.req(Spotify, "nowPlaying", nil))
	if err != nil || len(res.Output.Rows) != 0 || res.Output.Rows == nil || e.runs() != 2 {
		t.Fatalf("204: %+v %v runs=%d", res.Output, err, e.runs())
	}
	f.playing = `{"is_playing":true,"item":{"name":"群青","artists":[{"name":"YOASOBI"}],"album":{"name":"THE BOOK"},"external_urls":{"spotify":"https://open.spotify.com/track/4TPwlDm6S3qvEiDlEHEx7z"}}}`
	res, err = e.call(e.req(Spotify, "nowPlaying", nil))
	if err != nil || !slices.Equal(res.Output.Rows, []Row{{Title: "群青", Subtitle: "YOASOBI・THE BOOK", Value: "再生中", URL: "https://open.spotify.com/track/4TPwlDm6S3qvEiDlEHEx7z"}}) {
		t.Fatalf("playing: %+v %v", res.Output.Rows, err)
	}
	f.playing = `{"is_playing":false,"item":{"name":"群青"}}`
	if res, err = e.call(e.req(Spotify, "nowPlaying", nil)); err != nil || res.Output.Rows[0].Value != "一時停止" {
		t.Fatalf("paused: %+v %v", res.Output.Rows, err)
	}
	// a podcast episode (no track item) is nothing to show
	f.playing = `{"is_playing":true,"currently_playing_type":"episode","item":null}`
	if res, err = e.call(e.req(Spotify, "nowPlaying", nil)); err != nil || len(res.Output.Rows) != 0 {
		t.Fatalf("episode: %+v %v", res.Output.Rows, err)
	}

	// search: the asked type's list, null entries skipped
	res, err = e.call(e.req(Spotify, "search", map[string]string{"q": "YOASOBI"}))
	if err != nil || !slices.Equal(res.Output.Rows, []Row{{Title: "アイドル", Subtitle: "YOASOBI・THE BOOK 3", Value: "3:33", URL: "https://open.spotify.com/track/6D4ao1bBPbbHdGIp5ySHVt"}}) {
		t.Fatalf("search tracks: %+v %v", res.Output.Rows, err)
	}
	if f.lastQuery.Get("q") != "YOASOBI" || f.lastQuery.Get("type") != "track" || f.lastQuery.Get("limit") != "10" || f.lastQuery.Get("market") != "from_token" {
		t.Fatalf("search query %v", f.lastQuery)
	}
	res, err = e.call(e.req(Spotify, "search", map[string]string{"q": "YOASOBI", "type": "artist", "limit": "5"}))
	if err != nil || !slices.Equal(res.Output.Rows, []Row{{Title: "YOASOBI", Subtitle: "アーティスト", Value: "1234567 フォロワー", URL: "https://open.spotify.com/artist/64tJ2EAv1R6UaZqc4iOCyj"}}) {
		t.Fatalf("search artists: %+v %v", res.Output.Rows, err)
	}
	if res, err = e.call(e.req(Spotify, "search", map[string]string{"q": "x", "type": "album"})); err != nil || len(res.Output.Rows) != 0 {
		t.Fatalf("search albums (none in the answer): %+v %v", res.Output.Rows, err)
	}

	// expired → refreshed; the answer has no refresh token, so the old one stays
	tok, _ := e.svc.Conns.Token(ctx, user, Spotify)
	tok.Expiry = e.c.t.Add(-time.Minute)
	if err := e.svc.Conns.Save(ctx, user, Spotify, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Spotify, "nowPlaying", nil)); err != nil {
		t.Fatal(err)
	}
	tok, err = e.svc.Conns.Token(ctx, user, Spotify)
	if err != nil || tok.AccessToken != "sp-access-1" || tok.RefreshToken != "sp-refresh-0" || f.refreshes != 1 ||
		!slices.Equal(tok.Scopes, []string{"user-read-recently-played", "user-read-currently-playing"}) {
		t.Fatalf("refreshed: %v %+v refreshes=%d", err, tok, f.refreshes)
	}

	// a refresh token Spotify refuses is invalid_grant → error
	if err := e.svc.Conns.Save(ctx, user, Spotify, Token{AccessToken: "sp-access-0", RefreshToken: "gone", Expiry: e.c.t.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Spotify, "nowPlaying", nil)); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("invalid_grant: %v", err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Spotify); c.Status != StatusError {
		t.Fatalf("status %q, want error", c.Status)
	}

	// reconnect and cut: no revoke endpoint, the stored token goes
	e.connectSpotify(f, nil, "user-read-recently-played user-read-currently-playing")
	if err := e.svc.Revoke(ctx, user, Spotify); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Spotify); c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("after revoke %+v", c)
	}
}

// 401 turns the connection to error; 403 (scope / not in the developer-mode
// list) is 409 connect again; 429 / 503 are retryable upstream errors. None
// of them keeps the run.
func TestSpotifyRefused(t *testing.T) {
	e, f := useSpotify(t)
	e.connectSpotify(f, nil, "user-read-recently-played user-read-currently-playing")
	ctx := context.Background()
	for _, c := range []struct {
		token  string
		reason string
		status string
	}{
		{"dead", ReasonNotConnected, StatusError},
		{"noscope", ReasonNotConnected, StatusActive},
		{"busy", ReasonUpstream, StatusActive},
		{"down", ReasonUpstream, StatusActive},
	} {
		if err := e.svc.Conns.Save(ctx, user, Spotify, Token{AccessToken: c.token}); err != nil {
			t.Fatal(err)
		}
		_, err := e.call(e.req(Spotify, "recentlyPlayed", nil))
		if Reason(err) != c.reason || e.runs() != 0 {
			t.Fatalf("%s: %v (%s) runs=%d", c.token, err, Reason(err), e.runs())
		}
		if conn, _, _ := e.mem.Get(ctx, user, Spotify); conn.Status != c.status {
			t.Fatalf("%s: status %q, want %q", c.token, conn.Status, c.status)
		}
		if c.token == "busy" {
			var pe *ProviderError
			if !errors.As(err, &pe) || !pe.Retryable || pe.Status != 429 {
				t.Fatalf("429: %v", err)
			}
		}
	}
	// a token granted without the scope is refused before any request
	if err := e.svc.Conns.Save(ctx, user, Spotify, Token{AccessToken: "sp-access-0", Scopes: []string{spotifyScopeNow}}); err != nil {
		t.Fatal(err)
	}
	f.lastPath = ""
	if _, err := e.call(e.req(Spotify, "recentlyPlayed", nil)); !errors.Is(err, ErrNotConnected) || f.lastPath != "" {
		t.Fatalf("missing scope: %v path=%q", err, f.lastPath)
	}
}

func TestSpotifyCheck(t *testing.T) {
	p, _ := Lookup(Spotify)
	for _, c := range []struct {
		op   string
		args map[string]string
		ok   bool
	}{
		{"recentlyPlayed", nil, true},
		{"recentlyPlayed", map[string]string{"limit": "50"}, true},
		{"recentlyPlayed", map[string]string{"limit": "0"}, false},
		{"recentlyPlayed", map[string]string{"limit": "51"}, false},
		{"recentlyPlayed", map[string]string{"q": "x"}, false},
		{"nowPlaying", nil, true},
		{"nowPlaying", map[string]string{"limit": "1"}, false},
		{"search", map[string]string{"q": "夜に駆ける"}, true},
		{"search", map[string]string{"q": "x", "type": "playlist", "limit": "1"}, true},
		{"search", nil, false},
		{"search", map[string]string{"q": "  "}, false},
		{"search", map[string]string{"q": "x", "type": "show"}, false},
		{"search", map[string]string{"q": "a\nb"}, false},
		{"search", map[string]string{"q": strings.Repeat("あ", 201)}, false},
		{"search", map[string]string{"q": "https://example.com/x"}, false},
		{"search", map[string]string{"q": "x", "limit": "51"}, false},
	} {
		_, err := p.CheckArgs(c.op, c.args)
		if (err == nil) != c.ok || (err != nil && !errors.Is(err, ErrUnknown)) {
			t.Errorf("%s %v: %v", c.op, c.args, err)
		}
	}
}

// The real client reaches Spotify's hosts only.
func TestSpotifyEgress(t *testing.T) {
	f, srv := newFakeSpotify(t)
	w := f.wire(srv)
	w.Client = nil
	c, err := newSpotify(Secrets{ClientID: spotifyTestID}, w)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.nowPlaying(context.Background(), Input{Token: &Token{AccessToken: "sp-access-0"}})
	if !errors.Is(err, ErrEgress) || f.lastPath != "" {
		t.Fatalf("local server reached: %v", err)
	}
}

// A transport failure (API and token endpoint) is the shared upstream error:
// retryable, 502, no token or secret in it.
func TestSpotifyTransportError(t *testing.T) {
	f, srv := newFakeSpotify(t)
	c, err := newSpotify(Secrets{ClientID: spotifyTestID, ClientSecret: spotifyTestSecret}, f.wire(srv))
	if err != nil {
		t.Fatal(err)
	}
	srv.Close()
	_, err = c.nowPlaying(context.Background(), Input{Token: &Token{AccessToken: "sp-access-0"}})
	var pe *ProviderError
	if !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream || StatusOf(Reason(err)) != http.StatusBadGateway {
		t.Fatalf("api transport: %v", err)
	}
	if strings.Contains(err.Error(), "sp-access-0") {
		t.Fatalf("token in the error: %v", err)
	}
	_, err = c.Refresh(context.Background(), Token{AccessToken: "sp-access-0", RefreshToken: "sp-refresh-0"})
	if !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream {
		t.Fatalf("token transport: %v", err)
	}
	if strings.Contains(err.Error(), spotifyTestSecret) || strings.Contains(err.Error(), "sp-refresh-0") {
		t.Fatalf("secret in the error: %v", err)
	}
}
