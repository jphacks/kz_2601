package extapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

func withLimits(l Limits) func(*Config) {
	return func(c *Config) {
		c.Limits = map[string]Limits{FakeFree: l, FakePaid: l, FakeOAuth: l}
		c.UpstreamPause = UpstreamPause
	}
}

// limitErr is err's LimitError (fails when it is not one).
func limitErr(t *testing.T, err error) *LimitError {
	t.Helper()
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("err = %v, want a LimitError", err)
	}
	if Reason(err) != ReasonRateLimited || StatusOf(Reason(err)) != http.StatusTooManyRequests {
		t.Fatalf("reason %q status %d", Reason(err), StatusOf(Reason(err)))
	}
	return le
}

// Each counter stops the call one past its cap, with its scope and the time
// to its reset (fake clock: 12:00 JST); the next window counts again.
func TestCallLimits(t *testing.T) {
	type call struct{ uid, app string }
	cases := []struct {
		name    string
		limits  Limits
		calls   []call // all allowed
		blocked call
		scope   string
		retry   time.Duration
		advance time.Duration // then the blocked call goes through
	}{
		{"user hourly", Limits{UserHourly: 2}, []call{{"u1", "a1"}, {"u1", "a2"}}, call{"u1", "a3"}, LimitUserHourly, time.Hour, time.Hour},
		{"user daily", Limits{UserDaily: 2}, []call{{"u1", "a1"}, {"u1", "a2"}}, call{"u1", "a3"}, LimitUserDaily, 12 * time.Hour, 12 * time.Hour},
		{"app hourly", Limits{AppHourly: 2}, []call{{"u1", "a1"}, {"u2", "a1"}}, call{"u3", "a1"}, LimitAppHourly, time.Hour, time.Hour},
		{"app daily", Limits{AppDaily: 2}, []call{{"u1", "a1"}, {"u2", "a1"}}, call{"u3", "a1"}, LimitAppDaily, 12 * time.Hour, 12 * time.Hour},
		{"global daily", Limits{GlobalDaily: 3}, []call{{"u1", "a1"}, {"u2", "a2"}, {"u3", "a3"}}, call{"u4", "a4"}, LimitGlobalDaily, 12 * time.Hour, 12 * time.Hour},
		{"user hourly before daily", Limits{UserHourly: 1, UserDaily: 5}, []call{{"u1", "a1"}}, call{"u1", "a1"}, LimitUserHourly, time.Hour, time.Hour},
		{"daily outlives the hour", Limits{UserHourly: 5, UserDaily: 1}, []call{{"u1", "a1"}}, call{"u1", "a1"}, LimitUserDaily, 12 * time.Hour, 12 * time.Hour},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, opts{cfg: withLimits(c.limits)})
			do := func(k call) error {
				r := e.req(FakeFree, "listItems", map[string]string{"q": "x"})
				r.UserUID, r.AppID = k.uid, k.app
				_, err := e.call(r)
				return err
			}
			for i, k := range c.calls {
				if err := do(k); err != nil {
					t.Fatalf("call %d: %v", i, err)
				}
			}
			runs := e.runs()
			le := limitErr(t, do(c.blocked))
			if le.Scope != c.scope || le.RetryAfter != c.retry || le.Count != len(c.calls) {
				t.Fatalf("limit %+v, want scope %s retry %v count %d", le, c.scope, c.retry, len(c.calls))
			}
			if e.runs() != runs {
				t.Fatal("a limited call counted a run")
			}
			// the blocked call charged nothing: still blocked a moment later
			e.c.t = e.c.t.Add(time.Minute)
			_ = limitErr(t, do(c.blocked))
			e.c.t = e.c.t.Add(c.advance - time.Minute)
			if err := do(c.blocked); err != nil {
				t.Fatalf("after the reset: %v", err)
			}
		})
	}
}

// Counters are per provider: one provider's cap does not stop another.
func TestCallLimitsPerProvider(t *testing.T) {
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Limits = map[string]Limits{FakeFree: {UserHourly: 1}, FakePaid: {UserHourly: 5}}
	}})
	e.fund(user, 1000)
	if _, err := e.call(e.req(FakeFree, "listItems", nil)); err != nil {
		t.Fatal(err)
	}
	_ = limitErr(t, func() error { _, err := e.call(e.req(FakeFree, "listItems", nil)); return err }())
	if _, err := e.call(e.req(FakePaid, "echo", map[string]string{"text": "a", "costUsd": "0.01"})); err != nil {
		t.Fatalf("other provider: %v", err)
	}
}

// A call that never reached the provider gives its limit count back; one
// that did (a provider failure) keeps it.
func TestCallLimitsRefund(t *testing.T) {
	cases := []struct {
		name   string
		req    func(e *env) CallRequest
		reason string
		kept   bool
	}{
		{"not connected", func(e *env) CallRequest { return e.req(FakeOAuth, "listEvents", nil) }, ReasonNotConnected, false},
		{"coins insufficient", func(e *env) CallRequest {
			return e.req(FakePaid, "echo", map[string]string{"text": "a", "costUsd": "0.01"})
		}, ReasonCoinsInsufficient, false},
		{"op refuses the arguments", func(e *env) CallRequest {
			return e.req(FakeFree, "listItems", map[string]string{"fail": "args"})
		}, ReasonBadRequest, false},
		{"provider 5xx", func(e *env) CallRequest {
			return e.req(FakeFree, "listItems", map[string]string{"fail": "5xx"})
		}, ReasonUpstream, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, opts{cfg: withLimits(Limits{UserHourly: 1})})
			if _, err := e.call(c.req(e)); Reason(err) != c.reason {
				t.Fatalf("first: %v", err)
			}
			_, err := e.call(c.req(e))
			if c.kept {
				if le := limitErr(t, err); le.Scope != LimitUserHourly {
					t.Fatalf("scope %s", le.Scope)
				}
				return
			}
			if Reason(err) != c.reason {
				t.Fatalf("second: %v (the first kept its count)", err)
			}
		})
	}
}

// A provider's 429 pauses it for a minute on this instance (other providers
// go on); after the minute it is called again.
func TestUpstreamPause(t *testing.T) {
	e := newEnv(t, opts{cfg: withLimits(Limits{})})
	_, err := e.call(e.req(FakeFree, "listItems", map[string]string{"fail": "429"}))
	if Reason(err) != ReasonUpstream {
		t.Fatalf("429: %v", err)
	}
	if !strings.Contains(e.logs.String(), `"paused":"provider_paused"`) {
		t.Fatalf("the pause is not in the log: %s", e.logs.String())
	}
	steps := []struct {
		after    time.Duration
		provider string
		paused   time.Duration // 0 = goes through
	}{
		{0, FakeFree, time.Minute},
		{30 * time.Second, FakeFree, 30 * time.Second},
		{0, FakeOAuth, 0}, // another provider (not connected, but not paused)
		{30 * time.Second, FakeFree, 0},
	}
	for i, s := range steps {
		e.c.t = e.c.t.Add(s.after)
		op := "listItems"
		if s.provider == FakeOAuth {
			op = "listEvents"
		}
		_, err := e.call(e.req(s.provider, op, nil))
		if s.paused == 0 {
			if Reason(err) == ReasonRateLimited {
				t.Fatalf("step %d: still paused", i)
			}
			continue
		}
		le := limitErr(t, err)
		if le.Scope != LimitProviderPaused || le.RetryAfter != s.paused {
			t.Fatalf("step %d: %+v", i, le)
		}
	}
}

// The ext.call line of a limited call has the scope and the counts, never a
// uid or an argument.
func TestCallLimitLog(t *testing.T) {
	e := newEnv(t, opts{cfg: withLimits(Limits{UserHourly: 1})})
	r := e.req(FakeFree, "listItems", map[string]string{"q": "secret-words"})
	if _, err := e.call(r); err != nil {
		t.Fatal(err)
	}
	e.logs.Reset()
	r.RequestID = "again"
	if _, err := e.call(r); Reason(err) != ReasonRateLimited {
		t.Fatal(err)
	}
	var line map[string]any
	for _, l := range strings.Split(strings.TrimSpace(e.logs.String()), "\n") {
		if strings.Contains(l, `"event":"ext.call"`) {
			if err := json.Unmarshal([]byte(l), &line); err != nil {
				t.Fatal(err)
			}
		}
	}
	if line["result"] != ReasonRateLimited || line["limit"] != LimitUserHourly || line["limit_max"] != 1.0 ||
		line["limit_count"] != 1.0 || line["retry_after_sec"] != 3600.0 {
		t.Fatalf("log %v", line)
	}
	if s := e.logs.String(); strings.Contains(s, user) || strings.Contains(s, "secret-words") {
		t.Fatalf("personal data in the log: %s", s)
	}
}

// The runner's answer carries the scope and the seconds to wait, and the
// client reads them back.
func TestCallLimitAnswer(t *testing.T) {
	e := newEnv(t, opts{cfg: withLimits(Limits{AppDaily: 1})})
	mux := http.NewServeMux()
	e.svc.Routes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	cl := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	b := CallBody{Provider: FakeFree, Op: "listItems", AppID: appID, CreatorUID: creator, UserUID: user, RequestID: "r1"}
	if _, err := cl.Call(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	b.RequestID = "r2"
	_, err := cl.Call(context.Background(), b)
	var re *RunnerError
	if !errors.As(err, &re) || re.Status != http.StatusTooManyRequests || re.Reason != ReasonRateLimited ||
		re.Scope != LimitAppDaily || re.RetryAfterSec != 12*3600 {
		t.Fatalf("answer %+v (%v)", re, err)
	}
	// the raw body is the FailBody
	rec := httptest.NewRecorder()
	raw, _ := json.Marshal(CallBody{Provider: FakeFree, Op: "listItems", AppID: appID, CreatorUID: creator, UserUID: user, RequestID: "r3"})
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/internal/ext/call", bytes.NewReader(raw)))
	if got := strings.TrimSpace(rec.Body.String()); got != `{"reason":"rate_limited","scope":"app_daily","retryAfterSec":43200}` {
		t.Fatalf("body %s", got)
	}
}

// The store failing falls back to counting in memory (never open).
func TestCallLimitsStoreFallback(t *testing.T) {
	e := newEnv(t, opts{cfg: withLimits(Limits{UserHourly: 1})})
	e.svc.UseLimitStore(failingStore{})
	if _, err := e.call(e.req(FakeFree, "listItems", nil)); err != nil {
		t.Fatal(err)
	}
	_ = limitErr(t, func() error { _, err := e.call(e.req(FakeFree, "listItems", nil)); return err }())
}

type failingStore struct{}

func (failingStore) Consume(context.Context, []ratelimit.Counter, int, time.Time) (ratelimit.Usage, error) {
	return ratelimit.Usage{}, errors.New("down")
}
func (failingStore) Refund(context.Context, []ratelimit.Counter, int, time.Time) error {
	return errors.New("down")
}

// EXT_LIMIT_* / EXT_GLOBAL_DAILY*: defaults, every provider, one provider, 0 = off.
func TestLimitsFromEnv(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want map[string]Limits // checked providers
		err  bool
	}{
		{"defaults", nil, map[string]Limits{FakeFree: DefaultLimits, FakePaid: DefaultLimits}, false},
		{"every provider", map[string]string{"EXT_LIMIT_USER_HOURLY": "7", "EXT_GLOBAL_DAILY": "9"},
			map[string]Limits{FakeFree: {UserHourly: 7, UserDaily: DefaultLimits.UserDaily, AppHourly: DefaultLimits.AppHourly, AppDaily: DefaultLimits.AppDaily, GlobalDaily: 9}}, false},
		{"one provider wins", map[string]string{"EXT_LIMIT_APP_DAILY": "4", "EXT_LIMIT_FAKE_PAID_APP_DAILY": "2", "EXT_GLOBAL_DAILY_FAKE_PAID": "0"},
			map[string]Limits{
				FakeFree: {UserHourly: DefaultLimits.UserHourly, UserDaily: DefaultLimits.UserDaily, AppHourly: DefaultLimits.AppHourly, AppDaily: 4, GlobalDaily: DefaultLimits.GlobalDaily},
				FakePaid: {UserHourly: DefaultLimits.UserHourly, UserDaily: DefaultLimits.UserDaily, AppHourly: DefaultLimits.AppHourly, AppDaily: 2, GlobalDaily: 0},
			}, false},
		{"negative", map[string]string{"EXT_LIMIT_USER_DAILY": "-1"}, nil, true},
		{"not a number", map[string]string{"EXT_GLOBAL_DAILY_FAKE_FREE": "many"}, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg, err := ConfigFromEnv(func(k string) string { return c.env[k] })
			if c.err {
				if err == nil {
					t.Fatal("no error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for p, want := range c.want {
				if got := cfg.Limits[p]; got != want {
					t.Fatalf("%s = %+v, want %+v", p, got, want)
				}
			}
			for _, p := range Providers() {
				if _, ok := cfg.Limits[p.ID]; !ok {
					t.Fatalf("%s has no limits", p.ID)
				}
			}
		})
	}
	if cfg, _ := ConfigFromEnv(func(string) string { return "" }); cfg.UpstreamPause != time.Minute {
		t.Fatalf("pause %v", cfg.UpstreamPause)
	}
	// the defaults are on (a forgotten env never means unlimited)
	for _, n := range []int{DefaultLimits.UserHourly, DefaultLimits.UserDaily, DefaultLimits.AppHourly, DefaultLimits.AppDaily, DefaultLimits.GlobalDaily} {
		if n <= 0 {
			t.Fatalf("a default is off: %+v", DefaultLimits)
		}
	}
}

// A bad lat / lng (or forecast limit) is refused at CheckArgs, before any
// limit is counted and before the API is called.
func TestOpenWeatherBadArgsKeepLimits(t *testing.T) {
	var status atomic.Int64
	status.Store(200)
	var seen atomic.Value
	srv := owServer(t, &status, &seen)
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[OpenWeather] = true
		c.Secrets = map[string]Secrets{OpenWeather: {APIKey: owTestKey}}
		c.Limits = map[string]Limits{OpenWeather: {UserHourly: 1, AppHourly: 1, GlobalDaily: 1}}
	}})
	e.svc.ops[OpenWeather] = owOps(t, srv)
	p, _ := Lookup(OpenWeather)
	bad := []struct {
		op   string
		args map[string]string
	}{
		{"current", map[string]string{}},
		{"current", map[string]string{"lat": "95", "lng": "139"}},
		{"airQuality", map[string]string{"lat": "35"}},
		{"airQuality", map[string]string{"lat": "0x1", "lng": "1"}},
		{"forecast", map[string]string{"lat": "35", "lng": "139e0"}},
		{"forecast", map[string]string{"lat": "35", "lng": "139", "limit": "41"}},
		{"forecast", map[string]string{"lat": "35", "lng": "139", "limit": "0"}},
	}
	for _, b := range bad {
		if _, err := p.CheckArgs(b.op, b.args); Reason(err) != ReasonBadRequest {
			t.Fatalf("CheckArgs %s %v: %v", b.op, b.args, err)
		}
		for range 3 {
			if _, err := e.call(e.req(OpenWeather, b.op, b.args)); Reason(err) != ReasonBadRequest {
				t.Fatalf("%s %v: %s (%v)", b.op, b.args, Reason(err), err)
			}
		}
	}
	if seen.Load() != nil {
		t.Fatal("a bad argument reached the API")
	}
	// the one allowed call of each limit is still there
	if _, err := e.call(e.req(OpenWeather, "forecast", map[string]string{"lat": "36.5", "lng": "136.6", "limit": "2"})); err != nil {
		t.Fatalf("good call after bad ones: %v", err)
	}
	_ = limitErr(t, func() error {
		_, err := e.call(e.req(OpenWeather, "current", map[string]string{"lat": "36.5", "lng": "136.6"}))
		return err
	}())
}
