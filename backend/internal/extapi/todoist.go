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

// Todoist (#1496, API v1): free (one `runs` a call), OAuth with PKCE.
//
//	listTasks(filter?, projectId?, limit?)                                  data:read
//	addTask(content, description?, dueString?, dueDate?, priority?, projectId?)  task:add
//
// task:add only adds tasks (it cannot read or change any); data:read only
// reads. data:read_write is never asked for. New Todoist apps get an access
// token for an hour and a rotated refresh token; legacy apps get a
// long-lived token and no refresh token (then a refused token means connect
// again).
//
// Env: EXT_TODOIST_ENABLED=1, EXT_TODOIST_CLIENT_ID, EXT_TODOIST_CLIENT_SECRET
// (Secret Manager). Off by default; not in the review build (NeedsOAuth).
// Todoist's terms: name the integration "… for Todoist" and say it is not
// made by or affiliated with Todoist.
const Todoist = "todoist"

const (
	todoistScopeRead  = "data:read"
	todoistScopeAdd   = "task:add"
	todoistScopeWrite = "data:read_write" // covers both; accepted, never asked for
)

// todoistWire is where the provider talks to; tests point it at httptest.
type todoistWire struct {
	AuthURL   string // opened in the browser, not reached by the runner
	TokenURL  string
	RevokeURL string
	APIBase   string
	// Client is nil in production: NewHTTPClient on the provider's Hosts.
	Client *http.Client
	// Now is nil in production (time.Now).
	Now func() time.Time
}

var todoistWireDefault = todoistWire{
	AuthURL:   "https://app.todoist.com/oauth/authorize",
	TokenURL:  "https://api.todoist.com/oauth/access_token",
	RevokeURL: "https://api.todoist.com/api/v1/revoke",
	APIBase:   "https://api.todoist.com/api/v1",
}

var todoistHosts = []string{"https://api.todoist.com"}

func init() {
	register(Provider{ID: Todoist, Billing: BillingFree, NeedsOAuth: true,
		Ops: []OpSpec{
			{Name: "listTasks", Scopes: []string{todoistScopeRead}, Args: []string{"filter", "projectId", "limit"}},
			{Name: "addTask", Write: true, Scopes: []string{todoistScopeAdd}, Args: []string{"content", "description", "dueString", "dueDate", "priority", "projectId"}},
		},
		Hosts: todoistHosts,
		New: func(sec Secrets) (map[string]Operation, error) {
			c, err := newTodoist(sec, todoistWireDefault)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"listTasks": OperationFunc(c.listTasks), "addTask": OperationFunc(c.addTask)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) { return newTodoist(sec, todoistWireDefault) },
	})
}

// todoist is the provider's ops and OAuth client.
type todoist struct {
	sec  Secrets
	wire todoistWire
	hc   *http.Client
	now  func() time.Time
}

func newTodoist(sec Secrets, w todoistWire) (*todoist, error) {
	if sec.ClientID == "" || sec.ClientSecret == "" {
		return nil, errors.New("EXT_TODOIST_CLIENT_ID / _CLIENT_SECRET not set")
	}
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(todoistHosts, DefaultTimeout)
	}
	now := w.Now
	if now == nil {
		now = time.Now
	}
	return &todoist{sec: sec, wire: w, hc: hc, now: now}, nil
}

// --- OAuth ---

// AuthURL: Todoist's scopes are comma separated. Todoist does not add to
// the scopes granted before, so asking for task:add asks for data:read too
// (connecting again for a write must not lose the reads).
func (c *todoist) AuthURL(state, codeChallenge, redirectURI string, scopes []string) string {
	if slices.Contains(scopes, todoistScopeAdd) && !slices.Contains(scopes, todoistScopeRead) {
		scopes = append([]string{todoistScopeRead}, scopes...)
	}
	v := url.Values{
		"client_id":             {c.sec.ClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {strings.Join(scopes, ",")},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}
	return c.wire.AuthURL + "?" + v.Encode()
}

func (c *todoist) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (Token, error) {
	return c.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {codeVerifier}, "redirect_uri": {redirectURI}}, Token{})
}

// Refresh exchanges the refresh token (Todoist rotates it). A legacy
// token without one cannot be renewed: connect again.
func (c *todoist) Refresh(ctx context.Context, t Token) (Token, error) {
	if t.RefreshToken == "" {
		return Token{}, ErrInvalidGrant
	}
	return c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.RefreshToken}}, t)
}

// Revoke revokes the grant at Todoist (RFC 7009, client Basic auth). The
// revoke endpoint takes an access token, so an expired one is renewed first.
func (c *todoist) Revoke(ctx context.Context, t Token) error {
	if !t.Expiry.IsZero() && t.expired(c.now()) && t.RefreshToken != "" {
		nt, err := c.Refresh(ctx, t)
		if errors.Is(err, ErrInvalidGrant) {
			return nil // already gone at Todoist
		}
		if err != nil {
			return err
		}
		t = nt
	}
	if t.AccessToken == "" {
		return nil
	}
	form := url.Values{"token": {t.AccessToken}, "token_type_hint": {"access_token"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.wire.RevokeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.sec.ClientID, c.sec.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = ReadBody(resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500, Err: errors.New("revoke refused")}
}

func (c *todoist) token(ctx context.Context, form url.Values, prev Token) (Token, error) {
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
		return Token{}, errors.New("todoist: bad token answer")
	}
	t := Token{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken, Scopes: todoistSplitScopes(r.Scope)}
	// a legacy app's 10-year expires_in is no expiry
	if r.ExpiresIn > 0 && r.ExpiresIn < 365*24*60*60 {
		t.Expiry = c.now().Add(time.Duration(r.ExpiresIn) * time.Second).UTC()
	}
	if len(t.Scopes) == 0 {
		t.Scopes = prev.Scopes
	}
	return t, nil
}

func todoistSplitScopes(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
}

// todoistCanAdd: the granted scopes cover task:add (unknown scopes pass;
// Todoist then answers 403 and the person connects again). Reads are not
// checked here: a connection made for a write has data:read too (AuthURL),
// and Todoist's 403 covers the rest.
func todoistCanAdd(t *Token) bool {
	if t == nil || len(t.Scopes) == 0 {
		return true
	}
	return slices.Contains(t.Scopes, todoistScopeAdd) || slices.Contains(t.Scopes, todoistScopeWrite)
}

// --- ops ---

const (
	todoistDefaultLimit = 20
	todoistMaxLimit     = 50
)

var todoistIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// todoistText checks a free-text argument's length (runes) and control chars.
func todoistText(name, v string, max int, required bool) (string, error) {
	v = strings.TrimSpace(v)
	switch {
	case v == "" && required:
		return "", fmt.Errorf("%w: %s is required", ErrUnknown, name)
	case !utf8.ValidString(v) || utf8.RuneCountInString(v) > max:
		return "", fmt.Errorf("%w: %s is over %d characters", ErrUnknown, name, max)
	case strings.ContainsFunc(v, func(r rune) bool { return r < 0x20 && r != '\n' && r != '\t' }):
		return "", fmt.Errorf("%w: %s has control characters", ErrUnknown, name)
	}
	return v, nil
}

func todoistProjectID(v string) (string, error) {
	if v != "" && !todoistIDRe.MatchString(v) {
		return "", fmt.Errorf("%w: projectId", ErrUnknown)
	}
	return v, nil
}

type todoistTask struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Due     *struct {
		Date     string `json:"date"`
		Datetime string `json:"datetime"`
		String   string `json:"string"`
	} `json:"due"`
}

func (t todoistTask) row() Row {
	r := Row{Title: t.Content}
	if t.Due != nil {
		r.Subtitle, r.Value = t.Due.String, t.Due.Datetime
		if r.Value == "" {
			r.Value = t.Due.Date
		}
	}
	if todoistIDRe.MatchString(t.ID) {
		r.URL = "https://app.todoist.com/app/task/" + t.ID
	}
	return r
}

// listTasks lists the person's open tasks: a Todoist filter ("today |
// overdue"), one project, or all; one page of limit (default 20).
func (c *todoist) listTasks(ctx context.Context, in Input) (Output, error) {
	filter, err := todoistText("filter", in.Args["filter"], 1024, false)
	if err != nil {
		return Output{}, err
	}
	project, err := todoistProjectID(in.Args["projectId"])
	if err != nil {
		return Output{}, err
	}
	if filter != "" && project != "" {
		return Output{}, fmt.Errorf("%w: filter and projectId cannot be used together", ErrUnknown)
	}
	n := todoistDefaultLimit
	if v := in.Args["limit"]; v != "" {
		x, err := strconv.Atoi(v)
		if err != nil || x < 1 || x > todoistMaxLimit {
			return Output{}, fmt.Errorf("%w: limit must be 1-%d", ErrUnknown, todoistMaxLimit)
		}
		n = x
	}
	v := url.Values{"limit": {strconv.Itoa(n)}}
	path := "/tasks"
	switch {
	case filter != "":
		path = "/tasks/filter"
		v.Set("query", filter)
	case project != "":
		v.Set("project_id", project)
	}
	var r struct {
		Results []todoistTask `json:"results"`
	}
	if err := c.do(ctx, in, http.MethodGet, path+"?"+v.Encode(), nil, &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, len(r.Results))}
	for _, t := range r.Results {
		out.Rows = append(out.Rows, t.row())
	}
	return out, nil
}

// addTask adds one task (to the inbox unless projectId). dueString is
// Todoist's natural language ("明日 10時"); dueDate is YYYY-MM-DD. The
// request id is Todoist's X-Request-Id, so a retried request adds no second task.
func (c *todoist) addTask(ctx context.Context, in Input) (Output, error) {
	if !todoistCanAdd(in.Token) {
		return Output{}, ErrNotConnected
	}
	content, err := todoistText("content", in.Args["content"], 500, true)
	if err != nil {
		return Output{}, err
	}
	desc, err := todoistText("description", in.Args["description"], 2000, false)
	if err != nil {
		return Output{}, err
	}
	dueString, err := todoistText("dueString", in.Args["dueString"], 150, false)
	if err != nil {
		return Output{}, err
	}
	dueDate := in.Args["dueDate"]
	if dueDate != "" {
		if _, err := time.Parse(time.DateOnly, dueDate); err != nil {
			return Output{}, fmt.Errorf("%w: dueDate must be YYYY-MM-DD", ErrUnknown)
		}
	}
	if dueString != "" && dueDate != "" {
		return Output{}, fmt.Errorf("%w: dueString and dueDate cannot be used together", ErrUnknown)
	}
	project, err := todoistProjectID(in.Args["projectId"])
	if err != nil {
		return Output{}, err
	}
	var priority int
	if v := in.Args["priority"]; v != "" {
		if priority, err = strconv.Atoi(v); err != nil || priority < 1 || priority > 4 {
			return Output{}, fmt.Errorf("%w: priority must be 1-4", ErrUnknown)
		}
	}
	body := struct {
		Content     string `json:"content"`
		Description string `json:"description,omitempty"`
		DueString   string `json:"due_string,omitempty"`
		DueDate     string `json:"due_date,omitempty"`
		Priority    int    `json:"priority,omitempty"`
		ProjectID   string `json:"project_id,omitempty"`
	}{Content: content, Description: desc, DueString: dueString, DueDate: dueDate, Priority: priority, ProjectID: project}
	b, err := json.Marshal(body)
	if err != nil {
		return Output{}, err
	}
	var t todoistTask
	if err := c.do(ctx, in, http.MethodPost, "/tasks", b, &t); err != nil {
		return Output{}, err
	}
	row := t.row()
	return Output{ResultID: t.ID, Item: &row}, nil
}

// do calls the API with the person's token. 401 is a token Todoist no
// longer takes (ErrInvalidGrant: the connection turns to error); 403 is a
// missing scope (ErrNotConnected: connect again with it); 429 / 5xx are
// retryable provider errors.
func (c *todoist) do(ctx context.Context, in Input, method, path string, body []byte, out any) error {
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
		if in.RequestID != "" {
			req.Header.Set("X-Request-Id", in.RequestID)
		}
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
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("todoist: bad answer: %w", err)
		}
		return nil
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrInvalidGrant
	case resp.StatusCode == http.StatusForbidden:
		return ErrNotConnected
	}
	var e struct {
		ErrorTag string `json:"error_tag"`
	}
	_ = json.Unmarshal(b, &e)
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode == 429 || resp.StatusCode >= 500,
		Err: fmt.Errorf("todoist %s", e.ErrorTag)}
}
