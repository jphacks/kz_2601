package extapi

import (
	"context"
	"slices"
	"strconv"
	"strings"
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
	// beforeRevoke runs once at the start of the next revoke.
	beforeRevoke func()

	mu      sync.Mutex
	current string
	n       int
	// revoked / refused are the refresh tokens Revoke took / refused.
	revoked, refused []string
}

// Revoke refuses a refresh token that is no longer the current one (like a
// provider that renews the token before revoking it).
func (c *rotatingClient) Revoke(_ context.Context, t Token) error {
	if f := c.beforeRevoke; f != nil {
		c.beforeRevoke = nil
		f()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if t.RefreshToken != c.current {
		c.refused = append(c.refused, t.RefreshToken)
		return ErrInvalidGrant
	}
	c.revoked = append(c.revoked, t.RefreshToken)
	return nil
}

func (c *rotatingClient) revokes() (revoked, refused []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.revoked), slices.Clone(c.refused)
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

// A revoke on this instance waits for the refresh in progress and then
// revokes the token that refresh stored.
func TestRevokeWaitsForRefresh(t *testing.T) {
	e := newEnv(t, opts{})
	rc := expiredConnection(t, e)
	rc.gate = make(chan struct{})
	started := make(chan struct{})
	rc.before = func() { close(started) }
	callDone := make(chan error, 1)
	go func() {
		_, err := e.call(e.req(FakeOAuth, "listEvents", nil))
		callDone <- err
	}()
	<-started
	revokeDone := make(chan error, 1)
	go func() { revokeDone <- e.svc.Revoke(context.Background(), user, FakeOAuth) }()
	select {
	case err := <-revokeDone:
		t.Fatalf("the revoke did not wait for the refresh: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(rc.gate)
	if err := <-callDone; err != nil {
		t.Fatalf("call: %v", err)
	}
	if err := <-revokeDone; err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if revoked, refused := rc.revokes(); !slices.Equal(revoked, []string{"r2"}) || len(refused) != 0 {
		t.Fatalf("revoked %v refused %v", revoked, refused)
	}
	if c, ok := e.conn(user); !ok || c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("connection %+v", c)
	}
}

// The provider refusing the refresh token while revoking: the stored token
// is read again once. Renewed by another instance meanwhile, the new one is
// revoked; the same token is already gone and is not tried again.
func TestRevokeRefusedRereadsToken(t *testing.T) {
	t.Run("renewed elsewhere", func(t *testing.T) {
		e := newEnv(t, opts{})
		rc := expiredConnection(t, e)
		other := New(e.svc.cfg, e.q, e.coins, e.svc.Conns, e.mem, e.svc.Log)
		other.Now = e.c.now
		other.oauth[FakeOAuth] = rc
		rc.beforeRevoke = func() {
			if _, err := other.Call(context.Background(), e.req(FakeOAuth, "listEvents", nil)); err != nil {
				t.Errorf("other instance: %v", err)
			}
		}
		if err := e.svc.Revoke(context.Background(), user, FakeOAuth); err != nil {
			t.Fatal(err)
		}
		if revoked, refused := rc.revokes(); !slices.Equal(revoked, []string{"r2"}) || !slices.Equal(refused, []string{"r1"}) {
			t.Fatalf("revoked %v refused %v", revoked, refused)
		}
		if c, ok := e.conn(user); !ok || c.Status != StatusRevoked || len(c.Sealed) != 0 {
			t.Fatalf("connection %+v", c)
		}
	})
	t.Run("already gone", func(t *testing.T) {
		e := newEnv(t, opts{})
		rc := expiredConnection(t, e)
		rc.current = "rotated-and-gone"
		if err := e.svc.Revoke(context.Background(), user, FakeOAuth); err != nil {
			t.Fatal(err)
		}
		if revoked, refused := rc.revokes(); len(revoked) != 0 || !slices.Equal(refused, []string{"r1"}) {
			t.Fatalf("revoked %v refused %v", revoked, refused)
		}
		if c, ok := e.conn(user); !ok || c.Status != StatusRevoked || len(c.Sealed) != 0 {
			t.Fatalf("connection %+v", c)
		}
		if strings.Contains(e.logs.String(), "provider revoke failed") {
			t.Fatalf("a token already gone is logged as a failure: %s", e.logs.String())
		}
	})
}
