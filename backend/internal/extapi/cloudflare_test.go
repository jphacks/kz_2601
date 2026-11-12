package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

const cfTestAccount = "0123456789abcdef0123456789abcdef"

// fakeCloudflare is Cloudflare's API v4 (Pages, Workers, accounts) on httptest.
type fakeCloudflare struct {
	mu        sync.Mutex
	accounts  []map[string]any
	paths     []string
	lastQuery url.Values
}

func newFakeCloudflare(t *testing.T) (*fakeCloudflare, *httptest.Server) {
	f := &fakeCloudflare{accounts: []map[string]any{{"id": cfTestAccount, "name": "me"}}}
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, result any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "errors": []any{}, "messages": []any{}, "result": result})
	}
	fail := func(w http.ResponseWriter, status, code int) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "errors": []map[string]any{{"code": code, "message": "x"}}, "result": nil})
	}
	authed := func(w http.ResponseWriter, r *http.Request) bool {
		f.mu.Lock()
		f.paths = append(f.paths, r.URL.Path)
		f.lastQuery = r.URL.Query()
		f.mu.Unlock()
		switch strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") {
		case "cf-token":
			return true
		case "noperm":
			fail(w, 403, 10000)
		case "busy":
			fail(w, 503, 0)
		case "weird": // a 200 with success:false
			fail(w, 200, 1000)
			return false
		default:
			fail(w, 401, 10001)
		}
		return false
	}
	mux.HandleFunc("GET /client/v4/accounts", func(w http.ResponseWriter, r *http.Request) {
		if authed(w, r) {
			f.mu.Lock()
			defer f.mu.Unlock()
			ok(w, f.accounts)
		}
	})
	pagesDeployment := func(id, env, stage, status string) map[string]any {
		return map[string]any{"id": id, "url": "https://" + id[:8] + ".site.pages.dev", "environment": env, "created_on": "2026-11-11T04:00:00.000000Z",
			"latest_stage":       map[string]string{"name": stage, "status": status},
			"deployment_trigger": map[string]any{"type": "github:push", "metadata": map[string]string{"branch": "main", "commit_message": "feat: top page\nmore"}}}
	}
	mux.HandleFunc("GET /client/v4/accounts/{acc}/pages/projects", func(w http.ResponseWriter, r *http.Request) {
		if authed(w, r) {
			ok(w, []map[string]any{
				{"name": "site", "latest_deployment": pagesDeployment("4f1c2b3a-0000-4000-8000-000000000001", "production", "deploy", "success")},
				{"name": "blog", "latest_deployment": pagesDeployment("4f1c2b3a-0000-4000-8000-000000000002", "preview", "build", "failure")},
				{"name": "new", "latest_deployment": nil},
			})
		}
	})
	mux.HandleFunc("GET /client/v4/accounts/{acc}/pages/projects/{name}/deployments", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		if r.PathValue("name") == "missing" {
			fail(w, 404, 8000007)
			return
		}
		ok(w, []map[string]any{
			pagesDeployment("4f1c2b3a-0000-4000-8000-000000000003", "production", "build", "active"),
			pagesDeployment("4f1c2b3a-0000-4000-8000-000000000004", "production", "queued", "active"),
			pagesDeployment("4f1c2b3a-0000-4000-8000-000000000005", "production", "deploy", "canceled"),
		})
	})
	mux.HandleFunc("GET /client/v4/accounts/{acc}/workers/scripts/{script}/deployments", func(w http.ResponseWriter, r *http.Request) {
		if authed(w, r) {
			ok(w, map[string]any{"deployments": []map[string]any{
				{"id": "d1", "source": "dashboard", "created_on": "2026-11-10T04:00:00Z", "strategy": "percentage"},
				{"id": "d2", "source": "wrangler", "created_on": "2026-11-11T04:00:00Z", "annotations": map[string]string{"workers/message": "v2 リリース"}},
			}})
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return f, srv
}

func cfTestClient(t *testing.T) (*cloudflare, *fakeCloudflare) {
	f, srv := newFakeCloudflare(t)
	return newCloudflare(cloudflareWire{APIBase: srv.URL + "/client/v4", Client: srv.Client()}), f
}

var cfTok = &Token{AccessToken: "cf-token"}

func TestCloudflarePagesProjects(t *testing.T) {
	c, f := cfTestClient(t)
	out, err := c.pagesProjects(context.Background(), Input{Token: cfTok})
	if err != nil {
		t.Fatal(err)
	}
	// no accountId: the token's only account is looked up first
	if len(f.paths) != 2 || f.paths[0] != "/client/v4/accounts" || f.paths[1] != "/client/v4/accounts/"+cfTestAccount+"/pages/projects" ||
		f.lastQuery.Get("per_page") != "10" {
		t.Fatalf("requests %v %v", f.paths, f.lastQuery)
	}
	dash := "https://dash.cloudflare.com/" + cfTestAccount + "/pages/view/"
	want := []Row{
		{Title: "site", Value: "成功", Subtitle: "本番 · main · feat: top page · 2026-11-11 13:00", URL: dash + "site/4f1c2b3a-0000-4000-8000-000000000001"},
		{Title: "blog", Value: "失敗", Subtitle: "プレビュー · main · feat: top page · 2026-11-11 13:00", URL: dash + "blog/4f1c2b3a-0000-4000-8000-000000000002"},
		{Title: "new"},
	}
	if len(out.Rows) != len(want) {
		t.Fatalf("rows %+v", out.Rows)
	}
	for i := range want {
		if out.Rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, out.Rows[i], want[i])
		}
	}
}

func TestCloudflarePagesDeployments(t *testing.T) {
	c, f := cfTestClient(t)
	out, err := c.pagesDeployments(context.Background(), Input{Token: cfTok,
		Args: map[string]string{"project": "site", "accountId": cfTestAccount, "env": "production", "limit": "2"}})
	if err != nil {
		t.Fatal(err)
	}
	// accountId given: no account lookup; limit cuts the rows
	if len(f.paths) != 1 || f.lastQuery.Get("env") != "production" || f.lastQuery.Get("per_page") != "2" || len(out.Rows) != 2 {
		t.Fatalf("requests %v %v rows %d", f.paths, f.lastQuery, len(out.Rows))
	}
	if out.Rows[0].Value != "ビルド中" || out.Rows[1].Value != "待ち" || out.Rows[0].Title != "site" {
		t.Fatalf("rows %+v", out.Rows)
	}
	out, _ = c.pagesDeployments(context.Background(), Input{Token: cfTok, Args: map[string]string{"project": "site", "accountId": cfTestAccount}})
	if len(out.Rows) != 3 || out.Rows[2].Value != "取り消し" {
		t.Fatalf("rows %+v", out.Rows)
	}
}

func TestCloudflareWorkerDeployments(t *testing.T) {
	c, f := cfTestClient(t)
	out, err := c.workerDeployments(context.Background(), Input{Token: cfTok, Args: map[string]string{"script": "api-worker", "accountId": cfTestAccount}})
	if err != nil {
		t.Fatal(err)
	}
	if f.paths[0] != "/client/v4/accounts/"+cfTestAccount+"/workers/scripts/api-worker/deployments" {
		t.Fatalf("path %v", f.paths)
	}
	link := "https://dash.cloudflare.com/" + cfTestAccount + "/workers/services/view/api-worker/production/deployments"
	want := []Row{ // newest first, only the newest is live
		{Title: "api-worker", Value: "公開中", Subtitle: "wrangler · v2 リリース · 2026-11-11 13:00", URL: link},
		{Title: "api-worker", Subtitle: "dashboard · 2026-11-10 13:00", URL: link},
	}
	for i := range want {
		if out.Rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, out.Rows[i], want[i])
		}
	}
}

func TestCloudflareErrors(t *testing.T) {
	c, f := cfTestClient(t)
	ctx := context.Background()
	acc := map[string]string{"accountId": cfTestAccount}
	if _, err := c.pagesProjects(ctx, Input{Args: acc}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no key: %v", err)
	}
	if _, err := c.pagesProjects(ctx, Input{Token: &Token{AccessToken: "dead"}, Args: acc}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("401: %v", err)
	}
	if _, err := c.pagesProjects(ctx, Input{Token: &Token{AccessToken: "noperm"}, Args: acc}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("403: %v", err)
	}
	// a key that cannot list accounts: pass accountId
	if _, err := c.pagesProjects(ctx, Input{Token: &Token{AccessToken: "noperm"}}); Reason(err) != ReasonBadRequest {
		t.Fatalf("403 on accounts: %v", err)
	}
	_, err := c.pagesProjects(ctx, Input{Token: &Token{AccessToken: "busy"}, Args: acc})
	var pe *ProviderError
	if !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream {
		t.Fatalf("503: %v", err)
	}
	if _, err := c.pagesProjects(ctx, Input{Token: &Token{AccessToken: "weird"}, Args: acc}); !errors.As(err, &pe) || pe.Retryable {
		t.Fatalf("success:false: %v", err)
	}
	_, err = c.pagesDeployments(ctx, Input{Token: cfTok, Args: map[string]string{"project": "missing", "accountId": cfTestAccount}})
	if Reason(err) != ReasonBadRequest || !errors.As(err, &pe) || pe.Retryable {
		t.Fatalf("404: %v", err)
	}
	// two accounts and no accountId: bad_request
	f.accounts = append(f.accounts, map[string]any{"id": "fedcba9876543210fedcba9876543210"})
	if _, err := c.pagesProjects(ctx, Input{Token: cfTok}); Reason(err) != ReasonBadRequest {
		t.Fatalf("two accounts: %v", err)
	}
	// bad arguments never reach Cloudflare
	n := len(f.paths)
	for _, a := range []map[string]string{{"project": "../x"}, {"project": "Site"}, {"project": "site", "accountId": "x"}, {"project": "site", "env": "dev"}} {
		if _, err := c.pagesDeployments(ctx, Input{Token: cfTok, Args: a}); !errors.Is(err, ErrUnknown) {
			t.Errorf("%v: %v", a, err)
		}
	}
	if _, err := c.workerDeployments(ctx, Input{Token: cfTok, Args: map[string]string{"script": "a/b"}}); !errors.Is(err, ErrUnknown) {
		t.Errorf("script: %v", err)
	}
	if len(f.paths) != n {
		t.Fatal("a bad argument reached Cloudflare")
	}
}

func TestCloudflareCheck(t *testing.T) {
	p := cloudflareProvider
	bad := map[string][]map[string]string{
		"pagesProjects":     {{"limit": "0"}, {"limit": "26"}, {"accountId": "ABC"}, {"project": "x"}},
		"pagesDeployments":  {{}, {"project": "-x"}, {"project": strings.Repeat("a", 59)}, {"project": "site", "env": "all"}},
		"workerDeployments": {{}, {"script": "A"}, {"script": strings.Repeat("a", 64)}, {"script": "w", "accountId": "1"}},
	}
	for op, cases := range bad {
		for i, a := range cases {
			if _, err := p.CheckArgs(op, a); !errors.Is(err, ErrUnknown) {
				t.Errorf("%s case %d: %v", op, i, err)
			}
		}
	}
	for op, a := range map[string]map[string]string{
		"pagesProjects":     {"limit": "25"},
		"pagesDeployments":  {"project": "my-site", "env": "preview", "accountId": cfTestAccount},
		"workerDeployments": {"script": "api_worker-2"},
	} {
		if _, err := p.CheckArgs(op, a); err != nil {
			t.Errorf("%s: %v", op, err)
		}
	}
}

// The definition is valid, read only, the API host only — and not
// registered until #1671 can hold the person's key.
func TestCloudflareDefinition(t *testing.T) {
	p := cloudflareProvider
	if err := p.check(); err != nil {
		t.Fatal(err)
	}
	if _, ok := Lookup(Cloudflare); ok {
		t.Fatal("registered before #1671 (a person's API key)")
	}
	if p.Billing != BillingFree || p.EnvName() != "CLOUDFLARE" || len(p.Hosts) != 1 {
		t.Fatalf("%+v", p)
	}
	for _, o := range p.Ops {
		if o.Write || o.CacheTTL != 0 {
			t.Errorf("%s must be an uncached read", o.Name)
		}
	}
	u, _ := url.Parse(cloudflareWireDefault.APIBase + "/accounts")
	if !AllowedURL(p.Hosts, u) {
		t.Fatal("API not on the allowlist")
	}
	ops, err := p.New(Secrets{})
	if err != nil || len(ops) != len(p.Ops) {
		t.Fatalf("ops %v %v", ops, err)
	}
	if _, err := newCloudflare(cloudflareWireDefault).hc.Get("https://evil.example.com/"); !errors.Is(err, ErrEgress) {
		t.Fatalf("egress: %v", err)
	}
}
