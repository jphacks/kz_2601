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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	dropboxTestID     = "db-client"
	dropboxTestSecret = "db-secret"
)

// fakeDropbox is Dropbox's OAuth and API v2 on httptest (no live calls).
type fakeDropbox struct {
	url string

	mu        sync.Mutex
	challenge string
	refreshes int
	revoked   []string
	entries   []map[string]any
	lastPath  string
	lastBody  map[string]any
}

func newFakeDropbox(t *testing.T) (*fakeDropbox, *httptest.Server) {
	f := &fakeDropbox{entries: []map[string]any{
		{".tag": "folder", "name": "Photos", "path_display": "/Photos"},
		{".tag": "file", "name": "old.txt", "path_display": "/old.txt", "server_modified": "2026-01-05T00:00:00Z", "size": 12},
		{".tag": "file", "name": "資料.pdf", "path_display": "/資料.pdf", "server_modified": "2026-11-13T01:30:00Z", "size": 1258291},
		{".tag": "file", "name": "notes.md", "path_display": "/notes.md", "server_modified": "2026-11-01T10:00:00Z", "size": 2048},
	}}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("POST /oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != dropboxTestID || r.Form.Get("client_secret") != dropboxTestSecret {
			writeJSON(w, 400, map[string]string{"error": "invalid_client"})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "db-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			writeJSON(w, 200, map[string]any{"access_token": "db-access-0", "refresh_token": "db-refresh", "expires_in": 14400, "token_type": "bearer",
				"scope": "files.metadata.read", "uid": "1", "account_id": "dbid:x"})
		case "refresh_token":
			// not rotated; no refresh_token in the answer
			if r.Form.Get("refresh_token") != "db-refresh" {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			f.refreshes++
			writeJSON(w, 200, map[string]any{"access_token": "db-access-" + strconv.Itoa(f.refreshes), "expires_in": 14400, "token_type": "bearer"})
		default:
			writeJSON(w, 400, map[string]string{"error": "unsupported_grant_type"})
		}
	})
	authed := func(w http.ResponseWriter, r *http.Request) bool {
		switch tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); {
		case tok == "noscope":
			writeJSON(w, 403, map[string]any{"error_summary": "missing_scope/..", "error": map[string]any{".tag": "missing_scope"}})
			return false
		case tok == "busy":
			writeJSON(w, 503, map[string]any{"error_summary": "down"})
			return false
		case tok == "limited":
			w.Header().Set("Retry-After", "30")
			writeJSON(w, 429, map[string]any{"error_summary": "too_many_requests/.."})
			return false
		case !strings.HasPrefix(tok, "db-access-"):
			writeJSON(w, 401, map[string]any{"error_summary": "invalid_access_token/.."})
			return false
		}
		return true
	}
	mux.HandleFunc("POST /2/auth/token/revoke", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		f.revoked = append(f.revoked, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		f.mu.Unlock()
		w.WriteHeader(200)
	})
	readBody := func(w http.ResponseWriter, r *http.Request) bool {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || r.Header.Get("Content-Type") != "application/json" {
			writeJSON(w, 400, map[string]any{"error_summary": "bad body"})
			return false
		}
		f.lastPath, f.lastBody = r.URL.Path, body
		return true
	}
	mux.HandleFunc("POST /2/files/list_folder", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if !readBody(w, r) {
			return
		}
		if f.lastBody["path"] != "" {
			writeJSON(w, 409, map[string]any{"error_summary": "path/not_found/..", "error": map[string]any{".tag": "path"}})
			return
		}
		writeJSON(w, 200, map[string]any{"entries": f.entries, "cursor": "c", "has_more": false})
	})
	mux.HandleFunc("POST /2/files/search_v2", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if !readBody(w, r) {
			return
		}
		q, _ := f.lastBody["query"].(string)
		if q == "bad" {
			writeJSON(w, 409, map[string]any{"error_summary": "invalid_argument/..", "error": map[string]any{".tag": "invalid_argument"}})
			return
		}
		var matches []map[string]any
		for _, e := range f.entries {
			if strings.Contains(strings.ToLower(e["name"].(string)), strings.ToLower(q)) {
				matches = append(matches, map[string]any{"match_type": map[string]any{".tag": "filename"}, "metadata": map[string]any{".tag": "metadata", "metadata": e}})
			}
		}
		writeJSON(w, 200, map[string]any{"matches": matches, "has_more": false})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, srv
}

func (f *fakeDropbox) wire(srv *httptest.Server) dropboxWire {
	return dropboxWire{AuthURL: srv.URL + "/oauth2/authorize", TokenURL: srv.URL + "/oauth2/token", RevokeURL: srv.URL + "/2/auth/token/revoke",
		APIBase: srv.URL + "/2", Client: srv.Client(),
		Now: func() time.Time { return time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC) }}
}

func useDropbox(t *testing.T) (*env, *fakeDropbox) {
	f, srv := newFakeDropbox(t)
	old := dropboxWireDefault
	dropboxWireDefault = f.wire(srv)
	t.Cleanup(func() { dropboxWireDefault = old })
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[Dropbox] = true
		c.Secrets = map[string]Secrets{Dropbox: {ClientID: dropboxTestID, ClientSecret: dropboxTestSecret}}
	}})
	return e, f
}

func (e *env) connectDropbox(f *fakeDropbox) {
	e.t.Helper()
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, user, Dropbox, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/oauth2/authorize?") || q.Get("scope") != "files.metadata.read" || q.Get("code_challenge_method") != "S256" ||
		q.Get("token_access_type") != "offline" || q.Get("client_id") != dropboxTestID || q.Get("redirect_uri") != "https://runner.example/oauth/callback/dropbox" || q.Get("client_secret") != "" {
		e.t.Fatalf("consent url %s", auth)
	}
	f.mu.Lock()
	f.challenge = q.Get("code_challenge")
	f.mu.Unlock()
	if back := e.svc.Callback(ctx, Dropbox, q.Get("state"), "db-code"); back != "tailorit://connected?provider=dropbox&status=ok" {
		e.t.Fatalf("callback → %s", back)
	}
}

// connect (PKCE, metadata only) → recent (files only, newest first) →
// search → refresh (not rotated) → revoke with the access token.
func TestDropboxFlow(t *testing.T) {
	e, f := useDropbox(t)
	ctx := context.Background()
	e.connectDropbox(f)

	res, err := e.call(e.req(Dropbox, "recent", map[string]string{"limit": "2"}))
	if err != nil {
		t.Fatal(err)
	}
	rows := res.Output.Rows
	if len(rows) != 2 || rows[0] != (Row{Title: "資料.pdf", Subtitle: "/資料.pdf · 2026/11/13 10:30", Value: "1.2 MB"}) ||
		rows[1] != (Row{Title: "notes.md", Subtitle: "/notes.md · 2026/11/1 19:00", Value: "2.0 KB"}) {
		t.Fatalf("rows %+v", rows)
	}
	if f.lastPath != "/2/files/list_folder" || f.lastBody["path"] != "" || f.lastBody["recursive"] != false || f.lastBody["limit"] != float64(dropboxListLimit) {
		t.Fatalf("request %s %v", f.lastPath, f.lastBody)
	}
	if e.runs() != 1 || res.Charge.Coins != 0 {
		t.Fatalf("runs = %d (free: one `runs` a call)", e.runs())
	}

	res, err = e.call(e.req(Dropbox, "search", map[string]string{"q": "o"}))
	if err != nil {
		t.Fatal(err)
	}
	rows = res.Output.Rows
	if len(rows) != 3 || rows[0] != (Row{Title: "Photos", Subtitle: "/Photos", Value: "フォルダ"}) || rows[1].Title != "old.txt" || rows[1].Value != "12 B" || rows[2].Title != "notes.md" {
		t.Fatalf("rows %+v", rows)
	}
	if o, _ := f.lastBody["options"].(map[string]any); f.lastPath != "/2/files/search_v2" || f.lastBody["query"] != "o" || o["max_results"] != float64(20) || o["filename_only"] != true || o["file_status"] != "active" {
		t.Fatalf("request %s %v", f.lastPath, f.lastBody)
	}
	for _, r := range rows {
		if r.URL != "" {
			t.Fatalf("a link %+v", r)
		}
	}

	// expired → refreshed; the refresh token is kept
	tok, _ := e.svc.Conns.Token(ctx, user, Dropbox)
	tok.Expiry = e.c.t.Add(-time.Minute)
	if err := e.svc.Conns.Save(ctx, user, Dropbox, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Dropbox, "recent", nil)); err != nil {
		t.Fatal(err)
	}
	tok, err = e.svc.Conns.Token(ctx, user, Dropbox)
	if err != nil || tok.AccessToken != "db-access-1" || tok.RefreshToken != "db-refresh" || f.refreshes != 1 {
		t.Fatalf("refreshed: %v refreshes=%d", err, f.refreshes)
	}

	// cut: the grant is revoked at Dropbox with the access token
	if err := e.svc.Revoke(ctx, user, Dropbox); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Dropbox); c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("after revoke %+v", c)
	}
	if len(f.revoked) != 1 || f.revoked[0] != "db-access-1" {
		t.Fatalf("revoked %v", f.revoked)
	}
}

// A 401 on a call turns the connection to error.
func TestDropboxRevokedAtProvider(t *testing.T) {
	e, f := useDropbox(t)
	e.connectDropbox(f)
	ctx := context.Background()
	if err := e.svc.Conns.Save(ctx, user, Dropbox, Token{AccessToken: "dead"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Dropbox, "recent", nil)); !errors.Is(err, ErrNotConnected) || e.runs() != 0 {
		t.Fatalf("401: %v runs=%d", err, e.runs())
	}
	if c, _, _ := e.mem.Get(ctx, user, Dropbox); c.Status != StatusError {
		t.Fatalf("status %q", c.Status)
	}
}

func dropboxTestClient(t *testing.T) (*dropbox, *fakeDropbox) {
	f, srv := newFakeDropbox(t)
	c, err := newDropbox(Secrets{ClientID: dropboxTestID, ClientSecret: dropboxTestSecret}, f.wire(srv))
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

func TestDropboxOps(t *testing.T) {
	c, f := dropboxTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: "db-access-0"}
	// recent: every file when limit is large; folders left out; a file
	// without a modified time sorts last
	f.entries = append(f.entries, map[string]any{".tag": "file", "name": "x", "path_display": "/x", "size": 0})
	out, err := c.recent(ctx, Input{Token: tok, Args: map[string]string{"limit": "50"}})
	if err != nil || len(out.Rows) != 4 || out.Rows[0].Title != "資料.pdf" || out.Rows[2].Title != "old.txt" || out.Rows[3] != (Row{Title: "x", Subtitle: "/x", Value: "0 B"}) {
		t.Fatalf("recent: %v %+v", err, out.Rows)
	}
	// no files in the root: 0 rows
	f.entries = f.entries[:1]
	if out, err := c.recent(ctx, Input{Token: tok}); err != nil || len(out.Rows) != 0 {
		t.Fatalf("empty: %v %+v", err, out.Rows)
	}
	// search: Dropbox's 409 (bad argument) is a bad request, not a retry
	_, err = c.search(ctx, Input{Token: tok, Args: map[string]string{"q": "bad"}})
	var pe *ProviderError
	if Reason(err) != ReasonBadRequest || !errors.As(err, &pe) || pe.Retryable {
		t.Fatalf("409: %v", err)
	}
	if out, err := c.search(ctx, Input{Token: tok, Args: map[string]string{"q": " zzz ", "limit": "3"}}); err != nil || len(out.Rows) != 0 || f.lastBody["query"] != "zzz" {
		t.Fatalf("no match: %v %+v %v", err, out.Rows, f.lastBody)
	}
	// bad arguments are refused before anything is sent
	f.lastPath = ""
	for _, a := range []map[string]string{{}, {"q": "a\nb"}, {"q": "x", "limit": "0"}} {
		if _, err := c.search(ctx, Input{Token: tok, Args: a}); !errors.Is(err, ErrBadArgs) || f.lastPath != "" {
			t.Fatalf("%v: %v %s", a, err, f.lastPath)
		}
	}
}

func TestDropboxSize(t *testing.T) {
	for n, want := range map[int64]string{0: "0 B", 1023: "1023 B", 1024: "1.0 KB", 1258291: "1.2 MB", 5 << 30: "5.0 GB", 3 << 40: "3.0 TB", 2000 << 40: "2000.0 TB"} {
		if got := dropboxSize(n); got != want {
			t.Errorf("%d: %s, want %s", n, got, want)
		}
	}
}

func TestDropboxErrors(t *testing.T) {
	c, f := dropboxTestClient(t)
	ctx := context.Background()
	if _, err := c.recent(ctx, Input{Token: &Token{AccessToken: "noscope"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("403: %v", err)
	}
	if _, err := c.recent(ctx, Input{Token: &Token{AccessToken: "dead"}}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("401: %v", err)
	}
	for _, tok := range []string{"busy", "limited"} {
		_, err := c.recent(ctx, Input{Token: &Token{AccessToken: tok}})
		var pe *ProviderError
		if !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream {
			t.Fatalf("%s: %v", tok, err)
		}
	}
	if _, err := c.recent(ctx, Input{}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no token: %v", err)
	}
	// OAuth edges
	if _, err := c.Refresh(ctx, Token{AccessToken: "x"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("refresh without a refresh token: %v", err)
	}
	if _, err := c.Refresh(ctx, Token{RefreshToken: "gone"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("refused refresh: %v", err)
	}
	if _, err := c.Exchange(ctx, "bad", "v", "https://runner.example/oauth/callback/dropbox"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("bad code: %v", err)
	}
	// a token already gone (401) is done; no access token is ErrNoRevoke
	if err := c.Revoke(ctx, Token{AccessToken: "dead"}); err != nil {
		t.Fatalf("gone: %v", err)
	}
	if err := c.Revoke(ctx, Token{RefreshToken: "x"}); !errors.Is(err, ErrNoRevoke) || len(f.revoked) != 0 {
		t.Fatalf("no access token: %v %v", err, f.revoked)
	}
	if err := c.Revoke(ctx, Token{AccessToken: "busy"}); Reason(err) != ReasonUpstream {
		t.Fatalf("503: %v", err)
	}
}

// Check refuses bad arguments before anything is counted.
func TestDropboxCheck(t *testing.T) {
	p, _ := Lookup(Dropbox)
	bad := map[string][]map[string]string{
		"recent": {{"limit": "0"}, {"limit": "51"}, {"limit": "x"}, {"q": "x"}},
		"search": {{}, {"q": ""}, {"q": "  "}, {"q": "a\nb"}, {"q": "a\x00b"}, {"q": strings.Repeat("あ", dropboxQueryMax+1)}, {"q": "\xff"}, {"q": "https://x.example/"}, {"q": "x", "limit": "51"}, {"q": "x", "path": "/"}},
	}
	for op, cases := range bad {
		for i, a := range cases {
			if _, err := p.CheckArgs(op, a); !errors.Is(err, ErrUnknown) {
				t.Errorf("%s case %d: %v", op, i, err)
			}
		}
	}
	for op, a := range map[string]map[string]string{
		"recent": {},
		"search": {"q": strings.Repeat("あ", dropboxQueryMax), "limit": "50"},
	} {
		if _, err := p.CheckArgs(op, a); err != nil {
			t.Errorf("%s: %v", op, err)
		}
	}
}

// The real endpoints are on the allowlist; nothing else is reachable;
// without the app key / secret the provider stays off; off by default and
// never in the review build; nothing writes; only the metadata scope.
func TestDropboxDefinition(t *testing.T) {
	p, ok := Lookup(Dropbox)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "DROPBOX" || len(p.Hosts) != 2 {
		t.Fatalf("%+v", p)
	}
	w := dropboxWireDefault
	for _, raw := range []string{w.AuthURL, w.TokenURL, w.RevokeURL, w.APIBase + "/files/list_folder"} {
		u, _ := url.Parse(raw)
		if !AllowedURL(p.Hosts, u) {
			t.Errorf("%s not on the allowlist", raw)
		}
	}
	for _, o := range p.Ops {
		if o.Write || len(o.Scopes) != 1 || o.Scopes[0] != dropboxScopeMetadata {
			t.Errorf("%s write=%v scopes=%v", o.Name, o.Write, o.Scopes)
		}
	}
	c, err := newDropbox(Secrets{ClientID: "a", ClientSecret: "b"}, w)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.hc.Get("https://content.dropboxapi.com/2/files/download"); !errors.Is(err, ErrEgress) {
		t.Fatalf("egress (content host): %v", err)
	}
	if _, err := p.New(Secrets{ClientSecret: "b"}); err == nil {
		t.Fatal("built without a client id")
	}
	cfg, _ := ConfigFromEnv(func(string) string { return "" })
	if cfg.Enabled[Dropbox] {
		t.Fatal("on by default")
	}
	for _, f := range ReviewBuildFeatures() {
		if f == p.FeatureKey() {
			t.Fatal("in the review build")
		}
	}
}
