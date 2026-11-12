package extapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// GitHub (#1674): free (one `runs` a call), a GitHub App's user access
// token. Read only:
//
//	contributions(from?, to?)            GraphQL contributionsCollection
//	pullRequests(role?, state?, limit?)  search: review-requested / author
//	issues(state?, limit?)               search: assigned to me
//
// A GitHub App has no OAuth scopes: what a token reaches is the app's
// fine-grained permissions (Issues: read, Pull requests: read, Metadata:
// read) and only the repositories the app is installed on, so a private
// repository is read only where the person installed it. The app is
// registered with expiring user tokens (8 hours, a rotated refresh token
// for 6 months); an app that opted out gets a token without an expiry.
//
// GitHub asks for a User-Agent, at most 100 requests at once and no
// concurrent requests for one user: calls of one token wait for each other
// here. Search is 30 a minute a user; a rate-limited 403 / 429 pauses the
// provider (limits.go).
//
// Env: EXT_GITHUB_ENABLED=1, EXT_GITHUB_CLIENT_ID, EXT_GITHUB_CLIENT_SECRET
// (Secret Manager). Off by default; not in the review build (NeedsOAuth).
const GitHub = "github"

// githubAPIVersion is the X-GitHub-Api-Version of every REST call.
const githubAPIVersion = "2026-03-10"

const githubUserAgent = "tailorit-runner"

// githubWire is where the provider talks to; tests point it at httptest.
type githubWire struct {
	AuthURL  string // opened in the browser, not reached by the runner
	TokenURL string
	APIBase  string // REST; GraphQL is APIBase + /graphql
	// Client is nil in production: NewHTTPClient on the provider's Hosts.
	Client *http.Client
	// Now is nil in production (time.Now).
	Now func() time.Time
}

var githubWireDefault = githubWire{
	AuthURL:  "https://github.com/login/oauth/authorize",
	TokenURL: "https://github.com/login/oauth/access_token",
	APIBase:  "https://api.github.com",
}

// github.com is the token endpoint; api.github.com the API and the revoke.
var githubHosts = []string{"https://github.com", "https://api.github.com"}

func init() {
	register(Provider{ID: GitHub, Billing: BillingFree, NeedsOAuth: true, NameJa: "GitHub", NameEn: "GitHub",
		Ops: []OpSpec{
			{Name: "contributions", Args: []string{"from", "to"}, Check: checkGitHubContributions},
			{Name: "pullRequests", Args: []string{"role", "state", "limit"}, Check: checkGitHubPulls},
			{Name: "issues", Args: []string{"state", "limit"}, Check: checkGitHubIssues},
		},
		Hosts: githubHosts,
		New: func(sec Secrets) (map[string]Operation, error) {
			c, err := newGitHub(sec, githubWireDefault)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"contributions": OperationFunc(c.contributions),
				"pullRequests": OperationFunc(c.pullRequests), "issues": OperationFunc(c.issues)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) { return newGitHub(sec, githubWireDefault) },
	})
}

// github is the provider's ops and OAuth client.
type github struct {
	sec  Secrets
	wire githubWire
	hc   *http.Client
	now  func() time.Time
	per  keyedLock // one call at a time per token (GitHub's advice)
}

func newGitHub(sec Secrets, w githubWire) (*github, error) {
	if sec.ClientID == "" || sec.ClientSecret == "" {
		return nil, errors.New("EXT_GITHUB_CLIENT_ID / _CLIENT_SECRET not set")
	}
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(githubHosts, DefaultTimeout)
	}
	now := w.Now
	if now == nil {
		now = time.Now
	}
	return &github{sec: sec, wire: w, hc: hc, now: now}, nil
}

// --- OAuth ---

// AuthURL: a GitHub App takes no scopes (its permissions are fixed when it
// is registered); PKCE is S256.
func (c *github) AuthURL(state, codeChallenge, redirectURI string, _ []string) string {
	v := url.Values{
		"client_id":             {c.sec.ClientID},
		"redirect_uri":          {redirectURI},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
		"allow_signup":          {"false"},
	}
	return c.wire.AuthURL + "?" + v.Encode()
}

func (c *github) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (Token, error) {
	return c.token(ctx, url.Values{"code": {code}, "code_verifier": {codeVerifier}, "redirect_uri": {redirectURI}})
}

// Refresh exchanges the refresh token (rotated: the old one and its access
// token stop working). A token without one (expiring tokens off) cannot be
// renewed: connect again.
func (c *github) Refresh(ctx context.Context, t Token) (Token, error) {
	if t.RefreshToken == "" {
		return Token{}, ErrInvalidGrant
	}
	return c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.RefreshToken}})
}

// Revoke deletes the person's authorization of the app (DELETE
// /applications/{client_id}/grant, client Basic auth): every token of it
// stops working. It takes an access token, so an expired one is renewed
// first; one GitHub no longer knows (422) is done.
func (c *github) Revoke(ctx context.Context, t Token) error {
	if t.expired(c.now()) && t.RefreshToken != "" {
		nt, err := c.Refresh(ctx, t)
		if err != nil {
			// ErrInvalidGrant: gone at GitHub, or renewed elsewhere
			// (Connections.Revoke reads it again)
			return err
		}
		t = nt
	}
	if t.AccessToken == "" {
		return nil
	}
	b, err := json.Marshal(map[string]string{"access_token": t.AccessToken})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.wire.APIBase+"/applications/"+url.PathEscape(c.sec.ClientID)+"/grant", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.sec.ClientID, c.sec.ClientSecret)
	c.headers(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = ReadBody(resp.Body)
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300, resp.StatusCode == http.StatusNotFound, resp.StatusCode == http.StatusUnprocessableEntity:
		return nil
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500, Err: errors.New("revoke refused")}
}

// token posts to GitHub's token endpoint. It answers a refused code or
// refresh token with 200 and {"error"}.
func (c *github) token(ctx context.Context, form url.Values) (Token, error) {
	form.Set("client_id", c.sec.ClientID)
	form.Set("client_secret", c.sec.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.wire.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", githubUserAgent)
	resp, err := c.hc.Do(req)
	if err != nil {
		return Token{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := ReadBody(resp.Body)
	if err != nil {
		return Token{}, err
	}
	var r struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Error        string `json:"error"`
	}
	_ = json.Unmarshal(body, &r)
	switch r.Error {
	case "":
	case "bad_verification_code", "bad_refresh_token", "invalid_grant":
		return Token{}, ErrInvalidGrant
	default:
		return Token{}, &ProviderError{Status: resp.StatusCode, Err: fmt.Errorf("token endpoint refused: %s", r.Error)}
	}
	if resp.StatusCode != http.StatusOK {
		return Token{}, &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500 || resp.StatusCode == 429, Err: errors.New("token endpoint refused")}
	}
	if r.AccessToken == "" {
		return Token{}, errors.New("github: bad token answer")
	}
	t := Token{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken}
	if r.ExpiresIn > 0 && r.RefreshToken != "" {
		t.Expiry = c.now().Add(time.Duration(r.ExpiresIn) * time.Second).UTC()
	}
	return t, nil
}

func (c *github) headers(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	req.Header.Set("User-Agent", githubUserAgent)
}

// --- ops ---

const (
	githubDefaultLimit = 20
	githubMaxLimit     = 50
	githubDefaultDays  = 7
	githubMaxDays      = 366 // GitHub's contributionsCollection: at most a year
)

// githubZone is the day the contributions' dates are counted in.
var githubZone = time.FixedZone("JST", 9*60*60)

// githubDates reads from / to (YYYY-MM-DD, JST): default the last 7 days
// to today; to is not after today; at most 366 days.
func githubDates(a map[string]string, now time.Time) (time.Time, time.Time, error) {
	today := now.In(githubZone)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, githubZone)
	parse := func(name, v string) (time.Time, error) {
		d, err := time.ParseInLocation(time.DateOnly, v, githubZone)
		if err != nil {
			return time.Time{}, fmt.Errorf("%w: %s must be YYYY-MM-DD", ErrBadArgs, name)
		}
		return d, nil
	}
	to := today
	if v := a["to"]; v != "" {
		d, err := parse("to", v)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		to = d
	}
	from := to.AddDate(0, 0, -(githubDefaultDays - 1))
	if v := a["from"]; v != "" {
		d, err := parse("from", v)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		from = d
	}
	switch {
	case to.After(today):
		return time.Time{}, time.Time{}, fmt.Errorf("%w: to is after today", ErrBadArgs)
	case from.After(to):
		return time.Time{}, time.Time{}, fmt.Errorf("%w: from is after to", ErrBadArgs)
	case from.Year() < 2008:
		return time.Time{}, time.Time{}, fmt.Errorf("%w: from is before GitHub", ErrBadArgs)
	case to.Sub(from) >= githubMaxDays*24*time.Hour:
		return time.Time{}, time.Time{}, fmt.Errorf("%w: from - to is over %d days", ErrBadArgs, githubMaxDays)
	}
	return from, to, nil
}

func checkGitHubContributions(a map[string]string) error {
	_, _, err := githubDates(a, time.Now())
	return err
}

const githubContribQuery = `query($from: DateTime!, $to: DateTime!) {
  viewer { contributionsCollection(from: $from, to: $to) {
    contributionCalendar { totalContributions weeks { contributionDays { date contributionCount } } }
  } }
}`

// contributions is the person's contribution calendar (the 「草」) from
// from to to: one row a day, oldest first (title = date, value = count),
// and the total as the item. Private contributions count only as the
// person's profile shows them.
func (c *github) contributions(ctx context.Context, in Input) (Output, error) {
	from, to, err := githubDates(in.Args, c.now())
	if err != nil {
		return Output{}, err
	}
	vars := map[string]any{"from": from.Format(time.RFC3339), "to": to.Add(24*time.Hour - time.Second).Format(time.RFC3339)}
	var r struct {
		Viewer struct {
			ContributionsCollection struct {
				ContributionCalendar struct {
					TotalContributions int `json:"totalContributions"`
					Weeks              []struct {
						ContributionDays []struct {
							Date              string `json:"date"`
							ContributionCount int    `json:"contributionCount"`
						} `json:"contributionDays"`
					} `json:"weeks"`
				} `json:"contributionCalendar"`
			} `json:"contributionsCollection"`
		} `json:"viewer"`
	}
	if err := c.graphql(ctx, in, githubContribQuery, vars, &r); err != nil {
		return Output{}, err
	}
	cal := r.Viewer.ContributionsCollection.ContributionCalendar
	lo, hi := from.Format(time.DateOnly), to.Format(time.DateOnly)
	out := Output{Rows: []Row{}}
	for _, w := range cal.Weeks {
		for _, d := range w.ContributionDays {
			if d.Date < lo || d.Date > hi {
				continue
			}
			out.Rows = append(out.Rows, Row{Title: d.Date, Value: strconv.Itoa(d.ContributionCount)})
		}
	}
	out.Item = &Row{Title: lo + "〜" + hi, Value: strconv.Itoa(cal.TotalContributions)}
	return out, nil
}

// githubLimit reads limit (default 20, 1-50).
func githubLimit(v string) (int, error) {
	if v == "" {
		return githubDefaultLimit, nil
	}
	x, err := strconv.Atoi(v)
	if err != nil || x < 1 || x > githubMaxLimit {
		return 0, fmt.Errorf("%w: limit must be 1-%d", ErrBadArgs, githubMaxLimit)
	}
	return x, nil
}

// githubState is the search qualifier of state (open by default).
func githubState(v string, pr bool) (string, error) {
	switch v {
	case "", "open":
		return "is:open", nil
	case "closed":
		return "is:closed", nil
	case "merged":
		if pr {
			return "is:merged", nil
		}
	case "all":
		return "", nil
	}
	if pr {
		return "", fmt.Errorf("%w: state must be open, closed, merged or all", ErrBadArgs)
	}
	return "", fmt.Errorf("%w: state must be open, closed or all", ErrBadArgs)
}

// githubPullsQuery is pullRequests' search query: role reviewRequested
// (default: the PRs waiting for the person's review) or author.
func githubPullsQuery(a map[string]string) (string, int, error) {
	var role string
	switch a["role"] {
	case "", "reviewRequested":
		role = "review-requested:@me"
	case "author":
		role = "author:@me"
	default:
		return "", 0, fmt.Errorf("%w: role must be reviewRequested or author", ErrBadArgs)
	}
	state, err := githubState(a["state"], true)
	if err != nil {
		return "", 0, err
	}
	n, err := githubLimit(a["limit"])
	if err != nil {
		return "", 0, err
	}
	return strings.Join(githubNonEmpty("is:pr", role, state, "archived:false"), " "), n, nil
}

func githubIssuesQuery(a map[string]string) (string, int, error) {
	state, err := githubState(a["state"], false)
	if err != nil {
		return "", 0, err
	}
	n, err := githubLimit(a["limit"])
	if err != nil {
		return "", 0, err
	}
	return strings.Join(githubNonEmpty("is:issue", "assignee:@me", state, "archived:false"), " "), n, nil
}

func githubNonEmpty(s ...string) []string {
	out := s[:0]
	for _, v := range s {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func checkGitHubPulls(a map[string]string) error {
	_, _, err := githubPullsQuery(a)
	return err
}

func checkGitHubIssues(a map[string]string) error {
	_, _, err := githubIssuesQuery(a)
	return err
}

// pullRequests lists PRs waiting for the person's review (role
// reviewRequested) or theirs (author), newest update first.
func (c *github) pullRequests(ctx context.Context, in Input) (Output, error) {
	q, n, err := githubPullsQuery(in.Args)
	if err != nil {
		return Output{}, err
	}
	return c.search(ctx, in, q, n)
}

// issues lists the issues assigned to the person, newest update first.
func (c *github) issues(ctx context.Context, in Input) (Output, error) {
	q, n, err := githubIssuesQuery(in.Args)
	if err != nil {
		return Output{}, err
	}
	return c.search(ctx, in, q, n)
}

// search runs GET /search/issues: rows are title / "owner/repo#n" /
// updated / link, the item is the total count (up to GitHub's 1,000).
func (c *github) search(ctx context.Context, in Input, q string, n int) (Output, error) {
	v := url.Values{"q": {q}, "sort": {"updated"}, "order": {"desc"}, "per_page": {strconv.Itoa(n)}}
	var r struct {
		TotalCount int `json:"total_count"`
		Items      []struct {
			Title         string `json:"title"`
			Number        int    `json:"number"`
			HTMLURL       string `json:"html_url"`
			RepositoryURL string `json:"repository_url"`
			UpdatedAt     string `json:"updated_at"`
		} `json:"items"`
	}
	if err := c.rest(ctx, in, "/search/issues?"+v.Encode(), &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, len(r.Items)), Item: &Row{Title: q, Value: strconv.Itoa(r.TotalCount)}}
	for _, it := range r.Items {
		row := Row{Title: it.Title, Value: it.UpdatedAt}
		if repo, ok := strings.CutPrefix(it.RepositoryURL, c.wire.APIBase+"/repos/"); ok {
			row.Subtitle = repo + "#" + strconv.Itoa(it.Number)
		}
		if strings.HasPrefix(it.HTMLURL, "https://github.com/") {
			row.URL = it.HTMLURL
		}
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

// --- the wire ---

func (c *github) rest(ctx context.Context, in Input, path string, out any) error {
	return c.do(ctx, in, http.MethodGet, c.wire.APIBase+path, nil, out)
}

// graphql posts a query and reads data into out; GraphQL's errors (200)
// become the same errors as the REST statuses.
func (c *github) graphql(ctx context.Context, in Input, query string, vars map[string]any, out any) error {
	b, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return err
	}
	var r struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := c.do(ctx, in, http.MethodPost, c.wire.APIBase+"/graphql", b, &r); err != nil {
		return err
	}
	if len(r.Errors) > 0 {
		switch r.Errors[0].Type {
		case "RATE_LIMITED":
			return &ProviderError{Status: http.StatusTooManyRequests, Retryable: true, Err: errors.New("github graphql rate limited")}
		case "FORBIDDEN", "INSUFFICIENT_SCOPES":
			return ErrNotConnected
		}
		return &ProviderError{Status: http.StatusBadGateway, Err: fmt.Errorf("github graphql %s", r.Errors[0].Type)}
	}
	if err := json.Unmarshal(r.Data, out); err != nil {
		return fmt.Errorf("github: bad answer: %w", err)
	}
	return nil
}

// do calls the API with the person's token, one call at a time per token.
// 401 is a token GitHub no longer takes (ErrInvalidGrant: the connection
// turns to error); 403 is the app's missing permission (ErrNotConnected:
// connect again) unless it is a rate limit (x-ratelimit-remaining 0 or
// retry-after), which like 429 is a retryable 429 that pauses the
// provider; 404 / 422 are a bad request; 5xx are retryable.
func (c *github) do(ctx context.Context, in Input, method, u string, body []byte, out any) error {
	if in.Token == nil || in.Token.AccessToken == "" {
		return ErrNotConnected
	}
	sum := sha256.Sum256([]byte(in.Token.AccessToken))
	unlock, err := c.per.lock(ctx, hex.EncodeToString(sum[:8]))
	if err != nil {
		return err
	}
	defer unlock()
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.headers(req)
	req.Header.Set("Authorization", "Bearer "+in.Token.AccessToken)
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
	limited := resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.Header.Get("Retry-After") != ""
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("github: bad answer: %w", err)
		}
		return nil
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrInvalidGrant
	case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode == http.StatusForbidden && limited:
		return &ProviderError{Status: http.StatusTooManyRequests, Retryable: true, Err: errors.New("github rate limited")}
	case resp.StatusCode == http.StatusForbidden:
		return ErrNotConnected
	case resp.StatusCode == http.StatusNotFound, resp.StatusCode == http.StatusUnprocessableEntity:
		return &ProviderError{Status: resp.StatusCode, Err: fmt.Errorf("%w: github refused the query", ErrUnknown)}
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500, Err: fmt.Errorf("github status %d", resp.StatusCode)}
}
