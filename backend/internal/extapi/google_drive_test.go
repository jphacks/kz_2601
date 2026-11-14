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
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeDrive is Google's token / revoke endpoints and the Drive API's
// files.list on httptest (no live calls).
type fakeDrive struct {
	url string

	mu        sync.Mutex
	challenge string
	refreshOK bool
	revoked   []string
	lastQuery url.Values
}

func newFakeDrive(t *testing.T) (*fakeDrive, gcalWire) {
	f := &fakeDrive{refreshOK: true}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	gerr := func(w http.ResponseWriter, status int, reason string) {
		writeJSON(w, status, map[string]any{"error": map[string]any{"code": status, "errors": []map[string]string{{"reason": reason}}}})
	}
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != gcalTestID || r.Form.Get("client_secret") != gcalTestSecret {
			writeJSON(w, 401, map[string]string{"error": "invalid_client"})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "good-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge || !strings.HasSuffix(r.Form.Get("redirect_uri"), "/oauth/callback/googleDrive") {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			writeJSON(w, 200, map[string]any{"access_token": "access-1", "refresh_token": "refresh-1", "expires_in": 3600, "scope": gdriveScope})
		case "refresh_token":
			if !f.refreshOK || r.Form.Get("refresh_token") != "refresh-1" {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			writeJSON(w, 200, map[string]any{"access_token": "access-2", "expires_in": 3600})
		default:
			writeJSON(w, 400, map[string]string{"error": "unsupported_grant_type"})
		}
	})
	mux.HandleFunc("POST /revoke", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		f.revoked = append(f.revoked, r.Form.Get("token"))
		f.mu.Unlock()
		w.WriteHeader(200)
	})
	mux.HandleFunc("GET /drive/v3/files", func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") {
		case "access-1", "access-2", "wide":
		case "narrow":
			writeJSON(w, 403, map[string]any{"error": map[string]any{"code": 403, "status": "PERMISSION_DENIED",
				"details": []map[string]string{{"reason": "ACCESS_TOKEN_SCOPE_INSUFFICIENT"}}}})
			return
		case "quota":
			gerr(w, 403, "userRateLimitExceeded")
			return
		case "busy":
			gerr(w, 503, "backendError")
			return
		case "notfound":
			gerr(w, 404, "notFound")
			return
		case "badquery":
			gerr(w, 400, "invalid")
			return
		default:
			writeJSON(w, 401, map[string]any{"error": map[string]any{"code": 401, "status": "UNAUTHENTICATED"}})
			return
		}
		f.mu.Lock()
		f.lastQuery = r.URL.Query()
		f.mu.Unlock()
		writeJSON(w, 200, map[string]any{"files": []map[string]any{
			{"id": "f1", "name": "企画書", "mimeType": "application/vnd.google-apps.document", "modifiedTime": "2026-11-12T01:00:00.000Z",
				"webViewLink": "https://docs.google.com/document/d/f1/edit"},
			{"id": "f2", "name": "写真.jpg", "mimeType": "image/jpeg", "modifiedTime": "2026-11-11T23:30:00Z", "size": "1258291",
				"webViewLink": "https://drive.google.com/file/d/f2/view"},
			{"id": "f3", "mimeType": "application/zip", "size": "348160", "webViewLink": "javascript:alert(1)"},
		}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, gcalWire{AuthURL: srv.URL + "/auth", TokenURL: srv.URL + "/token", RevokeURL: srv.URL + "/revoke",
		APIBase: srv.URL + "/drive/v3", Client: srv.Client(),
		Now: func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, gcalJST) }}
}

func gdriveTestClient(t *testing.T) (*googleDrive, *fakeDrive) {
	f, w := newFakeDrive(t)
	d, err := newGoogleDrive(Secrets{ClientID: gcalTestID, ClientSecret: gcalTestSecret}, w)
	if err != nil {
		t.Fatal(err)
	}
	return d, f
}

// connect (PKCE, with Calendar's OAuth client) → recent → search → refresh
// → a refused refresh needs a reconnect → revoke.
func TestGoogleDriveFlow(t *testing.T) {
	f, w := newFakeDrive(t)
	old := googleDriveWire
	googleDriveWire = w
	t.Cleanup(func() { googleDriveWire = old })
	c, err := ConfigFromEnv(func(k string) string {
		return map[string]string{"EXT_GOOGLE_CALENDAR_CLIENT_ID": gcalTestID, "EXT_GOOGLE_CALENDAR_CLIENT_SECRET": gcalTestSecret}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, opts{cfg: func(cfg *Config) {
		cfg.Enabled[GoogleDrive] = true
		cfg.Secrets = map[string]Secrets{GoogleDrive: c.Secrets[GoogleDrive]}
	}})
	ctx := context.Background()
	if _, err := e.call(e.req(GoogleDrive, "recent", nil)); !errors.Is(err, ErrNotConnected) || e.runs() != 0 {
		t.Fatalf("before connecting: %v", err)
	}

	auth, err := e.svc.Start(ctx, user, GoogleDrive, nil)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/auth?") || q.Get("scope") != gdriveScope || q.Get("client_id") != gcalTestID ||
		q.Get("access_type") != "offline" || q.Get("code_challenge_method") != "S256" || q.Get("client_secret") != "" ||
		q.Get("redirect_uri") != "https://runner.example/oauth/callback/googleDrive" {
		t.Fatalf("consent url %s", auth)
	}
	f.mu.Lock()
	f.challenge = q.Get("code_challenge")
	f.mu.Unlock()
	if back := e.svc.Callback(ctx, GoogleDrive, q.Get("state"), "good-code"); back != "tailorit://connected?provider=googleDrive&status=ok" {
		t.Fatalf("callback → %s", back)
	}
	if conn, _, _ := e.mem.Get(ctx, user, GoogleDrive); conn.Status != StatusActive || len(conn.Scopes) != 1 || conn.Scopes[0] != gdriveScope {
		t.Fatalf("connection %+v", conn)
	}

	res, err := e.call(e.req(GoogleDrive, "recent", map[string]string{"limit": "5"}))
	if err != nil {
		t.Fatal(err)
	}
	rows := res.Output.Rows
	if len(rows) != 3 || rows[0].Title != "企画書" || rows[0].Subtitle != "2026-11-12 10:00・ドキュメント" || rows[0].Value != "" ||
		rows[0].URL != "https://docs.google.com/document/d/f1/edit" ||
		rows[1].Title != "写真.jpg" || rows[1].Subtitle != "2026-11-12 08:30・画像" || rows[1].Value != "1.2 MB" || rows[1].URL != "https://drive.google.com/file/d/f2/view" ||
		rows[2].Title != gdriveNoName || rows[2].Subtitle != "zip" || rows[2].Value != "340 KB" || rows[2].URL != "" || res.Charge.Coins != 0 {
		t.Fatalf("rows %+v", rows)
	}
	if lq := f.lastQuery; lq.Get("q") != "trashed = false" || lq.Get("orderBy") != "modifiedTime desc" || lq.Get("pageSize") != "5" ||
		lq.Get("spaces") != "drive" || lq.Get("fields") != gdriveFields {
		t.Fatalf("query %v", lq)
	}
	if e.runs() != 1 {
		t.Fatalf("runs = %d (free: one `runs` a call)", e.runs())
	}

	// search: the quotes and backslashes of q are escaped
	if _, err := e.call(e.req(GoogleDrive, "search", map[string]string{"q": `it's a \ test`})); err != nil {
		t.Fatal(err)
	}
	if lq := f.lastQuery; lq.Get("q") != `(name contains 'it\'s a \\ test' or fullText contains 'it\'s a \\ test') and trashed = false` || lq.Get("pageSize") != "20" {
		t.Fatalf("search query %v", lq)
	}

	// expired → refreshed (Google keeps the refresh token)
	if err := e.svc.Conns.Save(ctx, user, GoogleDrive, Token{AccessToken: "access-1", RefreshToken: "refresh-1",
		Expiry: e.c.t.Add(-time.Minute), Scopes: []string{gdriveScope}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(GoogleDrive, "recent", nil)); err != nil {
		t.Fatal(err)
	}
	if tok, err := e.svc.Conns.Token(ctx, user, GoogleDrive); err != nil || tok.AccessToken != "access-2" || tok.RefreshToken != "refresh-1" {
		t.Fatalf("refreshed %v", err)
	}

	// refresh refused → error: the person reconnects
	f.mu.Lock()
	f.refreshOK = false
	f.mu.Unlock()
	if err := e.svc.Conns.Save(ctx, user, GoogleDrive, Token{AccessToken: "access-2", RefreshToken: "refresh-1", Expiry: e.c.t.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(GoogleDrive, "recent", nil)); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("invalid_grant: %v", err)
	}
	if conn, _, _ := e.mem.Get(ctx, user, GoogleDrive); conn.Status != StatusError {
		t.Fatalf("status %q, want error", conn.Status)
	}

	// cut: the refresh token is revoked at Google (no other Google provider is connected)
	if err := e.svc.Conns.Save(ctx, user, GoogleDrive, Token{AccessToken: "access-1", RefreshToken: "refresh-1"}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Revoke(ctx, user, GoogleDrive); err != nil {
		t.Fatal(err)
	}
	if conn, _, _ := e.mem.Get(ctx, user, GoogleDrive); conn.Status != StatusRevoked || len(conn.Sealed) != 0 {
		t.Fatalf("after revoke %+v", conn)
	}
	if len(f.revoked) != 1 || f.revoked[0] != "refresh-1" {
		t.Fatalf("revoked %v", f.revoked)
	}
}

func TestGoogleDriveArgs(t *testing.T) {
	d, f := gdriveTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: "wide"}
	p, _ := Lookup(GoogleDrive)
	badRecent := []map[string]string{{"limit": "0"}, {"limit": "51"}, {"limit": "x"}}
	for i, a := range badRecent {
		if _, err := d.recent(ctx, Input{Token: tok, Args: a}); !errors.Is(err, ErrBadArgs) {
			t.Errorf("recent case %d: %v", i, err)
		}
		if _, err := p.CheckArgs("recent", a); !errors.Is(err, ErrUnknown) {
			t.Errorf("recent check %d: %v", i, err)
		}
	}
	badSearch := []map[string]string{
		{},                              // no q
		{"q": "   "},                    // blank
		{"q": strings.Repeat("あ", 257)}, // too long
		{"q": "a\x00b"},                 // control char
		{"q": "a\nb"},                   // two lines
		{"q": "企画", "limit": "100"},     // bad limit
		{"q": "企画", "limit": "-1"},      //
	}
	for i, a := range badSearch {
		if _, err := d.search(ctx, Input{Token: tok, Args: a}); !errors.Is(err, ErrBadArgs) {
			t.Errorf("search case %d: %v", i, err)
		}
		if _, err := p.CheckArgs("search", a); !errors.Is(err, ErrUnknown) {
			t.Errorf("search check %d: %v", i, err)
		}
	}
	if f.lastQuery != nil {
		t.Fatal("a bad argument reached Drive")
	}
	// no args: 20 rows
	if _, err := d.recent(ctx, Input{Token: tok}); err != nil {
		t.Fatal(err)
	}
	if f.lastQuery.Get("pageSize") != "20" {
		t.Fatalf("default query %v", f.lastQuery)
	}
	if _, err := d.search(ctx, Input{Token: tok, Args: map[string]string{"q": "  企画書 ", "limit": "3"}}); err != nil {
		t.Fatal(err)
	}
	if lq := f.lastQuery; lq.Get("q") != "(name contains '企画書' or fullText contains '企画書') and trashed = false" || lq.Get("pageSize") != "3" {
		t.Fatalf("search query %v", lq)
	}
	for in, want := range map[string]string{"": "", "x": "", "12": "12 B", "2048": "2 KB", "1258291": "1.2 MB", "2147483648": "2.0 GB"} {
		if got := gdriveSize(in); got != want {
			t.Errorf("gdriveSize(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"application/vnd.google-apps.spreadsheet": "スプレッドシート", "video/mp4": "動画", "audio/mpeg": "音声",
		"text/plain": "テキスト", "application/pdf": "PDF", "application/x-tar": "x-tar", "": ""} {
		if got := gdriveKind(in); got != want {
			t.Errorf("gdriveKind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGoogleDriveErrors(t *testing.T) {
	d, _ := gdriveTestClient(t)
	ctx := context.Background()
	if _, err := d.recent(ctx, Input{Token: &Token{AccessToken: "narrow"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("403 scope: %v", err)
	}
	// a token granted without the scope never reaches Drive
	if _, err := d.recent(ctx, Input{Token: &Token{AccessToken: "wide", Scopes: []string{gcalScopeRead}}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no drive scope: %v", err)
	}
	var pe *ProviderError
	_, err := d.recent(ctx, Input{Token: &Token{AccessToken: "busy"}})
	if !errors.As(err, &pe) || !pe.Retryable || pe.Billable || Reason(err) != ReasonUpstream {
		t.Fatalf("503: %v", err)
	}
	_, err = d.recent(ctx, Input{Token: &Token{AccessToken: "quota"}})
	if !errors.As(err, &pe) || !pe.Retryable || pe.Status != http.StatusTooManyRequests || !upstreamLimited(err) {
		t.Fatalf("quota 403: %v", err)
	}
	for _, tok := range []string{"notfound", "badquery"} {
		_, err = d.search(ctx, Input{Token: &Token{AccessToken: tok}, Args: map[string]string{"q": "x"}})
		if !errors.Is(err, ErrUnknown) || errors.Is(err, ErrBadArgs) || Reason(err) != ReasonBadRequest {
			t.Fatalf("%s: %v", tok, err)
		}
	}
	if _, err := d.recent(ctx, Input{Token: &Token{AccessToken: "dead"}}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("401: %v", err)
	}
	if _, err := d.recent(ctx, Input{}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no token: %v", err)
	}
}

// Off by default, only Google's token and Drive hosts, drive.file only
// (never a restricted scope), Calendar's OAuth client unless its own is
// set, never in the review build.
func TestGoogleDriveDefinition(t *testing.T) {
	p, ok := Lookup(GoogleDrive)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "GOOGLE_DRIVE" || p.ClientFrom != GoogleCalendar {
		t.Fatalf("%+v", p)
	}
	w := googleDriveWire
	for _, raw := range []string{w.TokenURL, w.RevokeURL, w.APIBase + "/files"} {
		u, _ := url.Parse(raw)
		if !AllowedURL(p.Hosts, u) {
			t.Errorf("%s not on the allowlist", raw)
		}
	}
	restricted := []string{"https://www.googleapis.com/auth/drive", "https://www.googleapis.com/auth/drive.readonly",
		"https://www.googleapis.com/auth/drive.metadata.readonly", "https://www.googleapis.com/auth/drive.metadata"}
	for _, o := range p.Ops {
		if o.Write || len(o.Scopes) != 1 || o.Scopes[0] != gdriveScope || o.Check == nil || o.CacheTTL != 0 {
			t.Errorf("%s: %+v", o.Name, o)
		}
		for _, s := range o.Scopes {
			for _, r := range restricted {
				if s == r {
					t.Errorf("%s asks for the restricted scope %s", o.Name, s)
				}
			}
		}
	}
	d, err := newGoogleDrive(Secrets{ClientID: "a", ClientSecret: "b"}, w)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.g.hc.Get("https://evil.example.com/"); !errors.Is(err, ErrEgress) {
		t.Fatalf("egress: %v", err)
	}
	if _, err := p.New(Secrets{ClientID: "a"}); err == nil || !strings.Contains(err.Error(), "EXT_GOOGLE_DRIVE_CLIENT_ID") {
		t.Fatalf("built without a secret: %v", err)
	}

	env := map[string]string{"EXT_GOOGLE_CALENDAR_CLIENT_ID": "cal-id", "EXT_GOOGLE_CALENDAR_CLIENT_SECRET": "cal-secret"}
	c, _ := ConfigFromEnv(func(k string) string { return env[k] })
	if c.Enabled[GoogleDrive] || c.Secrets[GoogleDrive].ClientID != "cal-id" || c.Secrets[GoogleDrive].ClientSecret != "cal-secret" {
		t.Fatal("not Calendar's client, or on by default")
	}
	env["EXT_GOOGLE_DRIVE_CLIENT_ID"], env["EXT_GOOGLE_DRIVE_CLIENT_SECRET"] = "drive-id", "drive-secret"
	c, _ = ConfigFromEnv(func(k string) string { return env[k] })
	if c.Secrets[GoogleDrive].ClientID != "drive-id" || c.Secrets[GoogleCalendar].ClientID != "cal-id" {
		t.Fatal("its own client must win")
	}
	for _, f := range ReviewBuildFeatures() {
		if f == p.FeatureKey() {
			t.Fatal("in the review build")
		}
	}
}
