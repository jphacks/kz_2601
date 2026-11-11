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
)

// Asana (#1683, REST API 1.0): free (one `runs` a call), OAuth with PKCE
// and granular scopes.
//
//	myTasks(workspaceId?, limit?)   tasks:read workspaces:read
//	projectTasks(projectId, limit?) tasks:read
//	complete(taskId)                tasks:write
//
// Only open tasks are listed (completed_since=now). workspaces:read is for
// myTasks without a workspaceId (the person's only workspace is used).
// tasks:write is asked for only when complete is first used. Asana's access
// token lives an hour; the refresh token is long-lived and not rotated.
// Asana's scopes are not additive, so a consent always asks for the reads too.
//
// Env: EXT_ASANA_ENABLED=1, EXT_ASANA_CLIENT_ID, EXT_ASANA_CLIENT_SECRET
// (Secret Manager). Off by default; not in the review build (NeedsOAuth).
const Asana = "asana"

const (
	asanaScopeTasksRead      = "tasks:read"
	asanaScopeWorkspacesRead = "workspaces:read"
	asanaScopeTasksWrite     = "tasks:write"
)

// asanaReadScopes are asked for on every consent (Asana replaces the grant).
var asanaReadScopes = []string{asanaScopeTasksRead, asanaScopeWorkspacesRead}

// asanaWire is where the provider talks to; tests point it at httptest.
type asanaWire struct {
	AuthURL   string // opened in the browser, not reached by the runner
	TokenURL  string
	RevokeURL string
	APIBase   string
	// Client is nil in production: NewHTTPClient on the provider's Hosts.
	Client *http.Client
	// Now is nil in production (time.Now).
	Now func() time.Time
}

var asanaWireDefault = asanaWire{
	AuthURL:   "https://app.asana.com/-/oauth_authorize",
	TokenURL:  "https://app.asana.com/-/oauth_token",
	RevokeURL: "https://app.asana.com/-/oauth_revoke",
	APIBase:   "https://app.asana.com/api/1.0",
}

var asanaHosts = []string{"https://app.asana.com"}

func init() {
	register(Provider{ID: Asana, Billing: BillingFree, NeedsOAuth: true, NameJa: "Asana", NameEn: "Asana",
		Ops: []OpSpec{
			{Name: "myTasks", Scopes: []string{asanaScopeTasksRead, asanaScopeWorkspacesRead}, Args: []string{"workspaceId", "limit"}, Check: checkAsanaMyTasks},
			{Name: "projectTasks", Scopes: []string{asanaScopeTasksRead}, Args: []string{"projectId", "limit"}, Check: checkAsanaProjectTasks},
			{Name: "complete", Write: true, Scopes: []string{asanaScopeTasksWrite}, Args: []string{"taskId"}, Check: checkAsanaComplete},
		},
		Hosts: asanaHosts,
		New: func(sec Secrets) (map[string]Operation, error) {
			c, err := newAsana(sec, asanaWireDefault)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"myTasks": OperationFunc(c.myTasks), "projectTasks": OperationFunc(c.projectTasks),
				"complete": OperationFunc(c.complete)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) { return newAsana(sec, asanaWireDefault) },
	})
}

// asana is the provider's ops and OAuth client.
type asana struct {
	sec  Secrets
	wire asanaWire
	hc   *http.Client
	now  func() time.Time
}

func newAsana(sec Secrets, w asanaWire) (*asana, error) {
	if sec.ClientID == "" || sec.ClientSecret == "" {
		return nil, errors.New("EXT_ASANA_CLIENT_ID / _CLIENT_SECRET not set")
	}
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(asanaHosts, DefaultTimeout)
	}
	now := w.Now
	if now == nil {
		now = time.Now
	}
	return &asana{sec: sec, wire: w, hc: hc, now: now}, nil
}

// --- OAuth ---

// AuthURL: Asana's scopes are space separated and a new consent replaces
// the old grant, so the read scopes are always asked for (connecting again
// for a write must not lose the reads).
func (c *asana) AuthURL(state, codeChallenge, redirectURI string, scopes []string) string {
	all := slices.Clone(asanaReadScopes)
	for _, s := range scopes {
		if !slices.Contains(all, s) {
			all = append(all, s)
		}
	}
	v := url.Values{
		"client_id":             {c.sec.ClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {strings.Join(all, " ")},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}
	return c.wire.AuthURL + "?" + v.Encode()
}

func (c *asana) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (Token, error) {
	return c.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {codeVerifier}, "redirect_uri": {redirectURI}}, Token{})
}

// Refresh renews the access token. Asana keeps the refresh token (an
// answer without one keeps the old).
func (c *asana) Refresh(ctx context.Context, t Token) (Token, error) {
	if t.RefreshToken == "" {
		return Token{}, ErrInvalidGrant
	}
	return c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.RefreshToken}}, t)
}

// Revoke revokes the grant at Asana. The endpoint takes the refresh token
// only (it ends the access tokens too); a 400 is a token already gone.
func (c *asana) Revoke(ctx context.Context, t Token) error {
	if t.RefreshToken == "" {
		return nil
	}
	form := url.Values{"client_id": {c.sec.ClientID}, "client_secret": {c.sec.ClientSecret}, "token": {t.RefreshToken}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.wire.RevokeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = ReadBody(resp.Body)
	if (resp.StatusCode >= 200 && resp.StatusCode < 300) || resp.StatusCode == http.StatusBadRequest {
		return nil
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500, Err: errors.New("revoke refused")}
}

func (c *asana) token(ctx context.Context, form url.Values, prev Token) (Token, error) {
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
	}
	if err := json.Unmarshal(body, &r); err != nil || r.AccessToken == "" {
		return Token{}, errors.New("asana: bad token answer")
	}
	// the answer has no scope: the callback keeps the ones asked for
	t := Token{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken, Scopes: prev.Scopes}
	if t.RefreshToken == "" {
		t.RefreshToken = prev.RefreshToken
	}
	if r.ExpiresIn > 0 {
		t.Expiry = c.now().Add(time.Duration(r.ExpiresIn) * time.Second).UTC()
	}
	return t, nil
}

// asanaCanWrite: the granted scopes cover tasks:write (unknown scopes pass;
// Asana then answers 403 and the person connects again).
func asanaCanWrite(t *Token) bool {
	if t == nil || len(t.Scopes) == 0 {
		return true
	}
	return slices.Contains(t.Scopes, asanaScopeTasksWrite)
}

// --- ops ---

const (
	asanaDefaultLimit = 20
	asanaMaxLimit     = 50
	// asanaTaskFields are the only task fields asked for (opt_fields).
	asanaTaskFields = "name,due_on,due_at,completed,permalink_url"
)

// asanaGIDRe is an Asana gid (digits only).
var asanaGIDRe = regexp.MustCompile(`^[0-9]{1,32}$`)

func asanaGID(name, v string, required bool) (string, error) {
	if v == "" && !required {
		return "", nil
	}
	if !asanaGIDRe.MatchString(v) {
		return "", fmt.Errorf("%w: %s must be an Asana id (digits)", ErrUnknown, name)
	}
	return v, nil
}

// asanaLimit reads limit (default 20, 1-50).
func asanaLimit(v string) (int, error) {
	if v == "" {
		return asanaDefaultLimit, nil
	}
	x, err := strconv.Atoi(v)
	if err != nil || x < 1 || x > asanaMaxLimit {
		return 0, fmt.Errorf("%w: limit must be 1-%d", ErrUnknown, asanaMaxLimit)
	}
	return x, nil
}

type asanaTask struct {
	GID          string `json:"gid"`
	Name         string `json:"name"`
	DueOn        string `json:"due_on"`
	DueAt        string `json:"due_at"`
	Completed    bool   `json:"completed"`
	PermalinkURL string `json:"permalink_url"`
}

// row: title = name, value = due (date-time, else date), link = the task
// (only Asana's own https link).
func (t asanaTask) row() Row {
	r := Row{Title: t.Name, Value: t.DueAt}
	if r.Value == "" {
		r.Value = t.DueOn
	}
	if t.Completed {
		r.Subtitle = "完了"
	}
	if strings.HasPrefix(t.PermalinkURL, "https://app.asana.com/") {
		r.URL = t.PermalinkURL
	}
	return r
}

func asanaRows(ts []asanaTask) Output {
	out := Output{Rows: make([]Row, 0, len(ts))}
	for _, t := range ts {
		out.Rows = append(out.Rows, t.row())
	}
	return out
}

func checkAsanaMyTasks(a map[string]string) error {
	if _, err := asanaGID("workspaceId", a["workspaceId"], false); err != nil {
		return err
	}
	_, err := asanaLimit(a["limit"])
	return err
}

func checkAsanaProjectTasks(a map[string]string) error {
	if _, err := asanaGID("projectId", a["projectId"], true); err != nil {
		return err
	}
	_, err := asanaLimit(a["limit"])
	return err
}

func checkAsanaComplete(a map[string]string) error {
	_, err := asanaGID("taskId", a["taskId"], true)
	return err
}

// myTasks lists the open tasks assigned to the person in one workspace, one
// page of limit (default 20). Without workspaceId the person's workspace is
// used; with several, workspaceId is required (400).
func (c *asana) myTasks(ctx context.Context, in Input) (Output, error) {
	ws, err := asanaGID("workspaceId", in.Args["workspaceId"], false)
	if err != nil {
		return Output{}, err
	}
	n, err := asanaLimit(in.Args["limit"])
	if err != nil {
		return Output{}, err
	}
	if ws == "" {
		if ws, err = c.onlyWorkspace(ctx, in); err != nil {
			return Output{}, err
		}
	}
	v := url.Values{"assignee": {"me"}, "workspace": {ws}, "completed_since": {"now"}, "limit": {strconv.Itoa(n)}, "opt_fields": {asanaTaskFields}}
	var r struct {
		Data []asanaTask `json:"data"`
	}
	if err := c.do(ctx, in, http.MethodGet, "/tasks?"+v.Encode(), nil, &r); err != nil {
		return Output{}, err
	}
	return asanaRows(r.Data), nil
}

// onlyWorkspace is the person's workspace when there is exactly one.
func (c *asana) onlyWorkspace(ctx context.Context, in Input) (string, error) {
	var r struct {
		Data []struct {
			GID string `json:"gid"`
		} `json:"data"`
	}
	if err := c.do(ctx, in, http.MethodGet, "/workspaces?"+url.Values{"limit": {"2"}, "opt_fields": {"gid"}}.Encode(), nil, &r); err != nil {
		return "", err
	}
	switch {
	case len(r.Data) == 0:
		return "", fmt.Errorf("%w: the Asana account has no workspace", ErrUnknown)
	case len(r.Data) > 1:
		return "", fmt.Errorf("%w: several Asana workspaces: workspaceId is required", ErrUnknown)
	}
	return asanaGID("workspace", r.Data[0].GID, true)
}

// projectTasks lists the open tasks of one project, one page of limit.
func (c *asana) projectTasks(ctx context.Context, in Input) (Output, error) {
	p, err := asanaGID("projectId", in.Args["projectId"], true)
	if err != nil {
		return Output{}, err
	}
	n, err := asanaLimit(in.Args["limit"])
	if err != nil {
		return Output{}, err
	}
	v := url.Values{"completed_since": {"now"}, "limit": {strconv.Itoa(n)}, "opt_fields": {asanaTaskFields}}
	var r struct {
		Data []asanaTask `json:"data"`
	}
	if err := c.do(ctx, in, http.MethodGet, "/projects/"+p+"/tasks?"+v.Encode(), nil, &r); err != nil {
		return Output{}, err
	}
	return asanaRows(r.Data), nil
}

// complete marks one task completed. Completing it again changes nothing,
// so a retried request is harmless.
func (c *asana) complete(ctx context.Context, in Input) (Output, error) {
	if !asanaCanWrite(in.Token) {
		return Output{}, ErrNotConnected
	}
	id, err := asanaGID("taskId", in.Args["taskId"], true)
	if err != nil {
		return Output{}, err
	}
	b, err := json.Marshal(map[string]any{"data": map[string]any{"completed": true}})
	if err != nil {
		return Output{}, err
	}
	var r struct {
		Data asanaTask `json:"data"`
	}
	if err := c.do(ctx, in, http.MethodPut, "/tasks/"+id+"?"+url.Values{"opt_fields": {asanaTaskFields}}.Encode(), b, &r); err != nil {
		return Output{}, err
	}
	row := r.Data.row()
	return Output{ResultID: id, Item: &row}, nil
}

// do calls the API with the person's token. 401 is a token Asana no longer
// takes (ErrInvalidGrant: the connection turns to error); 403 is a missing
// scope (ErrNotConnected: connect again with it) or, any other 403, an item
// the person has no access to (bad_request: connecting again would not
// help); 400 / 404 are an id the person does not have (bad_request); 429 /
// 5xx are retryable provider errors.
func (c *asana) do(ctx context.Context, in Input, method, path string, body []byte, out any) error {
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
			return fmt.Errorf("asana: bad answer: %w", err)
		}
		return nil
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrInvalidGrant
	case resp.StatusCode == http.StatusForbidden && asanaScopeMissing(b):
		return ErrNotConnected
	case resp.StatusCode == http.StatusForbidden:
		return &ProviderError{Status: resp.StatusCode, Err: fmt.Errorf("%w: asana: this person has no access to this item", ErrUnknown)}
	case resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound:
		return &ProviderError{Status: resp.StatusCode, Err: fmt.Errorf("%w: asana has no such item", ErrUnknown)}
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode == 429 || resp.StatusCode >= 500,
		Err: errors.New("asana refused")}
}

// asanaScopeMissing: a 403 says the token lacks a scope (Asana names the
// missing scopes in the message); the message is not kept.
func asanaScopeMissing(body []byte) bool {
	var e struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(body, &e)
	for _, x := range e.Errors {
		if strings.Contains(strings.ToLower(x.Message), "scope") {
			return true
		}
	}
	return false
}
