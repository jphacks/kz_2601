package extapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Jira Cloud (REST API 3, OAuth 2.0 3LO): free (one `runs` a call),
// per-user OAuth.
//
//	myIssues(limit?, jql?)                              read:jira-work
//	projects(limit?)                                    read:jira-work
//	addIssue(projectKey, summary, issueType?, description?)  write:jira-work (write)
//
// Every call first resolves the person's Jira site (cloud id) with
// /oauth/token/accessible-resources: the first site that has Jira is used
// (one extra GET a call; nothing is kept with the token). Atlassian's 3LO
// takes no PKCE (the client secret stays on the runner); offline_access is
// asked for so a refresh token comes back, and Atlassian rotates it. There
// is no revoke endpoint for 3LO tokens (ErrNoRevoke: the tokens are
// deleted; the person removes the app in Atlassian's connected apps).
//
// Env: EXT_JIRA_ENABLED=1, EXT_JIRA_CLIENT_ID, EXT_JIRA_CLIENT_SECRET
// (Secret Manager). Off by default; not in the review build (NeedsOAuth).
const Jira = "jira"

const (
	jiraScopeRead    = "read:jira-work"
	jiraScopeWrite   = "write:jira-work"
	jiraScopeOffline = "offline_access"
)

// jiraWire is where the provider talks to; tests point it at httptest.
type jiraWire struct {
	AuthURL  string // opened in the browser, not reached by the runner
	TokenURL string
	APIBase  string // https://api.atlassian.com
	// Client is nil in production: NewHTTPClient on the provider's Hosts.
	Client *http.Client
	// Now is nil in production (time.Now).
	Now func() time.Time
}

var jiraWireDefault = jiraWire{
	AuthURL:  "https://auth.atlassian.com/authorize",
	TokenURL: "https://auth.atlassian.com/oauth/token",
	APIBase:  "https://api.atlassian.com",
}

var jiraHosts = []string{"https://auth.atlassian.com", "https://api.atlassian.com"}

func init() {
	register(Provider{ID: Jira, Billing: BillingFree, NeedsOAuth: true, NameJa: "Jira", NameEn: "Jira",
		Ops: []OpSpec{
			{Name: "myIssues", Scopes: []string{jiraScopeRead, jiraScopeOffline}, Args: []string{"limit", "jql"}, Check: checkJiraMyIssues},
			{Name: "projects", Scopes: []string{jiraScopeRead, jiraScopeOffline}, Args: []string{"limit"}, Check: checkJiraProjects},
			{Name: "addIssue", Write: true, Scopes: []string{jiraScopeWrite, jiraScopeOffline}, Args: []string{"projectKey", "summary", "issueType", "description"}, Check: checkJiraAdd},
		},
		Hosts: jiraHosts,
		New: func(sec Secrets) (map[string]Operation, error) {
			c, err := newJira(sec, jiraWireDefault)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"myIssues": OperationFunc(c.myIssues), "projects": OperationFunc(c.projects), "addIssue": OperationFunc(c.addIssue)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) { return newJira(sec, jiraWireDefault) },
	})
}

type jira struct {
	sec  Secrets
	wire jiraWire
	hc   *http.Client
	now  func() time.Time
}

func newJira(sec Secrets, w jiraWire) (*jira, error) {
	if sec.ClientID == "" || sec.ClientSecret == "" {
		return nil, errors.New("EXT_JIRA_CLIENT_ID / _CLIENT_SECRET not set")
	}
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(jiraHosts, DefaultTimeout)
	}
	now := w.Now
	if now == nil {
		now = time.Now
	}
	return &jira{sec: sec, wire: w, hc: hc, now: now}, nil
}

// --- OAuth ---

// AuthURL: Atlassian's 3LO consent. The PKCE challenge is not sent (3LO
// does not take it); audience and prompt=consent are what Atlassian asks
// for. offline_access is always added so a refresh token comes back.
func (c *jira) AuthURL(state, _, redirectURI string, scopes []string) string {
	all := slices.Clone(scopes)
	if !slices.Contains(all, jiraScopeOffline) {
		all = append(all, jiraScopeOffline)
	}
	v := url.Values{
		"audience":      {"api.atlassian.com"},
		"client_id":     {c.sec.ClientID},
		"scope":         {strings.Join(all, " ")},
		"redirect_uri":  {redirectURI},
		"state":         {state},
		"response_type": {"code"},
		"prompt":        {"consent"},
	}
	return c.wire.AuthURL + "?" + v.Encode()
}

func (c *jira) Exchange(ctx context.Context, code, _, redirectURI string) (Token, error) {
	return c.token(ctx, map[string]string{"grant_type": "authorization_code", "code": code, "redirect_uri": redirectURI}, Token{})
}

// Refresh exchanges the refresh token (Atlassian rotates it).
func (c *jira) Refresh(ctx context.Context, t Token) (Token, error) {
	if t.RefreshToken == "" {
		return Token{}, ErrInvalidGrant
	}
	return c.token(ctx, map[string]string{"grant_type": "refresh_token", "refresh_token": t.RefreshToken}, t)
}

// Revoke: Atlassian has no revoke endpoint for 3LO tokens (file comment).
func (c *jira) Revoke(context.Context, Token) error { return ErrNoRevoke }

// token posts a JSON body to the token endpoint (Atlassian takes JSON).
func (c *jira) token(ctx context.Context, form map[string]string, prev Token) (Token, error) {
	form["client_id"] = c.sec.ClientID
	form["client_secret"] = c.sec.ClientSecret
	body, err := json.Marshal(form)
	if err != nil {
		return Token{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.wire.TokenURL, bytes.NewReader(body))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return Token{}, transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := ReadBody(resp.Body)
	if err != nil {
		return Token{}, err
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(b, &e)
		if e.Error == "invalid_grant" || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
			return Token{}, ErrInvalidGrant
		}
		return Token{}, statusError(resp.StatusCode)
	}
	var r struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(b, &r); err != nil || r.AccessToken == "" {
		return Token{}, errors.New("jira: bad token answer")
	}
	t := Token{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken, Scopes: strings.Fields(r.Scope)}
	if t.RefreshToken == "" {
		t.RefreshToken = prev.RefreshToken
	}
	if len(t.Scopes) == 0 {
		t.Scopes = prev.Scopes
	}
	if r.ExpiresIn > 0 {
		t.Expiry = c.now().Add(time.Duration(r.ExpiresIn) * time.Second).UTC()
	}
	return t, nil
}

// jiraCanWrite: the granted scopes cover write:jira-work (unknown scopes
// pass; Jira then answers 403 and the person connects again).
func jiraCanWrite(t *Token) bool {
	if t == nil || len(t.Scopes) == 0 {
		return true
	}
	return slices.Contains(t.Scopes, jiraScopeWrite)
}

// --- args ---

const (
	jiraDefaultLimit   = 20
	jiraMaxLimit       = 50
	jiraJQLMax         = 500
	jiraSummaryMax     = 255
	jiraDescriptionMax = 2000
	jiraIssueTypeMax   = 50
	// jiraDefaultJQL is myIssues without jql: the person's open issues.
	jiraDefaultJQL = "assignee = currentUser() AND resolution = Unresolved ORDER BY updated DESC"
	// jiraIssueFields are the only issue fields asked for.
	jiraIssueFields = "summary,status,duedate,priority"
)

var (
	// jiraKeyRe is a Jira project key (uppercase, up to 10).
	jiraKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,9}$`)
	// jiraCloudRe is a cloud id (UUID).
	jiraCloudRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	// jiraSiteRe is a site URL the browse links may point at.
	jiraSiteRe = regexp.MustCompile(`^https://[a-z0-9-]+\.atlassian\.net$`)
	// jiraIssueTypeRe is an issue type name (Task, Bug, サブタスク ...).
	jiraIssueTypeRe = regexp.MustCompile(`^[\p{L}\p{N} _-]{1,50}$`)
)

// jiraLine checks a one-line text argument (runes, no control chars).
func jiraLine(name, v string, max int, required bool) (string, error) {
	v = strings.TrimSpace(v)
	switch {
	case v == "" && required:
		return "", fmt.Errorf("%w: %s is required", ErrBadArgs, name)
	case !utf8.ValidString(v) || utf8.RuneCountInString(v) > max:
		return "", fmt.Errorf("%w: %s is over %d characters", ErrBadArgs, name, max)
	case strings.ContainsFunc(v, func(r rune) bool { return r < 0x20 || r == 0x7f }):
		return "", fmt.Errorf("%w: %s must be one line", ErrBadArgs, name)
	}
	return v, nil
}

// jiraText checks a multi-line text argument (newline and tab allowed).
func jiraText(name, v string, max int) (string, error) {
	v = strings.TrimSpace(v)
	switch {
	case !utf8.ValidString(v) || utf8.RuneCountInString(v) > max:
		return "", fmt.Errorf("%w: %s is over %d characters", ErrBadArgs, name, max)
	case strings.ContainsFunc(v, func(r rune) bool { return (r < 0x20 && r != '\n' && r != '\t') || r == 0x7f }):
		return "", fmt.Errorf("%w: %s has control characters", ErrBadArgs, name)
	}
	return v, nil
}

func jiraKey(v string) (string, error) {
	v = strings.TrimSpace(v)
	if !jiraKeyRe.MatchString(v) {
		return "", fmt.Errorf("%w: projectKey must be a Jira project key (uppercase)", ErrBadArgs)
	}
	return v, nil
}

func checkJiraMyIssues(a map[string]string) error {
	if _, err := argInt(a, "limit", jiraDefaultLimit, 1, jiraMaxLimit); err != nil {
		return err
	}
	_, err := jiraLine("jql", a["jql"], jiraJQLMax, false)
	return err
}

func checkJiraProjects(a map[string]string) error {
	_, err := argInt(a, "limit", jiraDefaultLimit, 1, jiraMaxLimit)
	return err
}

// jiraAdd is addIssue's checked arguments.
type jiraAdd struct {
	Project, Summary, IssueType, Description string
}

func jiraAddArgs(a map[string]string) (jiraAdd, error) {
	var r jiraAdd
	var err error
	if r.Project, err = jiraKey(a["projectKey"]); err != nil {
		return r, err
	}
	if r.Summary, err = jiraLine("summary", a["summary"], jiraSummaryMax, true); err != nil {
		return r, err
	}
	if r.IssueType = strings.TrimSpace(a["issueType"]); r.IssueType == "" {
		r.IssueType = "Task"
	} else if !jiraIssueTypeRe.MatchString(r.IssueType) || utf8.RuneCountInString(r.IssueType) > jiraIssueTypeMax {
		return r, fmt.Errorf("%w: issueType must be an issue type name", ErrBadArgs)
	}
	r.Description, err = jiraText("description", a["description"], jiraDescriptionMax)
	return r, err
}

func checkJiraAdd(a map[string]string) error {
	_, err := jiraAddArgs(a)
	return err
}

// --- ops ---

// jiraSite is the person's Jira site: the cloud id the API path needs and
// the site URL the browse links point at.
type jiraSite struct {
	ID, URL string
}

// site resolves the person's first Jira site (accessible-resources). No
// site is bad_request (connecting again would not help).
func (c *jira) site(ctx context.Context, in Input) (jiraSite, error) {
	var rs []struct {
		ID     string   `json:"id"`
		URL    string   `json:"url"`
		Scopes []string `json:"scopes"`
	}
	if err := c.do(ctx, in, http.MethodGet, "/oauth/token/accessible-resources", nil, &rs); err != nil {
		return jiraSite{}, err
	}
	for _, r := range rs {
		if !jiraCloudRe.MatchString(r.ID) {
			continue
		}
		s := jiraSite{ID: r.ID}
		if jiraSiteRe.MatchString(r.URL) {
			s.URL = r.URL
		}
		return s, nil
	}
	return jiraSite{}, fmt.Errorf("%w: the Atlassian account has no Jira site", ErrUnknown)
}

func (c *jira) api(s jiraSite, path string) string {
	return "/ex/jira/" + s.ID + "/rest/api/3" + path
}

// jiraIssue is one issue with the fields asked for.
type jiraIssue struct {
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
		Status  struct {
			Name string `json:"name"`
		} `json:"status"`
		DueDate  string `json:"duedate"`
		Priority struct {
			Name string `json:"name"`
		} `json:"priority"`
	} `json:"fields"`
}

// row: title = summary, subtitle = "KEY · status · due", value = priority,
// link = the site's browse page (only an atlassian.net https link).
func (i jiraIssue) row(s jiraSite) Row {
	parts := []string{i.Key}
	if i.Fields.Status.Name != "" {
		parts = append(parts, i.Fields.Status.Name)
	}
	if i.Fields.DueDate != "" {
		parts = append(parts, i.Fields.DueDate)
	}
	r := Row{Title: i.Fields.Summary, Subtitle: strings.Join(parts, " · "), Value: i.Fields.Priority.Name}
	if s.URL != "" && jiraKeyRe.MatchString(strings.TrimRight(i.Key, "0123456789-")) {
		r.URL = s.URL + "/browse/" + i.Key
	}
	return r
}

// myIssues searches issues with jql (default: the person's open issues),
// one page of limit.
func (c *jira) myIssues(ctx context.Context, in Input) (Output, error) {
	n, err := argInt(in.Args, "limit", jiraDefaultLimit, 1, jiraMaxLimit)
	if err != nil {
		return Output{}, err
	}
	jql, err := jiraLine("jql", in.Args["jql"], jiraJQLMax, false)
	if err != nil {
		return Output{}, err
	}
	if jql == "" {
		jql = jiraDefaultJQL
	}
	s, err := c.site(ctx, in)
	if err != nil {
		return Output{}, err
	}
	b, err := json.Marshal(map[string]any{"jql": jql, "maxResults": n, "fields": strings.Split(jiraIssueFields, ",")})
	if err != nil {
		return Output{}, err
	}
	var r struct {
		Issues []jiraIssue `json:"issues"`
	}
	if err := c.do(ctx, in, http.MethodPost, c.api(s, "/search/jql"), b, &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, len(r.Issues))}
	for _, i := range r.Issues {
		out.Rows = append(out.Rows, i.row(s))
	}
	return out, nil
}

// projects lists the projects the person can see, by name, one page of
// limit (title = name, value = key for addIssue).
func (c *jira) projects(ctx context.Context, in Input) (Output, error) {
	n, err := argInt(in.Args, "limit", jiraDefaultLimit, 1, jiraMaxLimit)
	if err != nil {
		return Output{}, err
	}
	s, err := c.site(ctx, in)
	if err != nil {
		return Output{}, err
	}
	q := url.Values{"maxResults": {strconv.Itoa(n)}, "orderBy": {"name"}}
	var r struct {
		Values []struct {
			Key  string `json:"key"`
			Name string `json:"name"`
		} `json:"values"`
	}
	if err := c.do(ctx, in, http.MethodGet, c.api(s, "/project/search?"+q.Encode()), nil, &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, len(r.Values))}
	for _, p := range r.Values {
		row := Row{Title: p.Name, Value: p.Key}
		if s.URL != "" && jiraKeyRe.MatchString(p.Key) {
			row.URL = s.URL + "/browse/" + p.Key
		}
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

// jiraADF is a plain-text description as an Atlassian Document (one
// paragraph a line).
func jiraADF(text string) map[string]any {
	lines := strings.Split(text, "\n")
	content := make([]map[string]any, 0, len(lines))
	for _, l := range lines {
		p := map[string]any{"type": "paragraph"}
		if l != "" {
			p["content"] = []map[string]any{{"type": "text", "text": l}}
		}
		content = append(content, p)
	}
	return map[string]any{"type": "doc", "version": 1, "content": content}
}

// addIssue creates one issue (default type Task). Jira has no idempotency
// key, and a write op is never retried.
func (c *jira) addIssue(ctx context.Context, in Input) (Output, error) {
	if !jiraCanWrite(in.Token) {
		return Output{}, ErrNotConnected
	}
	a, err := jiraAddArgs(in.Args)
	if err != nil {
		return Output{}, err
	}
	s, err := c.site(ctx, in)
	if err != nil {
		return Output{}, err
	}
	fields := map[string]any{"project": map[string]string{"key": a.Project}, "summary": a.Summary, "issuetype": map[string]string{"name": a.IssueType}}
	if a.Description != "" {
		fields["description"] = jiraADF(a.Description)
	}
	b, err := json.Marshal(map[string]any{"fields": fields})
	if err != nil {
		return Output{}, err
	}
	var r struct {
		Key string `json:"key"`
	}
	if err := c.do(ctx, in, http.MethodPost, c.api(s, "/issue"), b, &r); err != nil {
		return Output{}, err
	}
	if r.Key == "" {
		return Output{}, errors.New("jira: bad answer: no issue key")
	}
	var i jiraIssue
	i.Key, i.Fields.Summary = r.Key, a.Summary
	row := i.row(s)
	return Output{ResultID: r.Key, Item: &row}, nil
}

// do calls the API with the person's token. 401 is a token Jira no longer
// takes (ErrInvalidGrant: the connection turns to error); 403 is a missing
// scope or no access (ErrNotConnected: connect again); 400 / 404 are bad
// input or an item the person does not have (bad_request); 429 / 5xx are
// retryable provider errors. The body is not kept (it echoes the request).
func (c *jira) do(ctx context.Context, in Input, method, path string, body []byte, out any) error {
	if in.Token == nil || in.Token.AccessToken == "" {
		return ErrNotConnected
	}
	req, err := http.NewRequestWithContext(ctx, method, c.wire.APIBase+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+in.Token.AccessToken)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := ReadBody(resp.Body)
	if err != nil {
		return err
	}
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("jira: bad answer: %w", err)
		}
		return nil
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrInvalidGrant
	case resp.StatusCode == http.StatusForbidden:
		return ErrNotConnected
	case resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound:
		return &ProviderError{Status: resp.StatusCode, Err: fmt.Errorf("%w: jira refused the request", ErrUnknown)}
	}
	return statusError(resp.StatusCode)
}
