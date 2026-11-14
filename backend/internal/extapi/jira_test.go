package extapi

import (
	"context"
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
	jiraTestID     = "jr-client"
	jiraTestSecret = "jr-secret"
	jiraTestCloud  = "11111111-2222-3333-4444-555555555555"
)

// fakeJira is Atlassian's OAuth and Jira REST API 3 on httptest (no live calls).
type fakeJira struct {
	url string

	mu        sync.Mutex
	refreshes int
	sites     []map[string]any
	siteCalls int
	added     []string
	lastPath  string
	lastQuery url.Values
	lastBody  map[string]any
}

func newFakeJira(t *testing.T) (*fakeJira, *httptest.Server) {
	f := &fakeJira{sites: []map[string]any{{"id": jiraTestCloud, "url": "https://acme.atlassian.net", "name": "acme", "scopes": []string{"read:jira-work"}}}}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		var form map[string]string
		if err := json.NewDecoder(r.Body).Decode(&form); err != nil || r.Header.Get("Content-Type") != "application/json" {
			writeJSON(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
		if form["client_id"] != jiraTestID || form["client_secret"] != jiraTestSecret {
			writeJSON(w, 401, map[string]string{"error": "access_denied"})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch form["grant_type"] {
		case "authorization_code":
			if form["code"] != "jr-code" || form["code_verifier"] != "" {
				writeJSON(w, 403, map[string]string{"error": "invalid_grant"})
				return
			}
			writeJSON(w, 200, map[string]any{"access_token": "jr-access-0", "refresh_token": "jr-refresh-0", "expires_in": 3600,
				"scope": "read:jira-work offline_access", "token_type": "Bearer"})
		case "refresh_token":
			// rotated: the old refresh token is refused
			if form["refresh_token"] != "jr-refresh-"+strconv.Itoa(f.refreshes) {
				writeJSON(w, 403, map[string]string{"error": "invalid_grant"})
				return
			}
			f.refreshes++
			n := strconv.Itoa(f.refreshes)
			writeJSON(w, 200, map[string]any{"access_token": "jr-access-" + n, "refresh_token": "jr-refresh-" + n, "expires_in": 3600,
				"scope": "read:jira-work offline_access"})
		default:
			writeJSON(w, 400, map[string]string{"error": "unsupported_grant_type"})
		}
	})
	authed := func(w http.ResponseWriter, r *http.Request) bool {
		switch tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); {
		case tok == "noscope":
			writeJSON(w, 403, map[string]any{"errorMessages": []string{"scope"}})
			return false
		case tok == "busy":
			writeJSON(w, 503, map[string]any{"errorMessages": []string{"down"}})
			return false
		case tok == "limited":
			w.Header().Set("Retry-After", "30")
			writeJSON(w, 429, map[string]any{"errorMessages": []string{"rate limit"}})
			return false
		case !strings.HasPrefix(tok, "jr-access-"):
			writeJSON(w, 401, map[string]any{"errorMessages": []string{"Unauthorized"}})
			return false
		}
		return true
	}
	record := func(r *http.Request) {
		f.lastPath, f.lastQuery = r.URL.Path, r.URL.Query()
	}
	mux.HandleFunc("GET /oauth/token/accessible-resources", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.siteCalls++
		writeJSON(w, 200, f.sites)
	})
	issues := []map[string]any{
		{"key": "ACME-12", "fields": map[string]any{"summary": "資料を送る", "status": map[string]any{"name": "To Do"}, "duedate": "2026-11-20", "priority": map[string]any{"name": "High"}}},
		{"key": "ACME-7", "fields": map[string]any{"summary": "定例", "status": map[string]any{"name": "In Progress"}, "duedate": nil, "priority": nil}},
	}
	mux.HandleFunc("POST /ex/jira/{cloud}/rest/api/3/search/jql", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		record(r)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || r.PathValue("cloud") != jiraTestCloud {
			writeJSON(w, 404, map[string]any{"errorMessages": []string{"no site"}})
			return
		}
		f.lastBody = body
		if strings.Contains(body["jql"].(string), "bad jql") {
			writeJSON(w, 400, map[string]any{"errorMessages": []string{"Error in the JQL Query"}})
			return
		}
		writeJSON(w, 200, map[string]any{"issues": issues})
	})
	mux.HandleFunc("GET /ex/jira/{cloud}/rest/api/3/project/search", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		record(r)
		writeJSON(w, 200, map[string]any{"values": []map[string]any{{"id": "10000", "key": "ACME", "name": "Acme"}, {"id": "10001", "key": "OPS", "name": "Ops"}}})
	})
	mux.HandleFunc("POST /ex/jira/{cloud}/rest/api/3/issue", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		record(r)
		var body struct {
			Fields map[string]any `json:"fields"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || r.Header.Get("Content-Type") != "application/json" {
			writeJSON(w, 400, map[string]any{"errorMessages": []string{"bad body"}})
			return
		}
		f.lastBody = body.Fields
		if p, _ := body.Fields["project"].(map[string]any); p["key"] == "NOPE" {
			writeJSON(w, 400, map[string]any{"errors": map[string]string{"project": "project is required"}})
			return
		}
		key := "ACME-" + strconv.Itoa(100+len(f.added))
		f.added = append(f.added, key)
		writeJSON(w, 201, map[string]any{"id": "10100", "key": key, "self": "https://acme.atlassian.net/rest/api/3/issue/10100"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, srv
}

func (f *fakeJira) wire(srv *httptest.Server) jiraWire {
	return jiraWire{AuthURL: srv.URL + "/authorize", TokenURL: srv.URL + "/oauth/token", APIBase: srv.URL, Client: srv.Client(),
		Now: func() time.Time { return time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC) }}
}

func useJira(t *testing.T) (*env, *fakeJira) {
	f, srv := newFakeJira(t)
	old := jiraWireDefault
	jiraWireDefault = f.wire(srv)
	t.Cleanup(func() { jiraWireDefault = old })
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[Jira] = true
		c.Secrets = map[string]Secrets{Jira: {ClientID: jiraTestID, ClientSecret: jiraTestSecret}}
	}})
	return e, f
}

// connectJira connects user asking for scopes; want is the scope param.
func (e *env) connectJira(f *fakeJira, scopes []string, want string) {
	e.t.Helper()
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, user, Jira, scopes)
	if err != nil {
		e.t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/authorize?") || q.Get("scope") != want || q.Get("audience") != "api.atlassian.com" || q.Get("prompt") != "consent" ||
		q.Get("client_id") != jiraTestID || q.Get("redirect_uri") != "https://runner.example/oauth/callback/jira" || q.Get("client_secret") != "" || q.Get("code_challenge") != "" {
		e.t.Fatalf("consent url %s", auth)
	}
	if back := e.svc.Callback(ctx, Jira, q.Get("state"), "jr-code"); back != "tailorit://connected?provider=jira&status=ok" {
		e.t.Fatalf("callback → %s", back)
	}
}

// connect (no PKCE, reads + offline_access) → myIssues (site resolved a
// call) → addIssue needs write:jira-work (409) → refresh (rotated) →
// revoke deletes the tokens only (no endpoint).
func TestJiraFlow(t *testing.T) {
	e, f := useJira(t)
	ctx := context.Background()
	e.connectJira(f, nil, "read:jira-work offline_access")

	res, err := e.call(e.req(Jira, "myIssues", map[string]string{"limit": "10"}))
	if err != nil {
		t.Fatal(err)
	}
	rows := res.Output.Rows
	if len(rows) != 2 || rows[0] != (Row{Title: "資料を送る", Subtitle: "ACME-12 · To Do · 2026-11-20", Value: "High", URL: "https://acme.atlassian.net/browse/ACME-12"}) ||
		rows[1] != (Row{Title: "定例", Subtitle: "ACME-7 · In Progress", URL: "https://acme.atlassian.net/browse/ACME-7"}) {
		t.Fatalf("rows %+v", rows)
	}
	if f.lastPath != "/ex/jira/"+jiraTestCloud+"/rest/api/3/search/jql" || f.lastBody["jql"] != jiraDefaultJQL || f.lastBody["maxResults"] != float64(10) || f.siteCalls != 1 {
		t.Fatalf("request %s %v sites=%d", f.lastPath, f.lastBody, f.siteCalls)
	}
	if fs, _ := f.lastBody["fields"].([]any); len(fs) != 4 {
		t.Fatalf("fields %v", f.lastBody["fields"])
	}
	if e.runs() != 0 || res.Charge.Coins != 0 {
		t.Fatalf("runs = %d (NoCharge: Atlassian 8.3, nothing is counted)", e.runs())
	}

	// read-only grant: adding is 409 and gives the run back
	if _, err := e.call(e.req(Jira, "addIssue", map[string]string{"projectKey": "ACME", "summary": "x"})); !errors.Is(err, ErrNotConnected) || e.runs() != 0 || len(f.added) != 0 {
		t.Fatalf("addIssue without scope: %v runs=%d", err, e.runs())
	}
	e.connectJira(f, []string{jiraScopeWrite}, "write:jira-work offline_access")
	tok, _ := e.svc.Conns.Token(ctx, user, Jira)
	tok.Scopes = []string{jiraScopeWrite, jiraScopeOffline}
	if err := e.svc.Conns.Save(ctx, user, Jira, tok); err != nil {
		t.Fatal(err)
	}
	res, err = e.call(e.req(Jira, "addIssue", map[string]string{"projectKey": "ACME", "summary": " 資料を送る ", "description": "先方へ\n添付あり"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Output.ResultID != "ACME-100" || res.Output.Item == nil || *res.Output.Item != (Row{Title: "資料を送る", Subtitle: "ACME-100", URL: "https://acme.atlassian.net/browse/ACME-100"}) {
		t.Fatalf("added %+v", res.Output)
	}
	if it, _ := f.lastBody["issuetype"].(map[string]any); it["name"] != "Task" || f.lastBody["summary"] != "資料を送る" {
		t.Fatalf("body %v", f.lastBody)
	}
	if d, _ := f.lastBody["description"].(map[string]any); d["type"] != "doc" || len(d["content"].([]any)) != 2 {
		t.Fatalf("description %v", f.lastBody["description"])
	}

	// expired → refreshed; the refresh token is rotated
	tok, _ = e.svc.Conns.Token(ctx, user, Jira)
	tok.Expiry = e.c.t.Add(-time.Minute)
	if err := e.svc.Conns.Save(ctx, user, Jira, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Jira, "projects", nil)); err != nil {
		t.Fatal(err)
	}
	tok, err = e.svc.Conns.Token(ctx, user, Jira)
	if err != nil || tok.AccessToken != "jr-access-1" || tok.RefreshToken != "jr-refresh-1" || f.refreshes != 1 {
		t.Fatalf("refreshed: %v refreshes=%d", err, f.refreshes)
	}

	// cut: no revoke endpoint; the tokens are deleted
	if err := e.svc.Revoke(ctx, user, Jira); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Jira); c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("after revoke %+v", c)
	}
}

// A 401 on a call turns the connection to error.
func TestJiraRevokedAtProvider(t *testing.T) {
	e, f := useJira(t)
	e.connectJira(f, nil, "read:jira-work offline_access")
	ctx := context.Background()
	if err := e.svc.Conns.Save(ctx, user, Jira, Token{AccessToken: "dead"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Jira, "myIssues", nil)); !errors.Is(err, ErrNotConnected) || e.runs() != 0 {
		t.Fatalf("401: %v runs=%d", err, e.runs())
	}
	if c, _, _ := e.mem.Get(ctx, user, Jira); c.Status != StatusError {
		t.Fatalf("status %q", c.Status)
	}
}

func jiraTestClient(t *testing.T) (*jira, *fakeJira) {
	f, srv := newFakeJira(t)
	c, err := newJira(Secrets{ClientID: jiraTestID, ClientSecret: jiraTestSecret}, f.wire(srv))
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

func TestJiraOps(t *testing.T) {
	c, f := jiraTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: "jr-access-0"}
	// a given jql is sent as is
	if _, err := c.myIssues(ctx, Input{Token: tok, Args: map[string]string{"jql": "project = ACME"}}); err != nil || f.lastBody["jql"] != "project = ACME" || f.lastBody["maxResults"] != float64(20) {
		t.Fatalf("jql: %v %v", err, f.lastBody)
	}
	// Jira's 400 (a JQL it cannot run) is a bad request, not a retry
	_, err := c.myIssues(ctx, Input{Token: tok, Args: map[string]string{"jql": "bad jql"}})
	var pe *ProviderError
	if Reason(err) != ReasonBadRequest || !errors.As(err, &pe) || pe.Retryable {
		t.Fatalf("bad jql: %v", err)
	}
	// projects: by name, limit, value = key
	out, err := c.projects(ctx, Input{Token: tok, Args: map[string]string{"limit": "5"}})
	if err != nil || f.lastQuery.Get("maxResults") != "5" || f.lastQuery.Get("orderBy") != "name" || len(out.Rows) != 2 ||
		out.Rows[0] != (Row{Title: "Acme", Value: "ACME", URL: "https://acme.atlassian.net/browse/ACME"}) {
		t.Fatalf("projects: %v %v %+v", err, f.lastQuery, out.Rows)
	}
	// a site without a usable URL gives rows without links
	f.sites = []map[string]any{{"id": jiraTestCloud, "url": "https://evil.example.com"}}
	out, err = c.projects(ctx, Input{Token: tok})
	if err != nil || out.Rows[0].URL != "" {
		t.Fatalf("no link: %v %+v", err, out.Rows)
	}
	// no Jira site: bad request, nothing else called
	f.sites = []map[string]any{{"id": "not-a-uuid", "url": "https://acme.atlassian.net"}}
	f.lastPath = ""
	if _, err := c.projects(ctx, Input{Token: tok}); Reason(err) != ReasonBadRequest || f.lastPath != "" {
		t.Fatalf("no site: %v %s", err, f.lastPath)
	}
	f.sites = nil
	if _, err := c.myIssues(ctx, Input{Token: tok}); Reason(err) != ReasonBadRequest {
		t.Fatalf("no sites: %v", err)
	}
}

func TestJiraAddIssue(t *testing.T) {
	c, f := jiraTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: "jr-access-0", Scopes: []string{jiraScopeWrite, jiraScopeOffline}}
	out, err := c.addIssue(ctx, Input{Token: tok, Args: map[string]string{"projectKey": "OPS", "summary": "障害", "issueType": "Bug"}})
	if err != nil || out.ResultID != "ACME-100" || out.Item.Title != "障害" || f.lastPath != "/ex/jira/"+jiraTestCloud+"/rest/api/3/issue" {
		t.Fatalf("add: %v %+v %s", err, out, f.lastPath)
	}
	if p, _ := f.lastBody["project"].(map[string]any); p["key"] != "OPS" || f.lastBody["issuetype"].(map[string]any)["name"] != "Bug" || f.lastBody["description"] != nil {
		t.Fatalf("body %v", f.lastBody)
	}
	// a project the person does not have (Jira's 400) is a bad request
	if _, err := c.addIssue(ctx, Input{Token: tok, Args: map[string]string{"projectKey": "NOPE", "summary": "x"}}); Reason(err) != ReasonBadRequest {
		t.Fatalf("unknown project: %v", err)
	}
	// reads only (before any call), or Jira's 403
	n := len(f.added)
	f.lastPath = ""
	if _, err := c.addIssue(ctx, Input{Token: &Token{AccessToken: "jr-access-0", Scopes: []string{jiraScopeRead}}, Args: map[string]string{"projectKey": "OPS", "summary": "x"}}); !errors.Is(err, ErrNotConnected) || f.lastPath != "" {
		t.Fatalf("reads only: %v", err)
	}
	if _, err := c.addIssue(ctx, Input{Token: &Token{AccessToken: "noscope"}, Args: map[string]string{"projectKey": "OPS", "summary": "x"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("403: %v", err)
	}
	if _, err := c.addIssue(ctx, Input{Token: &Token{AccessToken: "busy", Scopes: tok.Scopes}, Args: map[string]string{"projectKey": "OPS", "summary": "x"}}); Reason(err) != ReasonUpstream {
		t.Fatalf("503: %v", err)
	}
	if len(f.added) != n {
		t.Fatalf("added %v", f.added)
	}
	// bad arguments are refused before anything is sent
	f.lastPath = ""
	for _, a := range []map[string]string{{}, {"projectKey": "acme", "summary": "x"}, {"projectKey": "OPS", "summary": "a\nb"}} {
		if _, err := c.addIssue(ctx, Input{Token: tok, Args: a}); !errors.Is(err, ErrBadArgs) || f.lastPath != "" {
			t.Fatalf("%v: %v %s", a, err, f.lastPath)
		}
	}
}

func TestJiraErrors(t *testing.T) {
	c, f := jiraTestClient(t)
	ctx := context.Background()
	if _, err := c.myIssues(ctx, Input{Token: &Token{AccessToken: "noscope"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("403: %v", err)
	}
	if _, err := c.myIssues(ctx, Input{Token: &Token{AccessToken: "dead"}}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("401: %v", err)
	}
	for _, tok := range []string{"busy", "limited"} {
		_, err := c.myIssues(ctx, Input{Token: &Token{AccessToken: tok}})
		var pe *ProviderError
		if !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream {
			t.Fatalf("%s: %v", tok, err)
		}
	}
	if _, err := c.myIssues(ctx, Input{}); !errors.Is(err, ErrNotConnected) || f.siteCalls != 0 {
		t.Fatalf("no token: %v", err)
	}
	// OAuth edges
	if _, err := c.Refresh(ctx, Token{AccessToken: "x"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("refresh without a refresh token: %v", err)
	}
	if _, err := c.Refresh(ctx, Token{RefreshToken: "gone"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("refused refresh: %v", err)
	}
	if _, err := c.Exchange(ctx, "bad", "v", "https://runner.example/oauth/callback/jira"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("bad code: %v", err)
	}
	if err := c.Revoke(ctx, Token{RefreshToken: "jr-refresh-0"}); !errors.Is(err, ErrNoRevoke) {
		t.Fatalf("revoke: %v", err)
	}
}

// Check refuses bad arguments before anything is counted.
func TestJiraCheck(t *testing.T) {
	p, _ := Lookup(Jira)
	bad := map[string][]map[string]string{
		"myIssues": {{"limit": "0"}, {"limit": "51"}, {"limit": "x"}, {"jql": "a\nb"}, {"jql": strings.Repeat("a", jiraJQLMax+1)}, {"jql": "https://x.example/"}, {"projectKey": "A"}},
		"projects": {{"limit": "51"}, {"jql": "x"}},
		"addIssue": {{}, {"summary": "x"}, {"projectKey": "ACME"}, {"projectKey": "acme", "summary": "x"}, {"projectKey": "ACME-1", "summary": "x"}, {"projectKey": "ABCDEFGHIJK", "summary": "x"},
			{"projectKey": "ACME", "summary": "  "}, {"projectKey": "ACME", "summary": strings.Repeat("あ", jiraSummaryMax+1)}, {"projectKey": "ACME", "summary": "a\nb"}, {"projectKey": "ACME", "summary": "\xff"},
			{"projectKey": "ACME", "summary": "x", "issueType": "Bug;"}, {"projectKey": "ACME", "summary": "x", "description": strings.Repeat("d", jiraDescriptionMax+1)},
			{"projectKey": "ACME", "summary": "x", "description": "a\x1bb"}, {"projectKey": "ACME", "summary": "x", "limit": "1"}},
	}
	for op, cases := range bad {
		for i, a := range cases {
			if _, err := p.CheckArgs(op, a); !errors.Is(err, ErrUnknown) {
				t.Errorf("%s case %d: %v", op, i, err)
			}
		}
	}
	for op, a := range map[string]map[string]string{
		"myIssues": {"limit": "50", "jql": "project = ACME AND status != Done ORDER BY created DESC"},
		"projects": {},
		"addIssue": {"projectKey": "ACME_2", "summary": strings.Repeat("あ", jiraSummaryMax), "issueType": "サブタスク", "description": "1行目\n\t2行目"},
	} {
		if _, err := p.CheckArgs(op, a); err != nil {
			t.Errorf("%s: %v", op, err)
		}
	}
}

// The real endpoints are on the allowlist; nothing else is reachable;
// without the client id / secret the provider stays off; off by default
// and never in the review build; only addIssue writes.
func TestJiraDefinition(t *testing.T) {
	p, ok := Lookup(Jira)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "JIRA" || len(p.Hosts) != 2 {
		t.Fatalf("%+v", p)
	}
	w := jiraWireDefault
	for _, raw := range []string{w.AuthURL, w.TokenURL, w.APIBase + "/oauth/token/accessible-resources", w.APIBase + "/ex/jira/x/rest/api/3/search/jql"} {
		u, _ := url.Parse(raw)
		if !AllowedURL(p.Hosts, u) {
			t.Errorf("%s not on the allowlist", raw)
		}
	}
	for _, o := range p.Ops {
		if o.Write != (o.Name == "addIssue") {
			t.Errorf("%s write=%v", o.Name, o.Write)
		}
		for _, s := range o.Scopes {
			if (s == jiraScopeWrite && !o.Write) || strings.HasPrefix(s, "manage:") || strings.HasPrefix(s, "delete:") {
				t.Errorf("%s asks for a wide scope %s", o.Name, s)
			}
		}
	}
	c, err := newJira(Secrets{ClientID: "a", ClientSecret: "b"}, w)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.hc.Get("https://evil.example.com/"); !errors.Is(err, ErrEgress) {
		t.Fatalf("egress: %v", err)
	}
	if _, err := p.New(Secrets{ClientSecret: "b"}); err == nil {
		t.Fatal("built without a client id")
	}
	cfg, _ := ConfigFromEnv(func(string) string { return "" })
	if cfg.Enabled[Jira] {
		t.Fatal("on by default")
	}
	for _, f := range ReviewBuildFeatures() {
		if f == p.FeatureKey() {
			t.Fatal("in the review build")
		}
	}
}

// Atlassian Developer Terms 8.3: never charged, never Pro only; the data
// stays with the person who connected.
func TestJiraTerms(t *testing.T) {
	p, _ := Lookup(Jira)
	if !p.Terms.NoCharge || !p.Terms.ShowOwnDataOnly || p.Terms.NoAI {
		t.Fatalf("%+v", p.Terms)
	}
}
