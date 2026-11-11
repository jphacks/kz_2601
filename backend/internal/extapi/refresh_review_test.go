package extapi

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"
)

// rotatingClient is a provider that rotates the refresh token: the one
// used is refused afterwards (Microsoft / Todoist / Notion).
type rotatingClient struct {
	fakeOAuthClient
	expiry time.Time
	// gate holds every refresh until it is closed (nil: no wait).
	gate chan struct{}
	// before runs once at the start of the next refresh (sequential tests).
	before func()

	mu      sync.Mutex
	current string
	n       int
}

func (c *rotatingClient) Refresh(_ context.Context, t Token) (Token, error) {
	if f := c.before; f != nil {
		c.before = nil
		f()
	}
	if c.gate != nil {
		<-c.gate
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	if t.RefreshToken != c.current {
		return Token{}, ErrInvalidGrant
	}
	next := "r" + strconv.Itoa(c.n+1)
	c.current = next
	return Token{AccessToken: "fake-access-" + next, RefreshToken: next, Expiry: c.expiry, Scopes: t.Scopes}, nil
}

func (c *rotatingClient) refreshes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// expiredConnection stores an expired token whose refresh token is r1.
func expiredConnection(t *testing.T, e *env) *rotatingClient {
	t.Helper()
	rc := &rotatingClient{current: "r1", expiry: e.c.now().Add(time.Hour)}
	e.svc.oauth[FakeOAuth] = rc
	if err := e.svc.Conns.Save(context.Background(), user, FakeOAuth,
		Token{AccessToken: "fake-access-r1", RefreshToken: "r1", Expiry: e.c.now().Add(-time.Minute), Scopes: []string{"fake.read"}}); err != nil {
		t.Fatal(err)
	}
	return rc
}

// Calls at the same time on one instance refresh the expired token once;
// the others use the renewed token instead of sending the rotated one.
func TestConcurrentCallsRefreshOnce(t *testing.T) {
	e := newEnv(t, opts{})
	rc := expiredConnection(t, e)
	rc.gate = make(chan struct{})
	time.AfterFunc(50*time.Millisecond, func() { close(rc.gate) })
	const n = 5
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.call(e.req(FakeOAuth, "listEvents", nil))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("call: %v", err)
		}
	}
	if rc.refreshes() != 1 {
		t.Fatalf("refreshed %d times", rc.refreshes())
	}
	if c, _ := e.conn(user); c.Status != StatusActive {
		t.Fatalf("connection %q", c.Status)
	}
}

// Another instance renewed (and rotated) the token while this one was
// refreshing: the refusal of the old refresh token does not cut the
// connection; the renewed token is used.
func TestRefreshRefusedAfterOtherInstanceRenewed(t *testing.T) {
	e := newEnv(t, opts{})
	rc := expiredConnection(t, e)
	other := New(e.svc.cfg, e.q, e.coins, e.svc.Conns, e.mem, e.svc.Log)
	other.Now = e.c.now
	other.oauth[FakeOAuth] = rc
	rc.before = func() {
		if _, err := other.Call(context.Background(), e.req(FakeOAuth, "listEvents", nil)); err != nil {
			t.Errorf("other instance: %v", err)
		}
	}
	if _, err := e.call(e.req(FakeOAuth, "listEvents", nil)); err != nil {
		t.Fatalf("call: %v", err)
	}
	if c, _ := e.conn(user); c.Status != StatusActive {
		t.Fatalf("connection %q", c.Status)
	}
	tok, err := e.svc.Conns.Token(context.Background(), user, FakeOAuth)
	if err != nil || tok.RefreshToken != "r2" {
		t.Fatalf("stored %v %v", tok.RefreshToken, err)
	}
}

// A refresh token the provider refused with nobody renewing it still cuts
// the connection.
func TestRefreshRefusedMarksError(t *testing.T) {
	e := newEnv(t, opts{})
	rc := expiredConnection(t, e)
	rc.current = "rotated-elsewhere"
	if _, err := e.call(e.req(FakeOAuth, "listEvents", nil)); Reason(err) != ReasonNotConnected {
		t.Fatalf("err %v", err)
	}
	if c, _ := e.conn(user); c.Status != StatusError {
		t.Fatalf("connection %q", c.Status)
	}
}

// A refresh waiting for another one gives up with its context.
func TestKeyedLockContext(t *testing.T) {
	var l keyedLock
	unlock, err := l.lock(context.Background(), "k")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.lock(ctx, "k"); err == nil {
		t.Fatal("took a held lock")
	}
	unlock()
	u2, err := l.lock(context.Background(), "k")
	if err != nil {
		t.Fatal(err)
	}
	u2()
	if len(l.m) != 0 {
		t.Fatalf("entries left: %d", len(l.m))
	}
}
