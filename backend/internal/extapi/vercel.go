package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Vercel (#1676): free (one `runs` a call), read only, OAuth through a
// Vercel integration (the person installs it on their account or one team).
//
//	projects(limit?)                     the projects and their production deployment
//	deployments(project?, target?, limit?)  the newest deployments
//
// An integration's OAuth has no scope parameter, no PKCE (the token
// exchange takes no code_verifier; the client secret stays on the runner)
// and no refresh token: what it may read is set on the integration
// (Vercel's console: Projects read, Deployments read, Integration
// Configuration read/write for the uninstall below), and the token lasts
// until the integration is removed.
//
// Vercel has no token revoke; removing the integration's installation
// (DELETE /v1/integrations/configuration/{installation_id}) is what ends
// the token. Revoke does that, best effort: the stored token is deleted
// either way, and without the installation id it is ErrNoRevoke.
//
// The team id is optional: null in the token answer is a personal account
// (no ?teamId=). A token installed on a team needs ?teamId=: the team id is
// validated (team_…) and kept with the token as the pseudo-scope
// "team:<id>"; a team id that does not look like one fails the exchange
// rather than silently reading the personal account. The installation id
// is kept the same way ("installation:<id>").
//
// Env: EXT_VERCEL_ENABLED=1, EXT_VERCEL_CLIENT_ID / _CLIENT_SECRET (Secret
// Manager), EXT_VERCEL_INTEGRATION_SLUG (the integration's URL name). Off
// by default; not in the review build (NeedsOAuth).
const Vercel = "vercel"

// vercelWire is where the provider talks to; tests point it at httptest.
type vercelWire struct {
	AuthBase string // https://vercel.com/integrations: opened in the browser
	TokenURL string
	APIBase  string
	Client   *http.Client // nil in production: NewHTTPClient on vercelHosts
}

var vercelWireDefault = vercelWire{
	AuthBase: "https://vercel.com/integrations",
	TokenURL: "https://api.vercel.com/v2/oauth/access_token",
	APIBase:  "https://api.vercel.com",
}

// only the API host: the consent page is opened by the device
var vercelHosts = []string{"https://api.vercel.com"}

const (
	vercelTeamScope         = "team:"
	vercelInstallationScope = "installation:"
)

func init() {
	register(Provider{ID: Vercel, Billing: BillingFree, NeedsOAuth: true, NameJa: "Vercel", NameEn: "Vercel",
		Ops: []OpSpec{
			{Name: "projects", Args: []string{"limit"}, Check: checkVercelProjects},
			{Name: "deployments", Args: []string{"project", "target", "limit"}, Check: checkVercelDeployments},
		},
		Hosts: vercelHosts,
		New: func(sec Secrets) (map[string]Operation, error) {
			c, err := newVercel(sec, os.Getenv("EXT_VERCEL_INTEGRATION_SLUG"), vercelWireDefault)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"projects": OperationFunc(c.projects), "deployments": OperationFunc(c.deployments)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) {
			return newVercel(sec, os.Getenv("EXT_VERCEL_INTEGRATION_SLUG"), vercelWireDefault)
		},
	})
}

type vercel struct {
	sec  Secrets
	slug string
	wire vercelWire
	hc   *http.Client
}

var (
	vercelSlugRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	vercelTeamRe    = regexp.MustCompile(`^team_[A-Za-z0-9]{1,64}$`)
	vercelInstallRe = regexp.MustCompile(`^icfg_[A-Za-z0-9]{1,64}$`)
	vercelProjectRe = regexp.MustCompile(`^(prj_[A-Za-z0-9]{1,64}|[a-z0-9][a-z0-9._-]{0,99})$`)
	vercelHostRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,252}\.vercel\.app$`)
)

func newVercel(sec Secrets, slug string, w vercelWire) (*vercel, error) {
	if sec.ClientID == "" || sec.ClientSecret == "" {
		return nil, errors.New("EXT_VERCEL_CLIENT_ID / _CLIENT_SECRET not set")
	}
	slug = strings.TrimSpace(slug)
	if !vercelSlugRe.MatchString(slug) {
		return nil, errors.New("EXT_VERCEL_INTEGRATION_SLUG not set or not an integration slug")
	}
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(vercelHosts, DefaultTimeout)
	}
	return &vercel{sec: sec, slug: slug, wire: w, hc: hc}, nil
}

// --- OAuth ---

// AuthURL is the integration's install page. Vercel sends the person back
// to the redirect URL set on the integration with code and state.
func (c *vercel) AuthURL(state, _, _ string, _ []string) string {
	return c.wire.AuthBase + "/" + c.slug + "/new?" + url.Values{"state": {state}}.Encode()
}

func (c *vercel) Exchange(ctx context.Context, code, _, redirectURI string) (Token, error) {
	form := url.Values{"client_id": {c.sec.ClientID}, "client_secret": {c.sec.ClientSecret}, "code": {code}, "redirect_uri": {redirectURI}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.wire.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return Token{}, transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := ReadBody(resp.Body)
	if err != nil {
		return Token{}, err
	}
	switch {
	case resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return Token{}, ErrInvalidGrant // a used or wrong code
	case resp.StatusCode != http.StatusOK:
		return Token{}, statusError(resp.StatusCode)
	}
	var r struct {
		AccessToken    string  `json:"access_token"`
		TeamID         *string `json:"team_id"`
		InstallationID string  `json:"installation_id"`
	}
	if err := json.Unmarshal(body, &r); err != nil || r.AccessToken == "" {
		return Token{}, errors.New("vercel: bad token answer")
	}
	t := Token{AccessToken: r.AccessToken}
	if r.TeamID != nil && *r.TeamID != "" {
		if !vercelTeamRe.MatchString(*r.TeamID) {
			return Token{}, errors.New("vercel: bad team id in the token answer")
		}
		t.Scopes = append(t.Scopes, vercelTeamScope+*r.TeamID)
	}
	if vercelInstallRe.MatchString(r.InstallationID) {
		t.Scopes = append(t.Scopes, vercelInstallationScope+r.InstallationID)
	}
	return t, nil
}

// Refresh: integration tokens have no refresh token and no expiry (the
// service only asks when an expiry was given).
func (c *vercel) Refresh(context.Context, Token) (Token, error) { return Token{}, ErrInvalidGrant }

// Revoke removes the integration's installation, which ends the token
// (Vercel has no token revoke). 2xx, and 401 / 404 / 410 (the token or the
// installation is already gone) are done. Without the installation id it
// is ErrNoRevoke: the person removes the integration on Vercel's side.
func (c *vercel) Revoke(ctx context.Context, t Token) error {
	id := vercelScope(&t, vercelInstallationScope, vercelInstallRe)
	if id == "" || t.AccessToken == "" {
		return ErrNoRevoke
	}
	u := c.wire.APIBase + "/v1/integrations/configuration/" + url.PathEscape(id)
	if team := vercelTeam(&t); team != "" {
		u += "?" + url.Values{"teamId": {team}}.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+t.AccessToken)
	resp, err := c.hc.Do(req)
	if err != nil {
		return transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = ReadBody(resp.Body)
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300,
		resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusNotFound, resp.StatusCode == http.StatusGone:
		return nil
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500, Err: errors.New("vercel: uninstall refused")}
}

// vercelTeam is the team the token was installed on ("" = a personal account).
func vercelTeam(t *Token) string { return vercelScope(t, vercelTeamScope, vercelTeamRe) }

// vercelScope reads a pseudo-scope kept with the token.
func vercelScope(t *Token, prefix string, re *regexp.Regexp) string {
	if t == nil {
		return ""
	}
	for _, s := range t.Scopes {
		if id, ok := strings.CutPrefix(s, prefix); ok && re.MatchString(id) {
			return id
		}
	}
	return ""
}

// --- ops ---

type vercelDeployment struct {
	UID          string            `json:"uid"`
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	URL          string            `json:"url"`
	Created      int64             `json:"created"`
	CreatedAt    int64             `json:"createdAt"`
	ReadyState   string            `json:"readyState"`
	State        string            `json:"state"`
	Target       *string           `json:"target"`
	InspectorURL string            `json:"inspectorUrl"`
	Meta         map[string]string `json:"meta"`
}

func vercelState(s string) string {
	switch strings.ToUpper(s) {
	case "READY":
		return deployReady
	case "ERROR":
		return deployError
	case "BUILDING", "INITIALIZING":
		return deployBuilding
	case "QUEUED":
		return deployQueued
	case "CANCELED":
		return deployCanceled
	}
	return ""
}

// row: title = the project, value = the state, subtitle = 本番 / プレビュー ·
// branch · commit message · time, link = Vercel's inspector (or the
// deployment's *.vercel.app).
func (d vercelDeployment) row(title string) Row {
	state := d.ReadyState
	if state == "" {
		state = d.State
	}
	target := "プレビュー"
	if d.Target != nil && *d.Target == "production" {
		target = "本番"
	}
	var branch, msg string
	for _, git := range []string{"github", "gitlab", "bitbucket"} {
		if branch == "" {
			branch = d.Meta[git+"CommitRef"]
		}
		if msg == "" {
			msg = d.Meta[git+"CommitMessage"]
		}
	}
	msg, _, _ = strings.Cut(msg, "\n")
	if r := []rune(msg); len(r) > 60 {
		msg = string(r[:59]) + "…"
	}
	ms := d.Created
	if ms == 0 {
		ms = d.CreatedAt
	}
	var at time.Time
	if ms > 0 {
		at = time.UnixMilli(ms)
	}
	if title == "" {
		title = d.Name
	}
	r := Row{Title: title, Value: vercelState(state), Subtitle: deploySubtitle(target, branch, msg, deployTime(at))}
	switch {
	case strings.HasPrefix(d.InspectorURL, "https://vercel.com/") && !strings.ContainsAny(d.InspectorURL, " \"<>"):
		r.URL = d.InspectorURL
	case vercelHostRe.MatchString(d.URL):
		r.URL = "https://" + d.URL
	}
	return r
}

func vercelLimit(a map[string]string) (int, error) { return argInt(a, "limit", 20, 1, 50) }

func checkVercelProjects(a map[string]string) error {
	_, err := vercelLimit(a)
	return err
}

// vercelDeploymentsQuery reads deployments' arguments.
func vercelDeploymentsQuery(a map[string]string) (url.Values, error) {
	n, err := vercelLimit(a)
	if err != nil {
		return nil, err
	}
	v := url.Values{"limit": {strconv.Itoa(n)}}
	if p := strings.TrimSpace(a["project"]); p != "" {
		if !vercelProjectRe.MatchString(p) {
			return nil, fmt.Errorf("%w: project must be a Vercel project id (prj_…) or name", ErrUnknown)
		}
		v.Set("projectId", p)
	}
	switch t := strings.TrimSpace(a["target"]); t {
	case "":
	case "production", "preview":
		v.Set("target", t)
	default:
		return nil, fmt.Errorf("%w: target must be production or preview", ErrUnknown)
	}
	return v, nil
}

func checkVercelDeployments(a map[string]string) error {
	_, err := vercelDeploymentsQuery(a)
	return err
}

// projects lists the person's projects (newest first) with the state of
// each one's production deployment (the newest deployment when there is no
// production one yet; no value when there is none at all).
func (c *vercel) projects(ctx context.Context, in Input) (Output, error) {
	n, err := vercelLimit(in.Args)
	if err != nil {
		return Output{}, err
	}
	var r struct {
		Projects []struct {
			Name    string                       `json:"name"`
			Targets map[string]*vercelDeployment `json:"targets"`
			Latest  []vercelDeployment           `json:"latestDeployments"`
		} `json:"projects"`
	}
	if err := c.do(ctx, in, "/v9/projects", url.Values{"limit": {strconv.Itoa(n)}}, &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, len(r.Projects))}
	for _, p := range r.Projects {
		var d *vercelDeployment
		if prod := p.Targets["production"]; prod != nil {
			d = prod
			if d.Target == nil {
				prod := "production"
				d.Target = &prod
			}
		} else if len(p.Latest) > 0 {
			d = &p.Latest[0]
		}
		if d == nil {
			out.Rows = append(out.Rows, Row{Title: p.Name})
			continue
		}
		out.Rows = append(out.Rows, d.row(p.Name))
	}
	return out, nil
}

// deployments lists the newest deployments (of one project, of one target).
func (c *vercel) deployments(ctx context.Context, in Input) (Output, error) {
	v, err := vercelDeploymentsQuery(in.Args)
	if err != nil {
		return Output{}, err
	}
	var r struct {
		Deployments []vercelDeployment `json:"deployments"`
	}
	if err := c.do(ctx, in, "/v6/deployments", v, &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, len(r.Deployments))}
	for _, d := range r.Deployments {
		out.Rows = append(out.Rows, d.row(""))
	}
	return out, nil
}

// do GETs the API with the person's token (and their team). 401 is a token
// Vercel no longer takes (the integration was removed: the connection
// turns to error); 403 is a project or team the integration may not read
// (connect again and pick it); 404 is a project that does not exist
// (bad_request); 429 / 5xx are retryable.
func (c *vercel) do(ctx context.Context, in Input, path string, q url.Values, out any) error {
	if in.Token == nil || in.Token.AccessToken == "" {
		return ErrNotConnected
	}
	if team := vercelTeam(in.Token); team != "" {
		q.Set("teamId", team)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.wire.APIBase+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+in.Token.AccessToken)
	req.Header.Set("Accept", "application/json")
	err = sendJSON(ctx, c.hc, req, out)
	var pe *ProviderError
	if errors.As(err, &pe) {
		switch pe.Status {
		case http.StatusUnauthorized:
			return ErrInvalidGrant
		case http.StatusForbidden:
			return ErrNotConnected
		case http.StatusNotFound:
			return &ProviderError{Status: pe.Status, Err: fmt.Errorf("%w: vercel has no such project", ErrUnknown)}
		}
	}
	return err
}
