package extapi

import (
	"context"
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

// Cloudflare (#1676): free (one `runs` a call), read only, the person's own
// API token (Cloudflare has no OAuth for other apps).
//
//	pagesProjects(accountId?, limit?)                 Pages projects and their newest deployment
//	pagesDeployments(project, accountId?, env?, limit?)  a Pages project's deployments
//	workerDeployments(script, accountId?, limit?)     a Worker's deployments
//
// The token is the person's: it is kept the way #1671 (a person's API key,
// sealed like an OAuth token) decides, and reaches the ops as Input.Token.
// Until #1671 lands this provider is defined but NOT registered
// (cloudflareProvider): nothing can call it. Recommend a token with only
// Account → Cloudflare Pages: Read and Workers Scripts: Read (and Account
// Settings: Read for leaving accountId out).
//
// Env (once registered): EXT_CLOUDFLARE_ENABLED=1. Off by default.
const Cloudflare = "cloudflare"

type cloudflareWire struct {
	APIBase string
	Client  *http.Client // nil in production: NewHTTPClient on cloudflareHosts
}

var cloudflareWireDefault = cloudflareWire{APIBase: "https://api.cloudflare.com/client/v4"}

var cloudflareHosts = []string{"https://api.cloudflare.com"}

// cloudflareProvider is registered by #1671 (with its "the person's key"
// field) — not before: without a stored key every call would be 409.
var cloudflareProvider = Provider{ID: Cloudflare, Billing: BillingFree, NameJa: "Cloudflare", NameEn: "Cloudflare",
	Ops: []OpSpec{
		{Name: "pagesProjects", Args: []string{"accountId", "limit"}, Check: checkCloudflarePagesProjects},
		{Name: "pagesDeployments", Args: []string{"project", "accountId", "env", "limit"}, Check: checkCloudflarePagesDeployments},
		{Name: "workerDeployments", Args: []string{"script", "accountId", "limit"}, Check: checkCloudflareWorkerDeployments},
	},
	Hosts: cloudflareHosts,
	New: func(Secrets) (map[string]Operation, error) {
		c := newCloudflare(cloudflareWireDefault)
		return map[string]Operation{"pagesProjects": OperationFunc(c.pagesProjects),
			"pagesDeployments": OperationFunc(c.pagesDeployments), "workerDeployments": OperationFunc(c.workerDeployments)}, nil
	},
}

type cloudflare struct {
	wire cloudflareWire
	hc   *http.Client
}

func newCloudflare(w cloudflareWire) *cloudflare {
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(cloudflareHosts, DefaultTimeout)
	}
	return &cloudflare{wire: w, hc: hc}
}

var (
	cfAccountRe = regexp.MustCompile(`^[0-9a-f]{32}$`)
	cfPagesRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,57}$`)
	cfScriptRe  = regexp.MustCompile(`^[a-z0-9_][a-z0-9_-]{0,62}$`)
	cfIDRe      = regexp.MustCompile(`^[0-9a-f-]{1,64}$`)
	cfHostRe    = regexp.MustCompile(`^https://[a-z0-9][a-z0-9.-]{0,252}\.pages\.dev$`)
)

// --- arguments ---

func cfLimit(a map[string]string) (int, error) { return argInt(a, "limit", 10, 1, 25) }

func cfAccount(a map[string]string) (string, error) {
	id := strings.TrimSpace(a["accountId"])
	if id != "" && !cfAccountRe.MatchString(id) {
		return "", fmt.Errorf("%w: accountId must be a Cloudflare account id (32 hex)", ErrUnknown)
	}
	return id, nil
}

func cfName(a map[string]string, arg string, re *regexp.Regexp) (string, error) {
	v := strings.TrimSpace(a[arg])
	if !re.MatchString(v) {
		return "", fmt.Errorf("%w: %s must be a Cloudflare %s name", ErrUnknown, arg, arg)
	}
	return v, nil
}

func checkCloudflarePagesProjects(a map[string]string) error {
	if _, err := cfAccount(a); err != nil {
		return err
	}
	_, err := cfLimit(a)
	return err
}

func checkCloudflarePagesDeployments(a map[string]string) error {
	if err := checkCloudflarePagesProjects(a); err != nil {
		return err
	}
	if _, err := cfName(a, "project", cfPagesRe); err != nil {
		return err
	}
	if e := strings.TrimSpace(a["env"]); e != "" && e != "production" && e != "preview" {
		return fmt.Errorf("%w: env must be production or preview", ErrUnknown)
	}
	return nil
}

func checkCloudflareWorkerDeployments(a map[string]string) error {
	if err := checkCloudflarePagesProjects(a); err != nil {
		return err
	}
	_, err := cfName(a, "script", cfScriptRe)
	return err
}

// --- answers ---

type cfPagesDeployment struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	Environment string    `json:"environment"`
	CreatedOn   time.Time `json:"created_on"`
	ProjectName string    `json:"project_name"`
	LatestStage struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"latest_stage"`
	Trigger struct {
		Metadata struct {
			Branch        string `json:"branch"`
			CommitMessage string `json:"commit_message"`
		} `json:"metadata"`
	} `json:"deployment_trigger"`
}

// cfPagesState: the newest stage's status (a finished deploy stage is success).
func cfPagesState(stage, status string) string {
	switch status {
	case "success":
		if stage == "deploy" {
			return deployReady
		}
		return deployBuilding
	case "failure":
		return deployError
	case "canceled":
		return deployCanceled
	case "active":
		if stage == "queued" {
			return deployQueued
		}
		return deployBuilding
	case "idle":
		return deployQueued
	}
	return ""
}

// row: title = the project, value = the state, subtitle = 本番 / プレビュー ·
// branch · commit message · time, link = the dashboard's deployment page.
func (d cfPagesDeployment) row(account, project string) Row {
	env := "プレビュー"
	if d.Environment == "production" {
		env = "本番"
	}
	msg, _, _ := strings.Cut(d.Trigger.Metadata.CommitMessage, "\n")
	if r := []rune(msg); len(r) > 60 {
		msg = string(r[:59]) + "…"
	}
	r := Row{Title: project, Value: cfPagesState(d.LatestStage.Name, d.LatestStage.Status),
		Subtitle: deploySubtitle(env, d.Trigger.Metadata.Branch, msg, deployTime(d.CreatedOn))}
	switch {
	case cfIDRe.MatchString(d.ID) && cfPagesRe.MatchString(project):
		r.URL = "https://dash.cloudflare.com/" + account + "/pages/view/" + project + "/" + d.ID
	case cfHostRe.MatchString(d.URL):
		r.URL = d.URL
	}
	return r
}

type cfWorkerDeployment struct {
	Source      string            `json:"source"`
	CreatedOn   time.Time         `json:"created_on"`
	Annotations map[string]string `json:"annotations"`
}

// --- ops ---

// pagesProjects lists the account's Pages projects with each one's newest
// deployment (no value when it has none).
func (c *cloudflare) pagesProjects(ctx context.Context, in Input) (Output, error) {
	if err := checkCloudflarePagesProjects(in.Args); err != nil {
		return Output{}, err
	}
	account, err := c.account(ctx, in)
	if err != nil {
		return Output{}, err
	}
	n, _ := cfLimit(in.Args)
	var r []struct {
		Name   string             `json:"name"`
		Latest *cfPagesDeployment `json:"latest_deployment"`
	}
	q := url.Values{"page": {"1"}, "per_page": {strconv.Itoa(n)}}
	if err := c.get(ctx, in, "/accounts/"+account+"/pages/projects", q, &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, min(len(r), n))}
	for _, p := range r[:min(len(r), n)] {
		if p.Latest == nil {
			out.Rows = append(out.Rows, Row{Title: p.Name})
			continue
		}
		out.Rows = append(out.Rows, p.Latest.row(account, p.Name))
	}
	return out, nil
}

// pagesDeployments lists one Pages project's deployments, newest first.
func (c *cloudflare) pagesDeployments(ctx context.Context, in Input) (Output, error) {
	if err := checkCloudflarePagesDeployments(in.Args); err != nil {
		return Output{}, err
	}
	account, err := c.account(ctx, in)
	if err != nil {
		return Output{}, err
	}
	project, _ := cfName(in.Args, "project", cfPagesRe)
	n, _ := cfLimit(in.Args)
	q := url.Values{"page": {"1"}, "per_page": {strconv.Itoa(n)}}
	if e := strings.TrimSpace(in.Args["env"]); e != "" {
		q.Set("env", e)
	}
	var r []cfPagesDeployment
	if err := c.get(ctx, in, "/accounts/"+account+"/pages/projects/"+project+"/deployments", q, &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, min(len(r), n))}
	for _, d := range r[:min(len(r), n)] {
		out.Rows = append(out.Rows, d.row(account, project))
	}
	return out, nil
}

// workerDeployments lists one Worker's deployments, newest first. A Worker
// deployment either went live or did not happen, so the newest is 公開中 and
// the others have no value; subtitle = how (wrangler / dashboard / api) ·
// the message · time.
func (c *cloudflare) workerDeployments(ctx context.Context, in Input) (Output, error) {
	if err := checkCloudflareWorkerDeployments(in.Args); err != nil {
		return Output{}, err
	}
	account, err := c.account(ctx, in)
	if err != nil {
		return Output{}, err
	}
	script, _ := cfName(in.Args, "script", cfScriptRe)
	n, _ := cfLimit(in.Args)
	var r struct {
		Deployments []cfWorkerDeployment `json:"deployments"`
	}
	if err := c.get(ctx, in, "/accounts/"+account+"/workers/scripts/"+script+"/deployments", url.Values{}, &r); err != nil {
		return Output{}, err
	}
	ds := r.Deployments
	slices.SortStableFunc(ds, func(a, b cfWorkerDeployment) int { return b.CreatedOn.Compare(a.CreatedOn) })
	out := Output{Rows: make([]Row, 0, min(len(ds), n))}
	for i, d := range ds[:min(len(ds), n)] {
		msg, _, _ := strings.Cut(d.Annotations["workers/message"], "\n")
		if r := []rune(msg); len(r) > 60 {
			msg = string(r[:59]) + "…"
		}
		row := Row{Title: script, Subtitle: deploySubtitle(d.Source, msg, deployTime(d.CreatedOn)),
			URL: "https://dash.cloudflare.com/" + account + "/workers/services/view/" + script + "/production/deployments"}
		if i == 0 {
			row.Value = deployLive
		}
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

// account is accountId, or the token's only account (two or more, or none
// it may list, is bad_request: pass accountId).
func (c *cloudflare) account(ctx context.Context, in Input) (string, error) {
	if id, _ := cfAccount(in.Args); id != "" {
		return id, nil
	}
	var r []struct {
		ID string `json:"id"`
	}
	if err := c.get(ctx, in, "/accounts", url.Values{"per_page": {"5"}}, &r); err != nil {
		if errors.Is(err, ErrNotConnected) {
			return "", fmt.Errorf("%w: pass accountId (the token cannot list accounts)", ErrUnknown)
		}
		return "", err
	}
	if len(r) != 1 || !cfAccountRe.MatchString(r[0].ID) {
		return "", fmt.Errorf("%w: the token sees %d accounts; pass accountId", ErrUnknown, len(r))
	}
	return r[0].ID, nil
}

// get reads Cloudflare's envelope ({success, errors, result}) into out.
// 401 is a key Cloudflare no longer takes (the connection turns to error);
// 403 is a key without the permission (ErrNotConnected: make a token with
// it); 404 is a project / script that does not exist (bad_request); 429 /
// 5xx are retryable.
func (c *cloudflare) get(ctx context.Context, in Input, path string, q url.Values, out any) error {
	if in.Token == nil || in.Token.AccessToken == "" {
		return ErrNotConnected
	}
	u := c.wire.APIBase + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+in.Token.AccessToken)
	req.Header.Set("Accept", "application/json")
	env := struct {
		Success bool `json:"success"`
		Result  any  `json:"result"`
	}{Result: out}
	err = sendJSON(ctx, c.hc, req, &env)
	var pe *ProviderError
	if errors.As(err, &pe) {
		switch pe.Status {
		case http.StatusUnauthorized:
			return ErrInvalidGrant
		case http.StatusForbidden:
			return ErrNotConnected
		case http.StatusNotFound:
			return &ProviderError{Status: pe.Status, Err: fmt.Errorf("%w: cloudflare has no such project or script", ErrUnknown)}
		}
	}
	if err != nil {
		return err
	}
	if !env.Success {
		return &ProviderError{Status: http.StatusOK, Err: errors.New("cloudflare answered success:false")}
	}
	return nil
}
