package extapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// listMonthCalls is GET /v1/me/connections' monthCalls of uid as the runner
// answers them (nil = absent).
func (e *env) listMonthCalls(uid string) map[string]*int {
	e.t.Helper()
	mux := http.NewServeMux()
	e.svc.Routes(mux)
	b, _ := json.Marshal(ConnBody{UID: uid})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/internal/ext/connections/list", bytes.NewReader(b)))
	if rec.Code != http.StatusOK {
		e.t.Fatalf("list %d %s", rec.Code, rec.Body.String())
	}
	var out ConnectionList
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		e.t.Fatal(err)
	}
	m := map[string]*int{}
	for _, c := range out.Connections {
		m[c.Provider] = c.MonthCalls
	}
	return m
}

func wantMonthCalls(t *testing.T, got map[string]*int, provider string, want int) {
	t.Helper()
	if n := got[provider]; n == nil || *n != want {
		t.Fatalf("%s monthCalls = %v, want %d", provider, n, want)
	}
}

// #2144: each connection carries the user's calls of this month, counted
// whether or not the provider has limits; a refused call is not counted
// and the next month starts from 0.
func TestMonthCalls(t *testing.T) {
	cases := []struct {
		name   string
		limits Limits
	}{
		{"limits on", Limits{UserHourly: 2, UserDaily: 10, AppDaily: 10, GlobalDaily: 10}},
		{"limits off", Limits{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, opts{cfg: withLimits(c.limits)})
			e.connect(user)
			wantMonthCalls(t, e.listMonthCalls(user), FakeOAuth, 0)

			for range 2 {
				if _, err := e.call(e.req(FakeOAuth, "listEvents", nil)); err != nil {
					t.Fatal(err)
				}
			}
			_, err := e.call(e.req(FakeOAuth, "listEvents", nil))
			if c.limits.UserHourly > 0 {
				_ = limitErr(t, err) // the third is stopped and not counted
			} else if err != nil {
				t.Fatal(err)
			}
			want := 3
			if c.limits.UserHourly > 0 {
				want = 2
			}
			wantMonthCalls(t, e.listMonthCalls(user), FakeOAuth, want)
			if got := e.listMonthCalls("someone-else"); len(got) != 0 {
				t.Fatalf("another uid sees %v", got)
			}

			e.c.t = time.Date(2026, 12, 1, 0, 0, 0, 0, e.c.t.Location())
			wantMonthCalls(t, e.listMonthCalls(user), FakeOAuth, 0)
		})
	}
}

// A call that never reached the provider is given back on the month too.
func TestMonthCallsRefund(t *testing.T) {
	e := newEnv(t, opts{cfg: withLimits(Limits{})})
	// not connected: refunded
	if _, err := e.call(e.req(FakeOAuth, "listEvents", nil)); Reason(err) != ReasonNotConnected {
		t.Fatalf("err %v", err)
	}
	counts, err := e.svc.MonthCalls(context.Background(), user, []string{FakeOAuth, FakeFree})
	if err != nil {
		t.Fatal(err)
	}
	if counts[FakeOAuth] != 0 || counts[FakeFree] != 0 {
		t.Fatalf("counts %v", counts)
	}
	// a free provider (no connection) still counts per provider
	if _, err := e.call(e.req(FakeFree, "listItems", nil)); err != nil {
		t.Fatal(err)
	}
	if counts, _ = e.svc.MonthCalls(context.Background(), user, []string{FakeOAuth, FakeFree}); counts[FakeFree] != 1 || counts[FakeOAuth] != 0 {
		t.Fatalf("counts %v", counts)
	}
}

// The month is the last counter of the limits' Consume: when the store is
// down it is counted in the fallback with the limits, and a refused call
// counts on neither.
func TestMonthCallsFallback(t *testing.T) {
	e := newEnv(t, opts{cfg: withLimits(Limits{UserHourly: 1})})
	e.svc.UseLimitStore(failingStore{})
	if _, err := e.call(e.req(FakeFree, "listItems", nil)); err != nil {
		t.Fatal(err)
	}
	_ = limitErr(t, func() error { _, err := e.call(e.req(FakeFree, "listItems", nil)); return err }())
	peeker, ok := e.svc.limits.fallback.(ratelimit.Peeker)
	if !ok {
		t.Fatal("fallback cannot peek")
	}
	counts, err := peeker.Peek(context.Background(), []ratelimit.Counter{e.svc.monthCounter(FakeFree, user, e.svc.now())})
	if err != nil {
		t.Fatal(err)
	}
	if counts[0] != 1 {
		t.Fatalf("fallback month = %d, want 1", counts[0])
	}
}
