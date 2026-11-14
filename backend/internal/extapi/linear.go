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

// Linear (#1684, GraphQL): free (one `runs` a call), OAuth 2.0 with PKCE.
//
//	myIssues(state?, limit?)               read
//	createIssue(title, description?, team?)  issues:create (+ read)
//
// read is Linear's default scope; issues:create can create issues and
// nothing else (no edit, no delete; `write` is not asked for). Access
// tokens last 24 hours with a rotated refresh token (30 days).
//
// Linear's limits are per user: 5,000 requests and 2,000,000 complexity
// points an hour (a query over 10,000 points is refused); a rate-limited
// answer is 429 with RATELIMITED. Queries here ask for the fields shown
// and at most 50 nodes.
//
// Env: EXT_LINEAR_ENABLED=1, EXT_LINEAR_CLIENT_ID, EXT_LINEAR_CLIENT_SECRET
// (Secret Manager). Off by default; not in the review build (NeedsOAuth).
const Linear = "linear"

const (
	linearScopeRead   = "read"
	linearScopeCreate = "issues:create"
	linearScopeWrite  = "write" // covers issues:create; never asked for
)

// linearWire is where the provider talks to; tests point it at httptest.
type linearWire struct {
	AuthURL   string // opened in the browser, not reached by the runner
	TokenURL  string
	RevokeURL string
	GraphQL   string
	// Client is nil in production: NewHTTPClient on the provider's Hosts.
	Client *http.Client
	// Now is nil in production (time.Now).
	Now func() time.Time
}

var linearWireDefault = linearWire{
	AuthURL:   "https://linear.app/oauth/authorize",
	TokenURL:  "https://api.linear.app/oauth/token",
	RevokeURL: "https://api.linear.app/oauth/revoke",
	GraphQL:   "https://api.linear.app/graphql",
}

var linearHosts = []string{"https://api.linear.app"}

func init() {
	register(Provider{ID: Linear, Billing: BillingFree, NeedsOAuth: true, NameJa: "Linear", NameEn: "Linear",
		Ops: []OpSpec{
			{Name: "myIssues", Scopes: []string{linearScopeRead}, Args: []string{"state", "limit"}, Check: checkLinearMyIssues},
			{Name: "createIssue", Write: true, Scopes: []string{linearScopeCreate}, Args: []string{"title", "description", "team"}, Check: checkLinearCreate},
		},
		Hosts: linearHosts,
		New: func(sec Secrets) (map[string]Operation, error) {
			c, err := newLinear(sec, linearWireDefault)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"myIssues": OperationFunc(c.myIssues), "createIssue": OperationFunc(c.createIssue)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) { return newLinear(sec, linearWireDefault) },
	})
}

// linear is the provider's ops and OAuth client.
type linear struct {
	sec  Secrets
	wire linearWire
	hc   *http.Client
	now  func() time.Time
}

func newLinear(sec Secrets, w linearWire) (*linear, error) {
	if sec.ClientID == "" || sec.ClientSecret == "" {
		return nil, errors.New("EXT_LINEAR_CLIENT_ID / _CLIENT_SECRET not set")
	}
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(linearHosts, DefaultTimeout)
	}
	now := w.Now
	if now == nil {
		now = time.Now
	}
	return &linear{sec: sec, wire: w, hc: hc, now: now}, nil
}

// --- OAuth ---

// AuthURL: Linear's scopes are comma separated, and read is always asked
// for (connecting again for issues:create must not lose the reads, and
// createIssue reads the person's teams).
func (c *linear) AuthURL(state, codeChallenge, redirectURI string, scopes []string) string {
	if !slices.Contains(scopes, linearScopeRead) {
		scopes = append([]string{linearScopeRead}, scopes...)
	}
	v := url.Values{
		"client_id":             {c.sec.ClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {strings.Join(scopes, ",")},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
		"actor":                 {"user"},
	}
	return c.wire.AuthURL + "?" + v.Encode()
}

func (c *linear) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (Token, error) {
	return c.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {codeVerifier}, "redirect_uri": {redirectURI}}, Token{})
}

// Refresh exchanges the refresh token (Linear rotates it).
func (c *linear) Refresh(ctx context.Context, t Token) (Token, error) {
	if t.RefreshToken == "" {
		return Token{}, ErrInvalidGrant
	}
	return c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.RefreshToken}}, t)
}

// Revoke revokes the grant at Linear: the refresh token when there is one
// (it outlives the access token), else the access token. 400 (already
// revoked) and 401 (a token Linear no longer knows) are done.
func (c *linear) Revoke(ctx context.Context, t Token) error {
	form := url.Values{"token": {t.AccessToken}, "token_type_hint": {"access_token"}}
	if t.RefreshToken != "" {
		form = url.Values{"token": {t.RefreshToken}, "token_type_hint": {"refresh_token"}}
	}
	if form.Get("token") == "" {
		return nil
	}
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
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300, resp.StatusCode == http.StatusBadRequest, resp.StatusCode == http.StatusUnauthorized:
		return nil
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500, Err: errors.New("revoke refused")}
}

func (c *linear) token(ctx context.Context, form url.Values, prev Token) (Token, error) {
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
		AccessToken  string          `json:"access_token"`
		RefreshToken string          `json:"refresh_token"`
		ExpiresIn    int64           `json:"expires_in"`
		Scope        json.RawMessage `json:"scope"`
	}
	if err := json.Unmarshal(body, &r); err != nil || r.AccessToken == "" {
		return Token{}, errors.New("linear: bad token answer")
	}
	t := Token{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken, Scopes: linearScopes(r.Scope)}
	if r.ExpiresIn > 0 && r.RefreshToken != "" {
		t.Expiry = c.now().Add(time.Duration(r.ExpiresIn) * time.Second).UTC()
	}
	if len(t.Scopes) == 0 {
		t.Scopes = prev.Scopes
	}
	return t, nil
}

// linearScopes reads scope: "read write" / "read,write" or (apps made
// before December 2023) an array.
func linearScopes(raw json.RawMessage) []string {
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	var s string
	_ = json.Unmarshal(raw, &s)
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
}

// linearCanCreate: the granted scopes cover issues:create (unknown scopes
// pass; Linear then answers FORBIDDEN and the person connects again).
func linearCanCreate(t *Token) bool {
	if t == nil || len(t.Scopes) == 0 {
		return true
	}
	return slices.Contains(t.Scopes, linearScopeCreate) || slices.Contains(t.Scopes, linearScopeWrite)
}

// --- ops ---

const (
	linearDefaultLimit = 20
	linearMaxLimit     = 50
	linearMaxTitle     = 255
	linearMaxDesc      = 10000
)

var (
	linearTeamKeyRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,6}$`)
	linearUUIDRe    = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// linearStates are myIssues' state values: a workflow state type, open
// (every type but completed / canceled) or all.
var linearStates = []string{"open", "all", "triage", "backlog", "unstarted", "started", "completed", "canceled"}

// linearFilter is myIssues' IssueFilter (nil = all).
func linearFilter(state string) (map[string]any, error) {
	switch state {
	case "", "open":
		return map[string]any{"state": map[string]any{"type": map[string]any{"nin": []string{"completed", "canceled"}}}}, nil
	case "all":
		return nil, nil
	}
	if !slices.Contains(linearStates, state) {
		return nil, fmt.Errorf("%w: state must be one of %s", ErrBadArgs, strings.Join(linearStates, ", "))
	}
	return map[string]any{"state": map[string]any{"type": map[string]any{"eq": state}}}, nil
}

func linearLimit(v string) (int, error) {
	if v == "" {
		return linearDefaultLimit, nil
	}
	x, err := strconv.Atoi(v)
	if err != nil || x < 1 || x > linearMaxLimit {
		return 0, fmt.Errorf("%w: limit must be 1-%d", ErrBadArgs, linearMaxLimit)
	}
	return x, nil
}

func checkLinearMyIssues(a map[string]string) error {
	if _, err := linearFilter(a["state"]); err != nil {
		return err
	}
	_, err := linearLimit(a["limit"])
	return err
}

const linearMyIssuesQuery = `query($first: Int!, $filter: IssueFilter) {
  viewer { assignedIssues(first: $first, filter: $filter, orderBy: updatedAt) {
    nodes { identifier title url dueDate state { name } }
  } }
}`

type linearIssue struct {
	ID         string `json:"id"`
	Identifier string `json:"identifier"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	DueDate    string `json:"dueDate"`
	State      *struct {
		Name string `json:"name"`
	} `json:"state"`
}

func (i linearIssue) row() Row {
	r := Row{Title: i.Title, Subtitle: i.Identifier, Value: i.DueDate}
	if i.State != nil && i.State.Name != "" {
		r.Subtitle = strings.TrimPrefix(r.Subtitle+" · "+i.State.Name, " · ")
	}
	if strings.HasPrefix(i.URL, "https://linear.app/") {
		r.URL = i.URL
	}
	return r
}

// myIssues lists the issues assigned to the person, newest update first:
// title, "ENG-123 · state", due date, link.
func (c *linear) myIssues(ctx context.Context, in Input) (Output, error) {
	filter, err := linearFilter(in.Args["state"])
	if err != nil {
		return Output{}, err
	}
	n, err := linearLimit(in.Args["limit"])
	if err != nil {
		return Output{}, err
	}
	vars := map[string]any{"first": n}
	if filter != nil {
		vars["filter"] = filter
	}
	var r struct {
		Viewer struct {
			AssignedIssues struct {
				Nodes []linearIssue `json:"nodes"`
			} `json:"assignedIssues"`
		} `json:"viewer"`
	}
	if err := c.graphql(ctx, in, linearMyIssuesQuery, vars, &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, len(r.Viewer.AssignedIssues.Nodes))}
	for _, i := range r.Viewer.AssignedIssues.Nodes {
		out.Rows = append(out.Rows, i.row())
	}
	return out, nil
}

type linearCreateArgs struct {
	title, desc, team string
}

func linearCreateParse(a map[string]string) (linearCreateArgs, error) {
	title, err := todoistText("title", a["title"], linearMaxTitle, true)
	if err != nil {
		return linearCreateArgs{}, err
	}
	if strings.ContainsAny(title, "\n\t") {
		return linearCreateArgs{}, fmt.Errorf("%w: title must be one line", ErrBadArgs)
	}
	desc, err := todoistText("description", a["description"], linearMaxDesc, false)
	if err != nil {
		return linearCreateArgs{}, err
	}
	team := strings.TrimSpace(a["team"])
	if team != "" && !linearTeamKeyRe.MatchString(team) && !linearUUIDRe.MatchString(team) {
		return linearCreateArgs{}, fmt.Errorf("%w: team must be a team key (ENG) or id", ErrBadArgs)
	}
	return linearCreateArgs{title: title, desc: desc, team: team}, nil
}

func checkLinearCreate(a map[string]string) error {
	_, err := linearCreateParse(a)
	return err
}

const (
	linearMyTeamsQuery   = `query { viewer { teams(first: 2) { nodes { id } } } }`
	linearTeamByKey      = `query($key: String!) { teams(first: 1, filter: { key: { eq: $key } }) { nodes { id } } }`
	linearCreateMutation = `mutation($input: IssueCreateInput!) {
  issueCreate(input: $input) { success issue { id identifier title url dueDate state { name } } }
}`
)

// teamID resolves team: an id as is, a key (ENG) by Linear's teams, none
// the person's only team (more than one: say which).
func (c *linear) teamID(ctx context.Context, in Input, team string) (string, error) {
	if linearUUIDRe.MatchString(team) {
		return strings.ToLower(team), nil
	}
	var nodes []struct {
		ID string `json:"id"`
	}
	if team == "" {
		var r struct {
			Viewer struct {
				Teams struct {
					Nodes []struct {
						ID string `json:"id"`
					} `json:"nodes"`
				} `json:"teams"`
			} `json:"viewer"`
		}
		if err := c.graphql(ctx, in, linearMyTeamsQuery, nil, &r); err != nil {
			return "", err
		}
		nodes = r.Viewer.Teams.Nodes
		if len(nodes) > 1 {
			return "", fmt.Errorf("%w: team is required (the person is in more than one team)", ErrUnknown)
		}
	} else {
		var r struct {
			Teams struct {
				Nodes []struct {
					ID string `json:"id"`
				} `json:"nodes"`
			} `json:"teams"`
		}
		if err := c.graphql(ctx, in, linearTeamByKey, map[string]any{"key": strings.ToUpper(team)}, &r); err != nil {
			return "", err
		}
		nodes = r.Teams.Nodes
	}
	if len(nodes) == 0 || nodes[0].ID == "" {
		return "", fmt.Errorf("%w: no such team", ErrUnknown)
	}
	return nodes[0].ID, nil
}

// createIssue adds one issue to team (its default state: Backlog or
// Triage). The answer is the issue's identifier (ENG-123) and a row.
func (c *linear) createIssue(ctx context.Context, in Input) (Output, error) {
	if !linearCanCreate(in.Token) {
		return Output{}, ErrNotConnected
	}
	a, err := linearCreateParse(in.Args)
	if err != nil {
		return Output{}, err
	}
	teamID, err := c.teamID(ctx, in, a.team)
	if err != nil {
		return Output{}, err
	}
	input := map[string]any{"teamId": teamID, "title": a.title}
	if a.desc != "" {
		input["description"] = a.desc
	}
	var r struct {
		IssueCreate struct {
			Success bool         `json:"success"`
			Issue   *linearIssue `json:"issue"`
		} `json:"issueCreate"`
	}
	if err := c.graphql(ctx, in, linearCreateMutation, map[string]any{"input": input}, &r); err != nil {
		return Output{}, err
	}
	if !r.IssueCreate.Success || r.IssueCreate.Issue == nil {
		return Output{}, &ProviderError{Status: http.StatusBadGateway, Err: errors.New("linear: issueCreate did not succeed")}
	}
	row := r.IssueCreate.Issue.row()
	return Output{ResultID: r.IssueCreate.Issue.Identifier, Item: &row}, nil
}

// graphql posts a query with the person's token. 401 or
// AUTHENTICATION_ERROR is a token Linear no longer takes (ErrInvalidGrant:
// the connection turns to error); 403 or FORBIDDEN is a missing scope
// (ErrNotConnected: connect again); 429 or RATELIMITED is a retryable 429
// that pauses the provider; an input error is a bad request; 5xx are
// retryable. Linear may answer errors with 200 or 400.
func (c *linear) graphql(ctx context.Context, in Input, query string, vars map[string]any, out any) error {
	if in.Token == nil || in.Token.AccessToken == "" {
		return ErrNotConnected
	}
	body := map[string]any{"query": query}
	if vars != nil {
		body["variables"] = vars
	}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.wire.GraphQL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+in.Token.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	rb, err := ReadBody(resp.Body)
	if err != nil {
		return err
	}
	var r struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message    string `json:"message"`
			Extensions struct {
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(rb, &r)
	var code string
	if len(r.Errors) > 0 {
		code = r.Errors[0].Extensions.Code
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized, code == "AUTHENTICATION_ERROR":
		return ErrInvalidGrant
	case resp.StatusCode == http.StatusTooManyRequests, code == "RATELIMITED":
		return &ProviderError{Status: http.StatusTooManyRequests, Retryable: true, Err: errors.New("linear rate limited")}
	case resp.StatusCode == http.StatusForbidden, code == "FORBIDDEN":
		return ErrNotConnected
	case resp.StatusCode >= 500:
		return &ProviderError{Status: resp.StatusCode, Retryable: true, Err: fmt.Errorf("linear status %d", resp.StatusCode)}
	case code == "INVALID_INPUT" || code == "BAD_USER_INPUT" || code == "ENTITY_NOT_FOUND":
		return &ProviderError{Status: http.StatusBadRequest, Err: fmt.Errorf("%w: linear refused the input", ErrUnknown)}
	case len(r.Errors) > 0 || resp.StatusCode != http.StatusOK:
		return &ProviderError{Status: http.StatusBadGateway, Err: fmt.Errorf("linear status %d %s", resp.StatusCode, code)}
	}
	if err := json.Unmarshal(r.Data, out); err != nil {
		return fmt.Errorf("linear: bad answer: %w", err)
	}
	return nil
}
