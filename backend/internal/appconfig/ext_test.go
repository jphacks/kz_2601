package appconfig

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/extapi"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// fakeRuns is the runner's `runs` counter: Pro is the set pro, and every
// counted call is kept (creator, actor).
type fakeRuns struct {
	mu   sync.Mutex
	pro  map[string]bool
	runs []quota.Run
}

func (f *fakeRuns) ExtRun(_ context.Context, r quota.Run, _ *quota.Decision) (quota.Charge, *quota.Block, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.pro[r.CreatorUID] || r.CreatorAnonymous {
		return quota.Charge{}, &quota.Block{Scope: quota.ScopeExtNeedsPro}, nil
	}
	f.runs = append(f.runs, r)
	return quota.Charge{}, nil, nil
}

func (f *fakeRuns) ExtPro(_ context.Context, r quota.Run, _ *quota.Decision) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pro[r.CreatorUID] && !r.CreatorAnonymous
}

func (f *fakeRuns) Refund(context.Context, ...quota.Charge) error { return nil }

func (f *fakeRuns) last() quota.Run {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.runs) == 0 {
		return quota.Run{}
	}
	return f.runs[len(f.runs)-1]
}

// extEnv is appconfig in front of a real runner door (extapi.Service with the
// fake providers) on an httptest server.
type extEnv struct {
	*env
	runs   *fakeRuns
	runner *httptest.Server
	svc    *extapi.Service
}

func newExtEnv(t *testing.T, perHour int) *extEnv {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	runs := &fakeRuns{pro: map[string]bool{"owner": true, "solo": true}}
	mem := extapi.NewMemory()
	sealer, err := extapi.NewLocalSealer(nil)
	if err != nil {
		t.Fatal(err)
	}
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	conns := &extapi.Connections{Store: mem, Sealer: sealer, Log: lg}
	svc := extapi.New(extapi.Config{
		Enabled:      map[string]bool{extapi.FakeFree: true, extapi.FakePaid: true, extapi.FakeOAuth: true},
		CallbackBase: "https://runner.example", AppReturnURL: "tailorit://connected",
	}, runs, nil, conns, mem, lg)
	mux := http.NewServeMux()
	svc.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cl := &extapi.Client{BaseURL: srv.URL, HTTP: srv.Client()}
	h := auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory(), WithExt(cl), WithExtLimit(perHour, time.Now)))
	return &extEnv{env: &env{t: t, h: httpx.WithRequestID(h)}, runs: runs, runner: srv, svc: svc}
}

func wantExtErr(t *testing.T, r resp, status int, code, what string) {
	t.Helper()
	if r.code != status || r.errCode() != code {
		t.Fatalf("%s: want %d %s, got %d %s", what, status, code, r.code, r.raw)
	}
}

func TestExtCallOwnApp(t *testing.T) {
	e := newExtEnv(t, 100)
	const app = "/v1/apps/EXT-SOLO"
	e.ownApp("solo", app, kakeibo(t))

	r := e.do("solo", http.MethodPost, app+"/ext/fakeFree/listItems", `{"args":{"q":"rain"}}`, map[string]string{"X-Request-Id": "req-1"})
	rows, _ := r.body["rows"].([]any)
	if r.code != http.StatusOK || len(rows) != 2 || rows[0].(map[string]any)["subtitle"] != "rain" {
		t.Fatalf("free call: %d %s", r.code, r.raw)
	}
	if r.hdr.Get("X-Coins-Charged") != "" {
		t.Fatalf("a free provider sets no coin headers: %v", r.hdr)
	}
	if got := e.runs.last(); got.CreatorUID != "solo" || got.ActorUID != "solo" || got.AppID != "EXT-SOLO" {
		t.Fatalf("runs counted for %+v", got)
	}
	// a paid provider answers the coin headers (no biller here: 0)
	r = e.do("solo", http.MethodPost, app+"/ext/fakePaid/echo", `{"args":{"text":"hi"}}`, nil)
	if r.code != http.StatusOK || r.hdr.Get("X-Coins-Charged") != "0" || r.body["item"].(map[string]any)["title"] != "hi" {
		t.Fatalf("paid call: %d %s %v", r.code, r.raw, r.hdr)
	}

	// the contract's 400 / 404 before the runner
	wantExtErr(t, e.do("solo", http.MethodPost, app+"/ext/nope/listItems", `{"args":{}}`, nil), http.StatusNotFound, httpx.CodeNotFound, "unknown provider")
	wantExtErr(t, e.do("solo", http.MethodPost, app+"/ext/fakeFree/nope", `{"args":{}}`, nil), http.StatusNotFound, httpx.CodeNotFound, "unknown op")
	wantExtErr(t, e.do("solo", http.MethodPost, app+"/ext/fakeFree/listItems", `{"args":{"url":"x"}}`, nil), http.StatusBadRequest, httpx.CodeBadRequest, "unknown argument")
	wantExtErr(t, e.do("solo", http.MethodPost, app+"/ext/fakeFree/listItems", `{"args":{"q":"https://evil.example/x"}}`, nil), http.StatusBadRequest, httpx.CodeBadRequest, "URL argument")
	wantExtErr(t, e.do("solo", http.MethodPost, app+"/ext/fakeFree/listItems", `{"args":{"q":"`+strings.Repeat("a", extapi.MaxArgLen+1)+`"}}`, nil), http.StatusBadRequest, httpx.CodeBadRequest, "long argument")
	wantExtErr(t, e.do("solo", http.MethodPost, app+"/ext/fakeFree/listItems", `{"args":{},"url":"x"}`, nil), http.StatusBadRequest, httpx.CodeBadRequest, "unknown body field")
	wantExtErr(t, e.do("solo", http.MethodPost, "/v1/apps/null/ext/fakeFree/listItems", `{"args":{}}`, nil), http.StatusBadRequest, httpx.CodeBadRequest, "bad appId")
	// an app not in the caller's tree
	wantExtErr(t, e.do("solo", http.MethodPost, "/v1/apps/EXT-NONE/ext/fakeFree/listItems", `{"args":{}}`, nil), http.StatusNotFound, httpx.CodeNotFound, "no app")

	// the runner's reasons become the error codes
	wantExtErr(t, e.do("solo", http.MethodPost, app+"/ext/fakeFree/listItems", `{"args":{"fail":"5xx"}}`, nil), http.StatusBadGateway, httpx.CodeUpstreamUnavailable, "provider 5xx")
	wantExtErr(t, e.do("solo", http.MethodPost, app+"/ext/fakeOauth/listEvents", `{"args":{}}`, nil), http.StatusConflict, httpx.CodeExtNotConnected, "not connected")

	// no Pro: 402 from the runner, no reason for one's own app
	e.ownApp("free", "/v1/apps/EXT-FREE", kakeibo(t))
	r = e.do("free", http.MethodPost, "/v1/apps/EXT-FREE/ext/fakeFree/listItems", `{"args":{}}`, nil)
	wantExtErr(t, r, http.StatusPaymentRequired, httpx.CodeProRequired, "no Pro")
	if r.reason() != "" {
		t.Fatalf("own app: no creator reason: %s", r.raw)
	}
}

// A shared app's members call on the creator's `runs`; strangers cannot.
func TestExtCallSharedApp(t *testing.T) {
	e := newExtEnv(t, 100)
	const app = "/v1/apps/EXT-FAMILY"
	e.ownApp("owner", app, familyKakeibo(t))
	tok := e.share("owner", app)
	if r := e.do("mom", http.MethodPost, "/v1/shares/"+tok+"/import", nil, nil); r.code != http.StatusCreated {
		t.Fatalf("join: %d %s", r.code, r.raw)
	}
	r := e.do("mom", http.MethodPost, app+"/ext/fakeFree/listItems", `{"args":{}}`, nil)
	if r.code != http.StatusOK {
		t.Fatalf("member call: %d %s", r.code, r.raw)
	}
	if got := e.runs.last(); got.CreatorUID != "owner" || got.ActorUID != "mom" {
		t.Fatalf("member call counted as %+v", got)
	}
	e.do("stranger", http.MethodGet, app+"/dsl", nil, nil) // opening the appId installs a copy
	wantNotMember(t, e.do("stranger", http.MethodPost, app+"/ext/fakeFree/listItems", `{"args":{}}`, nil), "stranger")

	// the creator lost Pro: the member sees creator_needs_pro
	e.runs.mu.Lock()
	e.runs.pro["owner"] = false
	e.runs.mu.Unlock()
	r = e.do("mom", http.MethodPost, app+"/ext/fakeFree/listItems", `{"args":{}}`, nil)
	wantExtErr(t, r, http.StatusPaymentRequired, httpx.CodeProRequired, "creator without Pro")
	if r.reason() != reasonCreatorNeedsPro {
		t.Fatalf("want creator_needs_pro: %s", r.raw)
	}
}

func TestExtConnections(t *testing.T) {
	e := newExtEnv(t, 100)
	if r := e.do("solo", http.MethodGet, "/v1/me/connections", nil, nil); r.code != http.StatusOK || len(r.body["connections"].([]any)) != 0 {
		t.Fatalf("empty list: %d %s", r.code, r.raw)
	}
	wantExtErr(t, e.do("solo", http.MethodPost, "/v1/me/connections/fakeFree/start", nil, nil), http.StatusNotFound, httpx.CodeNotFound, "no OAuth provider")
	wantExtErr(t, e.do("solo", http.MethodPost, "/v1/me/connections/nope/start", nil, nil), http.StatusNotFound, httpx.CodeNotFound, "unknown provider")
	wantExtErr(t, e.do("solo", http.MethodPost, "/v1/me/connections/fakeOauth/start", `{"scopes":["other"]}`, nil), http.StatusBadRequest, httpx.CodeBadRequest, "unknown scope")

	r := e.do("solo", http.MethodPost, "/v1/me/connections/fakeOauth/start", nil, nil)
	authURL, _ := r.body["authUrl"].(string)
	if r.code != http.StatusOK || !strings.HasPrefix(authURL, "https://runner.example/oauth/callback/fakeOauth?") || r.hdr.Get("Cache-Control") != "no-store" {
		t.Fatalf("start: %d %s", r.code, r.raw)
	}
	// the consent comes back to the runner (the fake consents at once)
	u, _ := url.Parse(authURL)
	back := e.svc.Callback(context.Background(), extapi.FakeOAuth, u.Query().Get("state"), u.Query().Get("code"))
	if !strings.HasSuffix(back, "status=ok") {
		t.Fatalf("callback: %s", back)
	}
	r = e.do("solo", http.MethodGet, "/v1/me/connections", nil, nil)
	list, _ := r.body["connections"].([]any)
	if r.code != http.StatusOK || len(list) != 1 || list[0].(map[string]any)["provider"] != extapi.FakeOAuth || list[0].(map[string]any)["status"] != "active" {
		t.Fatalf("list: %d %s", r.code, r.raw)
	}
	if strings.Contains(string(r.raw), "fake-access") || strings.Contains(string(r.raw), "sealed") {
		t.Fatalf("a token reached the device: %s", r.raw)
	}
	// connected: the OAuth op runs with the token
	e.ownApp("solo", "/v1/apps/EXT-CAL", kakeibo(t))
	if r := e.do("solo", http.MethodPost, "/v1/apps/EXT-CAL/ext/fakeOauth/listEvents", `{"args":{}}`, nil); r.code != http.StatusOK || len(r.body["rows"].([]any)) != 1 {
		t.Fatalf("connected call: %d %s", r.code, r.raw)
	}
	// revoke: 204, then the list is empty and the op is 409 again
	if r := e.do("solo", http.MethodDelete, "/v1/me/connections/fakeOauth", nil, nil); r.code != http.StatusNoContent {
		t.Fatalf("revoke: %d %s", r.code, r.raw)
	}
	if r := e.do("solo", http.MethodGet, "/v1/me/connections", nil, nil); len(r.body["connections"].([]any)) != 0 {
		t.Fatalf("after revoke: %s", r.raw)
	}
	wantExtErr(t, e.do("solo", http.MethodPost, "/v1/apps/EXT-CAL/ext/fakeOauth/listEvents", `{"args":{}}`, nil), http.StatusConflict, httpx.CodeExtNotConnected, "after revoke")
	wantExtErr(t, e.do("solo", http.MethodDelete, "/v1/me/connections/fakeFree", nil, nil), http.StatusNotFound, httpx.CodeNotFound, "revoke a provider without OAuth")
}

func TestExtUserLimit(t *testing.T) {
	e := newExtEnv(t, 1)
	const app = "/v1/apps/EXT-LIMIT"
	e.ownApp("solo", app, kakeibo(t))
	if r := e.do("solo", http.MethodPost, app+"/ext/fakeFree/listItems", `{"args":{}}`, nil); r.code != http.StatusOK {
		t.Fatalf("first: %d %s", r.code, r.raw)
	}
	r := e.do("solo", http.MethodPost, app+"/ext/fakeFree/listItems", `{"args":{}}`, nil)
	wantExtErr(t, r, http.StatusTooManyRequests, httpx.CodeRateLimited, "over the hour")
	if r.hdr.Get("Retry-After") == "" {
		t.Fatal("no Retry-After")
	}
}

// The runner's call limits (#1715) are 429 rate_limited with the scope,
// the provider and Retry-After.
func TestExtRunnerLimit(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	cases := []struct {
		scope string
		sec   int
		want  string
	}{
		{extapi.LimitAppHourly, 1800, "1800"},
		{extapi.LimitGlobalDaily, 43200, "43200"},
		{extapi.LimitProviderPaused, 0, "1"},
	}
	for _, c := range cases {
		st := store.NewMemory()
		e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st, WithExt(stubRunner{err: &extapi.RunnerError{Status: 429,
			Reason: extapi.ReasonRateLimited, Scope: c.scope, RetryAfterSec: c.sec}})))}
		e.ownApp("solo", "/v1/apps/EXT-RL", kakeibo(t))
		r := e.do("solo", http.MethodPost, "/v1/apps/EXT-RL/ext/fakeFree/listItems", `{"args":{}}`, nil)
		wantExtErr(t, r, http.StatusTooManyRequests, httpx.CodeRateLimited, c.scope)
		d, _ := r.body["error"].(map[string]any)["details"].(map[string]any)
		if r.hdr.Get("Retry-After") != c.want || d["scope"] != c.scope || d["provider"] != "fakeFree" {
			t.Fatalf("%s: Retry-After %q details %v", c.scope, r.hdr.Get("Retry-After"), d)
		}
	}
}

// providerIDs returns GET /v1/me/connections providers as "id:enabled".
func providerIDs(t *testing.T, r resp) []string {
	t.Helper()
	list, ok := r.body["providers"].([]any)
	if r.code != http.StatusOK || !ok {
		t.Fatalf("providers: %d %s", r.code, r.raw)
	}
	out := []string{}
	for _, v := range list {
		p := v.(map[string]any)
		if p["nameJa"] == "" || p["nameEn"] == "" {
			t.Fatalf("no name: %v", p)
		}
		out = append(out, fmt.Sprintf("%s:%v", p["id"], p["enabled"]))
	}
	return out
}

// The list also says which services can be connected (#1566): OAuth
// providers only, enabled = the runner has it on, a fake only when on, and
// supportedFeatures narrows it (the review build sends no OAuth key).
func TestExtConnectableProviders(t *testing.T) {
	e := newExtEnv(t, 100)
	got := providerIDs(t, e.do("solo", http.MethodGet, "/v1/me/connections", nil, nil))
	want := []string{"fakeOauth:true", "googleCalendar:false", "notion:false", "outlookCalendar:false", "todoist:false"}
	if !slices.Equal(got, want) {
		t.Fatalf("providers: %v", got)
	}
	got = providerIDs(t, e.do("solo", http.MethodGet, "/v1/me/connections?supportedFeatures=externalCall.todoist,externalCall.fakeOauth&supportedFeatures=feed.v2", nil, nil))
	if !slices.Equal(got, []string{"fakeOauth:true", "todoist:false"}) {
		t.Fatalf("narrowed: %v", got)
	}
	review := "/v1/me/connections?supportedFeatures=" + url.QueryEscape(strings.Join(extapi.ReviewBuildFeatures(), ","))
	if got := providerIDs(t, e.do("solo", http.MethodGet, review, nil, nil)); len(got) != 0 {
		t.Fatalf("review build: %v", got)
	}
	if got := providerIDs(t, e.do("solo", http.MethodGet, "/v1/me/connections?supportedFeatures=", nil, nil)); len(got) != 0 {
		t.Fatalf("empty features: %v", got)
	}
}

// stubRunner answers every route with one reason.
type stubRunner struct {
	err error
}

func (s stubRunner) Call(context.Context, extapi.CallBody) (extapi.CallAnswer, error) {
	return extapi.CallAnswer{}, s.err
}
func (s stubRunner) Start(context.Context, string, string, []string) (string, error) {
	return "", s.err
}
func (s stubRunner) Revoke(context.Context, string, string) error   { return s.err }
func (s stubRunner) RevokeAll(context.Context, string) (int, error) { return 0, s.err }
func (s stubRunner) List(context.Context, string) (extapi.ConnectionList, error) {
	return extapi.ConnectionList{}, s.err
}

// Every runner reason has its code and the contract's status.
func TestExtReasonCodes(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{&extapi.RunnerError{Status: 400, Reason: extapi.ReasonBadRequest}, 400, httpx.CodeBadRequest},
		{&extapi.RunnerError{Status: 409, Reason: extapi.ReasonNotConnected}, 409, httpx.CodeExtNotConnected},
		{&extapi.RunnerError{Status: 503, Reason: extapi.ReasonUnavailable}, 503, httpx.CodeExtUnavailable},
		{&extapi.RunnerError{Status: 503, Reason: extapi.ReasonBusy}, 503, httpx.CodeExtBusy},
		{&extapi.RunnerError{Status: 409, Reason: extapi.ReasonDuplicateRequest}, 409, httpx.CodeDuplicateRequest},
		{&extapi.RunnerError{Status: 402, Reason: extapi.ReasonProRequired}, 402, httpx.CodeProRequired},
		{&extapi.RunnerError{Status: 402, Reason: extapi.ReasonAllowanceExhausted}, 402, httpx.CodeAllowanceExhausted},
		{&extapi.RunnerError{Status: 402, Reason: extapi.ReasonOverflowCap}, 402, httpx.CodeOverflowCapReached},
		{&extapi.RunnerError{Status: 402, Reason: extapi.ReasonCoinsInsufficient}, 402, httpx.CodeCoinsInsufficient},
		{&extapi.RunnerError{Status: 429, Reason: extapi.ReasonRateLimited, Scope: extapi.LimitUserDaily, RetryAfterSec: 60}, 429, httpx.CodeRateLimited},
		{&extapi.RunnerError{Status: 504, Reason: extapi.ReasonUpstreamTimeout}, 504, httpx.CodeUpstreamTimeout},
		{&extapi.RunnerError{Status: 502, Reason: extapi.ReasonUpstream}, 502, httpx.CodeUpstreamUnavailable},
		{&extapi.RunnerError{Status: 500, Reason: extapi.ReasonInternal}, 500, httpx.CodeInternal},
		{&extapi.RunnerError{Status: 418, Reason: "something_new"}, 500, httpx.CodeInternal},
		{context.DeadlineExceeded, 504, httpx.CodeUpstreamTimeout},
		{io.ErrUnexpectedEOF, 502, httpx.CodeUpstreamUnavailable},
	}
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	for _, c := range cases {
		st := store.NewMemory()
		e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st, WithExt(stubRunner{err: c.err})))}
		e.ownApp("solo", "/v1/apps/EXT-R", kakeibo(t))
		wantExtErr(t, e.do("solo", http.MethodPost, "/v1/apps/EXT-R/ext/fakeFree/listItems", `{"args":{}}`, nil), c.status, c.code, c.err.Error())
		wantExtErr(t, e.do("solo", http.MethodPost, "/v1/me/connections/fakeOauth/start", nil, nil), c.status, c.code, "start "+c.err.Error())
	}
}

// Without EXT_RUNNER_URL: calls are 503 ext_unavailable, the list is empty.
func TestExtOff(t *testing.T) {
	e := newEnv(t)
	e.ownApp("solo", "/v1/apps/EXT-OFF", kakeibo(t))
	wantExtErr(t, e.do("solo", http.MethodPost, "/v1/apps/EXT-OFF/ext/fakeFree/listItems", `{"args":{}}`, nil), http.StatusServiceUnavailable, httpx.CodeExtUnavailable, "off")
	wantExtErr(t, e.do("solo", http.MethodPost, "/v1/me/connections/fakeOauth/start", nil, nil), http.StatusServiceUnavailable, httpx.CodeExtUnavailable, "start off")
	wantExtErr(t, e.do("solo", http.MethodDelete, "/v1/me/connections/fakeOauth", nil, nil), http.StatusServiceUnavailable, httpx.CodeExtUnavailable, "revoke off")
	r := e.do("solo", http.MethodGet, "/v1/me/connections", nil, nil)
	if r.code != http.StatusOK || len(r.body["connections"].([]any)) != 0 {
		t.Fatalf("list off: %d %s", r.code, r.raw)
	}
	// the services are still listed, all off, the fake not at all
	if got := providerIDs(t, r); !slices.Equal(got, []string{"googleCalendar:false", "notion:false", "outlookCalendar:false", "todoist:false"}) {
		t.Fatalf("providers off: %v", got)
	}
}
