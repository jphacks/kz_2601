package extapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const gsheetsTestID = "1AbCdEfGhIjKlMnOpQrStUvWxYz0123456789"

// fakeSheets is Google's token / revoke endpoints and the Sheets API on
// httptest (no live calls).
type fakeSheets struct {
	url string

	mu        sync.Mutex
	challenge string
	revoked   []string
	appended  []map[string]any
	lastRange string // the range path segment, unescaped
	lastQuery url.Values
	rows      [][]any
}

func newFakeSheets(t *testing.T) (*fakeSheets, gcalWire) {
	f := &fakeSheets{rows: [][]any{
		{"商品", "売上", "店", "メモ"},
		{"りんご", "1,200", "金沢", "", "", "", "", "", "", "", "11列目"},
		{},
		{"みかん", "800"},
	}}
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
		f.mu.Lock()
		defer f.mu.Unlock()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if r.Form.Get("client_id") != gcalTestID || r.Form.Get("client_secret") != gcalTestSecret || r.Form.Get("code") != "good-code" ||
			base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge || !strings.HasSuffix(r.Form.Get("redirect_uri"), "/oauth/callback/googleSheets") {
			writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
			return
		}
		writeJSON(w, 200, map[string]any{"access_token": "access-1", "refresh_token": "refresh-1", "expires_in": 3600, "scope": gsheetsScope})
	})
	mux.HandleFunc("POST /revoke", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		f.revoked = append(f.revoked, r.Form.Get("token"))
		f.mu.Unlock()
		w.WriteHeader(200)
	})
	// {rest} is "<range>" or "<range>:append"
	mux.HandleFunc("/v4/spreadsheets/{id}/values/{rest}", func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") {
		case "access-1", "wide":
		case "narrow":
			writeJSON(w, 403, map[string]any{"error": map[string]any{"code": 403, "status": "PERMISSION_DENIED",
				"details": []map[string]string{{"reason": "ACCESS_TOKEN_SCOPE_INSUFFICIENT"}}}})
			return
		case "forbidden":
			writeJSON(w, 403, map[string]any{"error": map[string]any{"code": 403, "status": "PERMISSION_DENIED"}})
			return
		case "quota":
			gerr(w, 403, "rateLimitExceeded")
			return
		case "limited":
			gerr(w, 429, "rateLimitExceeded")
			return
		case "busy":
			gerr(w, 503, "backendError")
			return
		default:
			writeJSON(w, 401, map[string]any{"error": map[string]any{"code": 401, "status": "UNAUTHENTICATED"}})
			return
		}
		rest := r.PathValue("rest")
		if r.PathValue("id") != gsheetsTestID {
			writeJSON(w, 404, map[string]any{"error": map[string]any{"code": 404, "status": "NOT_FOUND"}})
			return
		}
		if strings.HasPrefix(rest, "Nope!") {
			writeJSON(w, 400, map[string]any{"error": map[string]any{"code": 400, "status": "INVALID_ARGUMENT"}})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.lastQuery = r.URL.Query()
		if rng, ok := strings.CutSuffix(rest, ":append"); ok && r.Method == http.MethodPost {
			var b map[string]any
			if err := json.NewDecoder(r.Body).Decode(&b); err != nil || r.Header.Get("Content-Type") != "application/json" {
				gerr(w, 400, "invalid")
				return
			}
			f.lastRange = rng
			f.appended = append(f.appended, b)
			writeJSON(w, 200, map[string]any{"spreadsheetId": gsheetsTestID, "updates": map[string]any{"updatedRange": "Sheet1!A5:C5", "updatedRows": 1}})
			return
		}
		if r.Method != http.MethodGet {
			gerr(w, 405, "method")
			return
		}
		f.lastRange = rest
		writeJSON(w, 200, map[string]any{"range": rest, "majorDimension": "ROWS", "values": f.rows})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, gcalWire{AuthURL: srv.URL + "/auth", TokenURL: srv.URL + "/token", RevokeURL: srv.URL + "/revoke",
		APIBase: srv.URL + "/v4", Client: srv.Client(),
		Now: func() time.Time { return time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC) }}
}

func gsheetsTestClient(t *testing.T) (*googleSheets, *fakeSheets) {
	f, w := newFakeSheets(t)
	s, err := newGoogleSheets(Secrets{ClientID: gcalTestID, ClientSecret: gcalTestSecret}, w)
	if err != nil {
		t.Fatal(err)
	}
	return s, f
}

// connect (PKCE, with Calendar's OAuth client, drive.file) → readRange →
// appendRow → revoke.
func TestGoogleSheetsFlow(t *testing.T) {
	f, w := newFakeSheets(t)
	old := googleSheetsWire
	googleSheetsWire = w
	t.Cleanup(func() { googleSheetsWire = old })
	c, err := ConfigFromEnv(func(k string) string {
		return map[string]string{"EXT_GOOGLE_CALENDAR_CLIENT_ID": gcalTestID, "EXT_GOOGLE_CALENDAR_CLIENT_SECRET": gcalTestSecret}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, opts{cfg: func(cfg *Config) {
		cfg.Enabled[GoogleSheets] = true
		cfg.Secrets = map[string]Secrets{GoogleSheets: c.Secrets[GoogleSheets]}
	}})
	ctx := context.Background()

	auth, err := e.svc.Start(ctx, user, GoogleSheets, nil)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/auth?") || q.Get("scope") != gsheetsScope || q.Get("client_id") != gcalTestID ||
		q.Get("redirect_uri") != "https://runner.example/oauth/callback/googleSheets" {
		t.Fatalf("consent url %s", auth)
	}
	f.mu.Lock()
	f.challenge = q.Get("code_challenge")
	f.mu.Unlock()
	if back := e.svc.Callback(ctx, GoogleSheets, q.Get("state"), "good-code"); back != "tailorit://connected?provider=googleSheets&status=ok" {
		t.Fatalf("callback → %s", back)
	}

	res, err := e.call(e.req(GoogleSheets, "readRange", map[string]string{"spreadsheetId": gsheetsTestID, "range": "Sheet1!A:D"}))
	if err != nil {
		t.Fatal(err)
	}
	r := res.Output.Rows
	if len(r) != 3 || r[0].Title != "商品" || r[0].Value != "売上" || r[0].Subtitle != "店・メモ" ||
		r[1].Title != "りんご" || r[1].Value != "1,200" || r[1].Subtitle != "金沢" || r[2].Title != "みかん" {
		t.Fatalf("rows %+v", r)
	}
	if f.lastRange != "Sheet1!A1:D50" || f.lastQuery.Get("valueRenderOption") != "FORMATTED_VALUE" {
		t.Fatalf("range %s %v", f.lastRange, f.lastQuery)
	}

	res, err = e.call(e.req(GoogleSheets, "appendRow", map[string]string{"spreadsheetId": gsheetsTestID, "range": "Sheet1",
		"values": `["2026-11-12","山田","=1+1"]`}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Output.ResultID != "Sheet1!A5:C5" || res.Output.Item.Title != "2026-11-12" || res.Output.Item.Value != "山田" ||
		res.Output.Item.URL != "https://docs.google.com/spreadsheets/d/"+gsheetsTestID+"/edit" {
		t.Fatalf("appendRow %+v", res.Output)
	}
	if f.lastRange != "'Sheet1'" || f.lastQuery.Get("valueInputOption") != "RAW" || f.lastQuery.Get("insertDataOption") != "INSERT_ROWS" {
		t.Fatalf("append %s %v", f.lastRange, f.lastQuery)
	}
	if got := fmt.Sprint(f.appended[0]["values"]); got != "[[2026-11-12 山田 =1+1]]" {
		t.Fatalf("appended %v", got)
	}
	if e.runs() != 2 {
		t.Fatalf("runs = %d", e.runs())
	}

	if err := e.svc.Revoke(ctx, user, GoogleSheets); err != nil {
		t.Fatal(err)
	}
	if len(f.revoked) != 1 || f.revoked[0] != "refresh-1" {
		t.Fatalf("revoked %v", f.revoked)
	}
}

// Ranges without row bounds get rows 1..limit; bounded ones are cut to limit.
func TestGoogleSheetsRanges(t *testing.T) {
	cases := []struct {
		in    string
		limit int
		want  string
	}{
		{"Sheet1", 50, "'Sheet1'!1:50"},
		{"売上 2026", 10, "'売上 2026'!1:10"},
		{"it's", 5, "'it''s'!1:5"},
		{"Sheet1!A:D", 50, "Sheet1!A1:D50"},
		{"Sheet1!B", 20, "Sheet1!B1:B20"},
		{"'My sheet'!A2:C", 10, "'My sheet'!A2:C11"},
		{"Sheet1!A1:D500", 100, "Sheet1!A1:D100"},
		{"Sheet1!A1:D5", 100, "Sheet1!A1:D5"},
		{"A1:B3", 50, "A1:B3"},
		{"Sheet1!C7", 50, "Sheet1!C7"},
		{"Sheet1!A:D", 0, "Sheet1!A:D"},
		{"Sheet1", 0, "'Sheet1'"},
	}
	for _, c := range cases {
		got, err := gsheetsRange(c.in, c.limit)
		if err != nil || got != c.want {
			t.Errorf("%q/%d → %q %v, want %q", c.in, c.limit, got, err, c.want)
		}
	}
	for _, bad := range []string{"", " ", "!A1", "Sheet1!", "Sheet1!a1", "Sheet1!A0:B2", "Sheet1!A1:B2:C3", "Sheet1!AAAA1", "a\x00b", strings.Repeat("s", 201)} {
		if _, err := gsheetsRange(bad, 50); !errors.Is(err, ErrUnknown) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	s, f := gsheetsTestClient(t)
	out, err := s.readRange(context.Background(), Input{Token: &Token{AccessToken: "wide"},
		Args: map[string]string{"spreadsheetId": gsheetsTestID, "range": "Sheet1!A1:D9", "limit": "2"}})
	if err != nil || len(out.Rows) != 2 || f.lastRange != "Sheet1!A1:D2" {
		t.Fatalf("limit 2: %+v %v %s", out.Rows, err, f.lastRange)
	}
}

func TestGoogleSheetsArgs(t *testing.T) {
	p, _ := Lookup(GoogleSheets)
	id := gsheetsTestID
	bad := map[string][]map[string]string{
		"readRange": {{}, {"spreadsheetId": id}, {"range": "Sheet1"}, {"spreadsheetId": "short", "range": "Sheet1"},
			{"spreadsheetId": id + "/x", "range": "Sheet1"}, {"spreadsheetId": id, "range": "Sheet1", "limit": "0"},
			{"spreadsheetId": id, "range": "Sheet1", "limit": "101"}, {"spreadsheetId": id, "range": "Sheet1!zz"},
			{"spreadsheetId": "https://docs.google.com/spreadsheets/d/" + id, "range": "Sheet1"}, {"spreadsheetId": id, "range": "Sheet1", "values": "[]"}},
		"appendRow": {{"spreadsheetId": id, "range": "Sheet1"}, {"spreadsheetId": id, "range": "Sheet1", "values": "[]"},
			{"spreadsheetId": id, "range": "Sheet1", "values": `"a"`}, {"spreadsheetId": id, "range": "Sheet1", "values": `[1,2]`},
			{"spreadsheetId": id, "range": "Sheet1", "values": `["` + strings.Repeat("a", 1001) + `"]`},
			{"spreadsheetId": id, "range": "Sheet1", "values": `["a\u0000b"]`},
			{"spreadsheetId": id, "range": "Sheet1", "values": `[` + strings.Repeat(`"a",`, 26) + `"a"]`},
			{"spreadsheetId": id, "values": `["a"]`}, {"spreadsheetId": id, "range": "Sheet1", "values": `["a"]`, "limit": "1"}},
	}
	for op, cases := range bad {
		for i, a := range cases {
			if _, err := p.CheckArgs(op, a); !errors.Is(err, ErrUnknown) {
				t.Errorf("%s case %d: %v", op, i, err)
			}
		}
	}
	good := map[string]map[string]string{
		"readRange": {"spreadsheetId": id, "range": "'売上'!A1:D50", "limit": "100"},
		"appendRow": {"spreadsheetId": id, "range": "Sheet1!A:C", "values": `["2026-11-12","メモ\n2行目",""]`},
	}
	for op, a := range good {
		if _, err := p.CheckArgs(op, a); err != nil {
			t.Errorf("%s: %v", op, err)
		}
	}
	// the op checks again: nothing reaches Google
	s, f := gsheetsTestClient(t)
	tok := &Token{AccessToken: "wide"}
	if _, err := s.appendRow(context.Background(), Input{Token: tok, Args: map[string]string{"spreadsheetId": id, "range": "Sheet1", "values": "x"}}); !errors.Is(err, ErrUnknown) {
		t.Fatal(err)
	}
	if _, err := s.readRange(context.Background(), Input{Token: tok, Args: map[string]string{"spreadsheetId": "x", "range": "Sheet1"}}); !errors.Is(err, ErrUnknown) {
		t.Fatal(err)
	}
	if len(f.appended) != 0 || f.lastQuery != nil {
		t.Fatal("a bad argument reached Google")
	}
}

// 401 → the connection turns to error; 403 → connect again; 404 (a file
// not picked) and 400 → 400 bad_request; 429 / 5xx / rate-limit 403 → 502
// upstream_unavailable.
func TestGoogleSheetsErrors(t *testing.T) {
	s, f := gsheetsTestClient(t)
	ctx := context.Background()
	read := func(tok string, args map[string]string) error {
		if args == nil {
			args = map[string]string{"spreadsheetId": gsheetsTestID, "range": "Sheet1"}
		}
		_, err := s.readRange(ctx, Input{Token: &Token{AccessToken: tok}, Args: args})
		return err
	}
	if err := read("dead", nil); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("401: %v", err)
	}
	for _, tok := range []string{"narrow", "forbidden"} {
		if err := read(tok, nil); !errors.Is(err, ErrNotConnected) || Reason(err) != ReasonNotConnected {
			t.Fatalf("403 %s: %v", tok, err)
		}
	}
	for _, tok := range []string{"limited", "busy", "quota"} {
		err := read(tok, nil)
		var pe *ProviderError
		if !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream {
			t.Fatalf("%s: %v", tok, err)
		}
	}
	other := strings.Repeat("Z", 30)
	err := read("wide", map[string]string{"spreadsheetId": other, "range": "Sheet1"})
	if Reason(err) != ReasonBadRequest || !strings.Contains(err.Error(), "Google Picker") {
		t.Fatalf("404 not picked: %v", err)
	}
	if err := read("wide", map[string]string{"spreadsheetId": gsheetsTestID, "range": "Nope!A1:B2"}); Reason(err) != ReasonBadRequest {
		t.Fatalf("400 range: %v", err)
	}
	_, err = s.appendRow(ctx, Input{Token: &Token{AccessToken: "wide"}, Args: map[string]string{"spreadsheetId": other, "range": "Sheet1", "values": `["a"]`}})
	if Reason(err) != ReasonBadRequest {
		t.Fatalf("404 append: %v", err)
	}
	if _, err := s.readRange(ctx, Input{Args: map[string]string{"spreadsheetId": gsheetsTestID, "range": "Sheet1"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no token: %v", err)
	}
	// a Calendar-only grant: 409 before anything is sent
	f.lastQuery = nil
	_, err = s.appendRow(ctx, Input{Token: &Token{AccessToken: "wide", Scopes: []string{gcalScopeRead}},
		Args: map[string]string{"spreadsheetId": gsheetsTestID, "range": "Sheet1", "values": `["a"]`}})
	if !errors.Is(err, ErrNotConnected) || f.lastQuery != nil {
		t.Fatalf("calendar-only token: %v", err)
	}
	if !gsheetsCan(&Token{Scopes: []string{"https://www.googleapis.com/auth/spreadsheets"}}) {
		t.Fatal("a wider grant must pass")
	}
}

// Off by default, only Google's token and Sheets hosts, drive.file only,
// appendRow a write, Calendar's OAuth client unless its own is set, never in
// the review build, never cached.
func TestGoogleSheetsDefinition(t *testing.T) {
	p, ok := Lookup(GoogleSheets)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "GOOGLE_SHEETS" || p.ClientFrom != GoogleCalendar {
		t.Fatalf("%+v", p)
	}
	w := googleSheetsWire
	for _, raw := range []string{w.TokenURL, w.RevokeURL, w.APIBase + "/spreadsheets/x/values/A1"} {
		u, _ := url.Parse(raw)
		if !AllowedURL(p.Hosts, u) {
			t.Errorf("%s not on the allowlist", raw)
		}
	}
	if len(p.Hosts) != 2 {
		t.Fatalf("hosts %v", p.Hosts)
	}
	for _, o := range p.Ops {
		if o.Write != (o.Name == "appendRow") || len(o.Scopes) != 1 || o.Scopes[0] != gsheetsScope || o.Check == nil || o.CacheTTL != 0 {
			t.Errorf("%s: %+v", o.Name, o)
		}
	}
	s, err := newGoogleSheets(Secrets{ClientID: "a", ClientSecret: "b"}, w)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"https://www.googleapis.com/drive/v3/files", "https://docs.google.com/spreadsheets/d/x"} {
		if _, err := s.g.hc.Get(raw); !errors.Is(err, ErrEgress) {
			t.Fatalf("egress %s: %v", raw, err)
		}
	}
	if _, err := p.New(Secrets{ClientID: "a"}); err == nil || !strings.Contains(err.Error(), "EXT_GOOGLE_SHEETS_CLIENT_ID") {
		t.Fatalf("built without a secret: %v", err)
	}

	env := map[string]string{"EXT_GOOGLE_CALENDAR_CLIENT_ID": "cal-id", "EXT_GOOGLE_CALENDAR_CLIENT_SECRET": "cal-secret"}
	c, _ := ConfigFromEnv(func(k string) string { return env[k] })
	if c.Enabled[GoogleSheets] || c.Secrets[GoogleSheets].ClientID != "cal-id" || c.Secrets[GoogleSheets].ClientSecret != "cal-secret" {
		t.Fatal("not Calendar's client, or on by default")
	}
	env["EXT_GOOGLE_SHEETS_ENABLED"] = "1"
	env["EXT_GOOGLE_SHEETS_CLIENT_ID"], env["EXT_GOOGLE_SHEETS_CLIENT_SECRET"] = "sheets-id", "sheets-secret"
	c, _ = ConfigFromEnv(func(k string) string { return env[k] })
	if !c.Enabled[GoogleSheets] || c.Secrets[GoogleSheets].ClientID != "sheets-id" || c.Secrets[GoogleCalendar].ClientID != "cal-id" {
		t.Fatal("its own client must win, and the switch must turn it on")
	}
	for _, f := range ReviewBuildFeatures() {
		if f == p.FeatureKey() {
			t.Fatal("in the review build")
		}
	}
}
